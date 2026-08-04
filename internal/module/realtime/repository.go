package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/infra/database"
	infrarealtime "admin_back_go/internal/infra/realtime"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TargetTypeUser          = "user"
	MaxResumeLimit          = 500
	DefaultCleanupBatchSize = 1000
	MaxCleanupBatchSize     = 10000
	DurableEventRetention   = 7 * 24 * time.Hour
)

var (
	ErrRepositoryNotConfigured = errors.New("realtime event repository is not configured")
	ErrAppendInputInvalid      = errors.New("realtime event append input is invalid")
	ErrResumeQueryInvalid      = errors.New("realtime resume query is invalid")
	ErrDuplicateEventID        = errors.New("realtime event id already exists")
)

type Event struct {
	Sequence    uint64     `gorm:"column:sequence;primaryKey;autoIncrement"`
	EventID     string     `gorm:"column:event_id"`
	EventType   string     `gorm:"column:event_type"`
	RequestID   *string    `gorm:"column:request_id"`
	TargetType  string     `gorm:"column:target_type"`
	TargetID    string     `gorm:"column:target_id"`
	Durability  string     `gorm:"column:durability"`
	PayloadJSON string     `gorm:"column:payload_json"`
	OccurredAt  time.Time  `gorm:"column:occurred_at"`
	ExpiresAt   *time.Time `gorm:"column:expires_at"`
}

func (Event) TableName() string { return "realtime_events" }

type RetentionWatermark struct {
	TargetType             string    `gorm:"column:target_type;primaryKey"`
	TargetID               string    `gorm:"column:target_id;primaryKey"`
	DeletedThroughSequence uint64    `gorm:"column:deleted_through_sequence"`
	UpdatedAt              time.Time `gorm:"column:updated_at"`
}

func (RetentionWatermark) TableName() string { return "realtime_event_retention_watermarks" }

func (e Event) Envelope(registry *EventRegistry) (infrarealtime.Envelope, error) {
	if registry == nil {
		registry = DefaultRegistry()
	}
	if strings.TrimSpace(e.Durability) != string(infrarealtime.Durable) {
		return infrarealtime.Envelope{}, fmt.Errorf("%w: persisted event %s is %s", ErrEventDurabilityInvalid, e.EventID, e.Durability)
	}
	requestID := ""
	if e.RequestID != nil {
		requestID = *e.RequestID
	}
	return registry.NewDurable(e.EventID, e.EventType, requestID, e.Sequence, json.RawMessage(e.PayloadJSON), e.OccurredAt)
}

type AppendInput struct {
	EventID    string
	Type       string
	RequestID  string
	UserID     int64
	Payload    any
	OccurredAt time.Time
}

type ResumeQuery struct {
	UserID        int64
	AfterSequence uint64
	Limit         int
	Now           time.Time
}

type ResumeResult struct {
	Events                  []Event
	ResyncRequired          bool
	OldestAvailableSequence uint64
	LatestSequence          uint64
}

type CleanupResult struct {
	Deleted int64
	Targets int
}

type TransactionalEventAppender interface {
	AppendTx(context.Context, *gorm.DB, AppendInput) (*Event, error)
}

type TransactionalEventSink interface {
	TransactionalEventAppender
	PublishBestEffort(context.Context, *Event)
}

type GormRepository struct {
	db       *gorm.DB
	registry *EventRegistry
}

func NewGormRepository(client *database.Client, registries ...*EventRegistry) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	registry := DefaultRegistry()
	if len(registries) > 0 && registries[0] != nil {
		registry = registries[0]
	}
	return &GormRepository{db: client.Gorm, registry: registry}
}

func (r *GormRepository) Append(ctx context.Context, input AppendInput) (*Event, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	return r.append(ctx, r.db.WithContext(ctx), input)
}

func (r *GormRepository) AppendTx(ctx context.Context, tx *gorm.DB, input AppendInput) (*Event, error) {
	if r == nil || r.db == nil || tx == nil {
		return nil, ErrRepositoryNotConfigured
	}
	return r.append(ctx, tx.WithContext(ctx), input)
}

func (r *GormRepository) append(ctx context.Context, db *gorm.DB, input AppendInput) (*Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	input.Type = strings.TrimSpace(input.Type)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.EventID = strings.TrimSpace(input.EventID)
	if input.UserID <= 0 || input.Type == "" || input.OccurredAt.IsZero() || utf8.RuneCountInString(input.RequestID) > 128 {
		return nil, ErrAppendInputInvalid
	}
	definition, ok := r.registry.Definition(input.Type)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownEventType, input.Type)
	}
	if definition.Durability != infrarealtime.Durable {
		return nil, fmt.Errorf("%w: %s", ErrEventDurabilityInvalid, input.Type)
	}
	payload, err := r.registry.EncodePayload(input.Type, input.Payload)
	if err != nil {
		return nil, err
	}
	if input.EventID == "" {
		input.EventID, err = infrarealtime.NewEventID(input.OccurredAt)
		if err != nil {
			return nil, err
		}
	} else if !infrarealtime.ValidEventID(input.EventID) {
		return nil, ErrAppendInputInvalid
	}
	var requestID *string
	if input.RequestID != "" {
		requestID = &input.RequestID
	}
	expiresAt := input.OccurredAt.Add(DurableEventRetention)
	event := Event{
		EventID: input.EventID, EventType: input.Type, RequestID: requestID,
		TargetType: TargetTypeUser, TargetID: strconv.FormatInt(input.UserID, 10),
		Durability: string(infrarealtime.Durable), PayloadJSON: string(payload),
		OccurredAt: input.OccurredAt, ExpiresAt: &expiresAt,
	}
	if err := db.Create(&event).Error; err != nil {
		var mysqlErr *mysqldriver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, errors.Join(ErrDuplicateEventID, err)
		}
		return nil, err
	}
	return &event, nil
}

func (r *GormRepository) ResumeUser(ctx context.Context, query ResumeQuery) (*ResumeResult, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if query.UserID <= 0 {
		return nil, ErrResumeQueryInvalid
	}
	if query.Limit <= 0 || query.Limit > MaxResumeLimit {
		query.Limit = MaxResumeLimit
	}
	targetID := strconv.FormatInt(query.UserID, 10)
	scoped := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&Event{}).
			Where("target_type = ? AND target_id = ? AND durability = ?", TargetTypeUser, targetID, infrarealtime.Durable)
	}

	result := &ResumeResult{Events: []Event{}}
	var latest struct {
		Value uint64 `gorm:"column:value"`
	}
	if err := scoped().Select("COALESCE(MAX(sequence), 0) AS value").Scan(&latest).Error; err != nil {
		return nil, err
	}
	var watermark RetentionWatermark
	err := r.db.WithContext(ctx).
		Where("target_type = ? AND target_id = ?", TargetTypeUser, targetID).
		First(&watermark).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	result.LatestSequence = max(latest.Value, watermark.DeletedThroughSequence)
	if query.AfterSequence < watermark.DeletedThroughSequence {
		result.ResyncRequired = true
		return result, nil
	}
	if err := scoped().Where("sequence > ?", query.AfterSequence).
		Order("sequence asc").Limit(query.Limit + 1).Find(&result.Events).Error; err != nil {
		return nil, err
	}
	if len(result.Events) > query.Limit {
		result.Events = []Event{}
		result.ResyncRequired = true
	}
	return result, nil
}

func (r *GormRepository) CleanupExpired(ctx context.Context, now time.Time, limit int) (CleanupResult, error) {
	if r == nil || r.db == nil {
		return CleanupResult{}, ErrRepositoryNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limit <= 0 {
		limit = DefaultCleanupBatchSize
	}
	if limit > MaxCleanupBatchSize {
		limit = MaxCleanupBatchSize
	}

	result := CleanupResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var expired []Event
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Select("sequence", "target_type", "target_id").
			Where("expires_at <= ?", now).
			Order("sequence ASC").Limit(limit).Find(&expired).Error; err != nil {
			return err
		}
		if len(expired) == 0 {
			return nil
		}

		type targetKey struct{ targetType, targetID string }
		through := make(map[targetKey]uint64)
		sequences := make([]uint64, 0, len(expired))
		for _, event := range expired {
			key := targetKey{targetType: event.TargetType, targetID: event.TargetID}
			if event.Sequence > through[key] {
				through[key] = event.Sequence
			}
			sequences = append(sequences, event.Sequence)
		}
		keys := make([]targetKey, 0, len(through))
		for key := range through {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(left, right int) bool {
			if keys[left].targetType == keys[right].targetType {
				return keys[left].targetID < keys[right].targetID
			}
			return keys[left].targetType < keys[right].targetType
		})
		for _, key := range keys {
			if strings.TrimSpace(key.targetType) == "" || strings.TrimSpace(key.targetID) == "" {
				return ErrAppendInputInvalid
			}
			if err := tx.Exec(`INSERT INTO realtime_event_retention_watermarks
(target_type,target_id,deleted_through_sequence,updated_at)
VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE
deleted_through_sequence=GREATEST(deleted_through_sequence,VALUES(deleted_through_sequence)),
updated_at=VALUES(updated_at)`, key.targetType, key.targetID, through[key], now).Error; err != nil {
				return err
			}
		}

		deleted := tx.Where("sequence IN ?", sequences).Delete(&Event{})
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected != int64(len(sequences)) {
			return fmt.Errorf("realtime cleanup deleted %d of %d selected events", deleted.RowsAffected, len(sequences))
		}
		result.Deleted = deleted.RowsAffected
		result.Targets = len(keys)
		return nil
	})
	return result, err
}

var _ TransactionalEventAppender = (*GormRepository)(nil)

type DurableEventSink struct {
	repository *GormRepository
	publisher  infrarealtime.Publisher
	logger     *slog.Logger
}

func NewDurableEventSink(repository *GormRepository, publisher infrarealtime.Publisher, logger *slog.Logger) *DurableEventSink {
	if logger == nil {
		logger = slog.Default()
	}
	return &DurableEventSink{repository: repository, publisher: publisher, logger: logger}
}

func (s *DurableEventSink) AppendTx(ctx context.Context, tx *gorm.DB, input AppendInput) (*Event, error) {
	if s == nil || s.repository == nil {
		return nil, ErrRepositoryNotConfigured
	}
	return s.repository.AppendTx(ctx, tx, input)
}

func (s *DurableEventSink) PublishBestEffort(ctx context.Context, event *Event) {
	if s == nil || s.publisher == nil || event == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	envelope, err := event.Envelope(s.repository.registry)
	if err != nil {
		s.logger.WarnContext(context.WithoutCancel(ctx), "failed to build durable realtime envelope", "event_id", event.EventID, "error", err)
		return
	}
	if event.TargetType != TargetTypeUser {
		s.logger.WarnContext(context.WithoutCancel(ctx), "unsupported durable realtime target", "event_id", event.EventID, "target_type", event.TargetType)
		return
	}
	userID, err := strconv.ParseInt(event.TargetID, 10, 64)
	if err != nil || userID <= 0 {
		s.logger.WarnContext(context.WithoutCancel(ctx), "invalid durable realtime user target", "event_id", event.EventID)
		return
	}
	if err := s.publisher.Publish(context.WithoutCancel(ctx), infrarealtime.Publication{Platform: "admin", UserID: userID, Envelope: envelope}); err != nil {
		s.logger.WarnContext(context.WithoutCancel(ctx), "durable realtime publish deferred to cursor resume", "event_id", event.EventID, "error", err)
	}
}

var _ TransactionalEventSink = (*DurableEventSink)(nil)
