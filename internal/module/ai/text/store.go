package aitext

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/module/ai/requestidentity"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"

	KindText      = "text"
	KindToolDraft = "tool_draft"
)

var (
	ErrStoreNotConfigured = errors.New("aitext store not configured")
	ErrAcceptInputInvalid = errors.New("aitext durable accept input is invalid")
	ErrCandidateConflict  = errors.New("aitext result candidate conflicts with persisted candidate")
)

type DurableStore interface {
	Accept(context.Context, AcceptInput) (*TextTask, error)
	FindByID(context.Context, uint64) (*TextTask, error)
	FindReplay(context.Context, int64, string) (*ReplayRecord, error)
}

type TextTask struct {
	ID                    uint64     `gorm:"column:id;primaryKey"`
	Platform              string     `gorm:"column:platform"`
	UserID                int64      `gorm:"column:user_id"`
	RequestID             string     `gorm:"column:request_id"`
	RequestFingerprint    []byte     `gorm:"column:request_fingerprint"`
	RequestIdentityStatus string     `gorm:"column:request_identity_status"`
	RequestIdentityMarker string     `gorm:"column:request_identity_marker"`
	RunID                 int64      `gorm:"column:run_id"`
	Kind                  string     `gorm:"column:kind"`
	AgentID               uint64     `gorm:"column:agent_id"`
	ProviderID            uint64     `gorm:"column:provider_id"`
	ModelID               string     `gorm:"column:model_id"`
	Prompt                string     `gorm:"column:prompt"`
	Answer                *string    `gorm:"column:answer"`
	Status                string     `gorm:"column:status"`
	ErrorMessage          *string    `gorm:"column:error_message"`
	LastErrorCode         string     `gorm:"column:last_error_code"`
	StartedAt             *time.Time `gorm:"column:started_at"`
	FinishedAt            *time.Time `gorm:"column:finished_at"`
	ElapsedMS             uint       `gorm:"column:elapsed_ms"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at"`
	PromptTokens          uint       `gorm:"column:run_prompt_tokens;->"`
	CompletionTokens      uint       `gorm:"column:run_completion_tokens;->"`
	TotalTokens           uint       `gorm:"column:run_total_tokens;->"`
	RunBillingStatus      string     `gorm:"column:run_billing_status;->"`
	RunBillingReason      string     `gorm:"column:run_billing_reason;->"`
}

func (TextTask) TableName() string { return "ai_text_tasks" }

type AcceptInput struct {
	Platform                 string
	UserID                   int64
	RequestID                string
	RequestFingerprint       [sha256.Size]byte
	Kind                     string
	AgentID                  uint64
	ProviderID               uint64
	ModelID                  string
	ModelDisplayName         string
	Prompt                   string
	InputSnapshot            string
	PricingSnapshotJSON      string
	EffectiveMaxOutputTokens int64
	AcceptedAt               time.Time
}

type Execution struct {
	Task                TextTask
	InputSnapshot       string
	PricingSnapshotJSON string
	BillingStatus       string
	BillingReason       string
	EngineType          string
	EngineBaseURL       string
	EngineAPIKeyEnc     string
	EngineAPIProtocol   string
}

type ReplayRecord struct {
	Task          TextTask
	InputSnapshot string
}

type replayRow struct {
	TextTask
	InputSnapshot            string `gorm:"column:input_snapshot"`
	RunRequestFingerprint    []byte `gorm:"column:run_request_fingerprint"`
	RunRequestIdentityStatus string `gorm:"column:run_request_identity_status"`
	RunRequestIdentityMarker string `gorm:"column:run_request_identity_marker"`
	RunAgentID               int64  `gorm:"column:run_agent_id"`
	RunProviderID            int64  `gorm:"column:run_provider_id"`
	RunModelID               string `gorm:"column:run_model_id"`
}

type canonicalRunRow struct {
	ID                    int64  `gorm:"column:id"`
	RequestFingerprint    []byte `gorm:"column:request_fingerprint"`
	RequestIdentityStatus string `gorm:"column:request_identity_status"`
	RequestIdentityMarker string `gorm:"column:request_identity_marker"`
}

type GormStore struct {
	db *gorm.DB
}

func NewGormStore(client *database.Client) *GormStore {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormStore{db: client.Gorm}
}

func (s *GormStore) Accept(ctx context.Context, input AcceptInput) (*TextTask, error) {
	if s == nil || s.db == nil {
		return nil, ErrStoreNotConfigured
	}
	input = normalizeAcceptInput(input)
	if err := validateAcceptInput(input); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	acceptedAt := input.AcceptedAt
	if acceptedAt.IsZero() {
		acceptedAt = time.Now()
	}
	var accepted *TextTask
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var canonical canonicalRunRow
		canonicalErr := canonicalRunLookupDB(tx, input.UserID, input.RequestID).Take(&canonical).Error
		if canonicalErr == nil {
			if err := compareCanonicalFingerprint(canonical, input.RequestFingerprint); err != nil {
				return err
			}
			var existing TextTask
			if err := tx.Where("run_id = ? AND user_id = ? AND request_id = ?", canonical.ID, input.UserID, input.RequestID).First(&existing).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return requestidentity.ErrRequestIdentityConflict
				}
				return err
			}
			locked, err := lockAcceptedTaskGraph(tx, existing.ID, input.UserID, input.RequestID)
			if err != nil {
				return err
			}
			accepted = cloneTask(&locked)
			return nil
		}
		if !errors.Is(canonicalErr, gorm.ErrRecordNotFound) {
			return canonicalErr
		}

		var existing TextTask
		err := tx.Where("user_id = ? AND request_id = ?", input.UserID, input.RequestID).
			First(&existing).Error
		if err == nil {
			if err := compareFingerprint(existing, input.RequestFingerprint); err != nil {
				return err
			}
			locked, err := lockAcceptedTaskGraph(tx, existing.ID, input.UserID, input.RequestID)
			if err != nil {
				return err
			}
			if err := compareFingerprint(locked, input.RequestFingerprint); err != nil {
				return err
			}
			accepted = cloneTask(&locked)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		key := textIdempotencyKey(input.UserID, input.RequestID)
		run := airun.Run{
			Platform:              input.Platform,
			RequestID:             input.RequestID,
			RequestFingerprint:    append([]byte(nil), input.RequestFingerprint[:]...),
			RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable),
			RequestIdentityMarker: "",
			IdempotencyKey:        &key,
			UserID:                input.UserID,
			AgentID:               int64(input.AgentID),
			ProviderID:            int64(input.ProviderID),
			ModelID:               input.ModelID,
			ModelDisplayName:      input.ModelDisplayName,
			InputSnapshot:         input.InputSnapshot,
			PricingSnapshotJSON:   input.PricingSnapshotJSON,
			Status:                enum.AIRunStatusRunning,
			BillingStatus:         string(billing.BillingStatusPending),
			BillingReason:         string(billing.BillingReasonPending),
			StartedAt:             &acceptedAt,
			CreatedAt:             acceptedAt,
			UpdatedAt:             acceptedAt,
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		if err := tx.Create(&airun.RunEvent{
			RunID: run.ID, Seq: 1, EventType: enum.AIRunEventStart,
			Message: enum.AIRunEventLabels[enum.AIRunEventStart], CreatedAt: acceptedAt,
		}).Error; err != nil {
			return err
		}
		snapshot, err := aigateway.ParsePricingSnapshot(input.PricingSnapshotJSON)
		if err != nil {
			return err
		}
		charge := billing.UsageCharge{
			RunID: run.ID, UserID: input.UserID, Currency: "CNY",
			PricingVersion: snapshot.Version, MultiplierPPM: snapshot.MultiplierPPM,
			Status: billing.ChargeStatusOpen, CreatedAt: acceptedAt, UpdatedAt: acceptedAt,
		}
		if err := tx.Create(&charge).Error; err != nil {
			return err
		}
		task := TextTask{
			Platform: input.Platform, UserID: input.UserID, RequestID: input.RequestID,
			RequestFingerprint:    append([]byte(nil), input.RequestFingerprint[:]...),
			RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable), RequestIdentityMarker: "",
			RunID: run.ID, Kind: input.Kind, AgentID: input.AgentID, ProviderID: input.ProviderID,
			ModelID: input.ModelID, Prompt: input.Prompt, Status: StatusRunning, LastErrorCode: "",
			StartedAt: &acceptedAt, CreatedAt: acceptedAt, UpdatedAt: acceptedAt,
		}
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		accepted = cloneTask(&task)
		return nil
	})
	if err == nil {
		return accepted, nil
	}
	replay, replayErr := s.loadAcceptedReplay(ctx, input.UserID, input.RequestID)
	if replayErr == nil && replay != nil {
		if compareErr := compareFingerprint(*replay, input.RequestFingerprint); compareErr != nil {
			return nil, compareErr
		}
		return replay, nil
	}
	if replayErr != nil && !errors.Is(replayErr, gorm.ErrRecordNotFound) {
		return nil, replayErr
	}
	var canonical canonicalRunRow
	if canonicalErr := canonicalRunLookupDB(s.db.WithContext(ctx), input.UserID, input.RequestID).Take(&canonical).Error; canonicalErr == nil {
		if compareErr := compareCanonicalFingerprint(canonical, input.RequestFingerprint); compareErr != nil {
			return nil, compareErr
		}
		return nil, requestidentity.ErrRequestIdentityConflict
	} else if !errors.Is(canonicalErr, gorm.ErrRecordNotFound) {
		return nil, canonicalErr
	}
	return nil, err
}

func (s *GormStore) FindByID(ctx context.Context, taskID uint64) (*TextTask, error) {
	if s == nil || s.db == nil {
		return nil, ErrStoreNotConfigured
	}
	if taskID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var task TextTask
	err := s.db.WithContext(ctx).Table("ai_text_tasks AS t").
		Select("t.*, r.prompt_tokens AS run_prompt_tokens, r.completion_tokens AS run_completion_tokens, r.total_tokens AS run_total_tokens, r.billing_status AS run_billing_status, r.billing_reason AS run_billing_reason").
		Joins("JOIN ai_runs r ON r.id = t.run_id").Where("t.id = ?", taskID).Take(&task).Error
	if err != nil {
		return nil, err
	}
	return cloneTask(&task), nil
}

func (s *GormStore) FindReplay(ctx context.Context, userID int64, requestID string) (*ReplayRecord, error) {
	if s == nil || s.db == nil {
		return nil, ErrStoreNotConfigured
	}
	requestID = strings.TrimSpace(requestID)
	if userID <= 0 || requestID == "" {
		return nil, ErrAcceptInputInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var row replayRow
	if err := replayLookupDB(s.db.WithContext(ctx), userID, requestID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	task := row.TextTask
	if task.ID == 0 || task.RunID <= 0 || task.UserID != userID || task.RequestID != requestID ||
		len(task.RequestFingerprint) != sha256.Size || len(row.RunRequestFingerprint) != sha256.Size ||
		!bytes.Equal(task.RequestFingerprint, row.RunRequestFingerprint) ||
		task.RequestIdentityStatus != row.RunRequestIdentityStatus || task.RequestIdentityMarker != row.RunRequestIdentityMarker ||
		row.RunAgentID <= 0 || uint64(row.RunAgentID) != task.AgentID || row.RunProviderID <= 0 || uint64(row.RunProviderID) != task.ProviderID ||
		strings.TrimSpace(row.RunModelID) != strings.TrimSpace(task.ModelID) || strings.TrimSpace(row.InputSnapshot) == "" {
		return nil, ErrAcceptInputInvalid
	}
	return &ReplayRecord{Task: *cloneTask(&task), InputSnapshot: strings.TrimSpace(row.InputSnapshot)}, nil
}

func replayLookupDB(db *gorm.DB, userID int64, requestID string) *gorm.DB {
	return db.Table("ai_text_tasks AS t").
		Select(`t.*, r.input_snapshot,
			r.request_fingerprint AS run_request_fingerprint,
			r.request_identity_status AS run_request_identity_status,
			r.request_identity_marker AS run_request_identity_marker,
			r.agent_id AS run_agent_id, r.provider_id AS run_provider_id, r.model_id AS run_model_id,
			r.prompt_tokens AS run_prompt_tokens, r.completion_tokens AS run_completion_tokens,
			r.total_tokens AS run_total_tokens, r.billing_status AS run_billing_status,
			r.billing_reason AS run_billing_reason`).
		Joins("JOIN ai_runs r ON r.id = t.run_id AND r.user_id = t.user_id AND r.request_id = t.request_id").
		Where("t.user_id = ? AND t.request_id = ?", userID, requestID)
}

func canonicalRunLookupDB(db *gorm.DB, userID int64, requestID string) *gorm.DB {
	return db.Table("ai_runs").
		Select("id, request_fingerprint, request_identity_status, request_identity_marker").
		Where("user_id = ? AND request_id = ?", userID, requestID)
}

func (s *GormStore) FindPending(ctx context.Context, limit int) ([]TextTask, error) {
	if s == nil || s.db == nil {
		return nil, ErrStoreNotConfigured
	}
	if limit <= 0 {
		limit = 25
	}
	var tasks []TextTask
	if err := pendingTasksDB(s.db.WithContext(ctx), limit).Find(&tasks).Error; err != nil {
		return nil, err
	}
	for index := range tasks {
		tasks[index].RequestFingerprint = append([]byte(nil), tasks[index].RequestFingerprint...)
	}
	return tasks, nil
}

func pendingTasksDB(db *gorm.DB, limit int) *gorm.DB {
	if limit <= 0 {
		limit = 25
	}
	return db.Table("ai_text_tasks AS t").Select("t.*").
		Joins("JOIN ai_runs r ON r.id = t.run_id").
		Where("t.status = ? AND r.status = ? AND r.billing_status IN ?", StatusRunning, enum.AIRunStatusRunning, []billing.BillingStatus{billing.BillingStatusPending, billing.BillingStatusHeld}).
		Where("NOT EXISTS (SELECT 1 FROM ai_provider_attempts a WHERE a.run_id = t.run_id AND a.state = ? AND a.dispatched_at IS NOT NULL AND a.dispatched_at > ?)", "dispatched", time.Now().Add(-GenerateTimeout)).
		Order("t.created_at ASC, t.id ASC").Limit(limit)
}

func (s *GormStore) LoadExecution(ctx context.Context, taskID uint64) (*Execution, error) {
	if s == nil || s.db == nil {
		return nil, ErrStoreNotConfigured
	}
	if taskID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var row struct {
		TextTask
		InputSnapshot       string `gorm:"column:input_snapshot"`
		PricingSnapshotJSON string `gorm:"column:pricing_snapshot_json"`
		BillingStatus       string `gorm:"column:billing_status"`
		BillingReason       string `gorm:"column:billing_reason"`
		EngineType          string `gorm:"column:engine_type"`
		EngineBaseURL       string `gorm:"column:engine_base_url"`
		EngineAPIKeyEnc     string `gorm:"column:engine_api_key_enc"`
		EngineAPIProtocol   string `gorm:"column:engine_api_protocol"`
	}
	err := s.db.WithContext(ctx).Table("ai_text_tasks AS t").
		Select(`t.*, r.input_snapshot, r.pricing_snapshot_json, r.billing_status, r.billing_reason,
			p.engine_type, p.base_url AS engine_base_url, p.api_key_enc AS engine_api_key_enc,
			p.api_protocol AS engine_api_protocol`).
		Joins("JOIN ai_runs r ON r.id = t.run_id AND r.user_id = t.user_id AND r.request_id = t.request_id").
		Joins("JOIN ai_providers p ON p.id = t.provider_id").
		Where("t.id = ?", taskID).Take(&row).Error
	if err != nil {
		return nil, err
	}
	return &Execution{
		Task: row.TextTask, InputSnapshot: row.InputSnapshot, PricingSnapshotJSON: row.PricingSnapshotJSON,
		BillingStatus: row.BillingStatus, BillingReason: row.BillingReason,
		EngineType: row.EngineType, EngineBaseURL: row.EngineBaseURL, EngineAPIKeyEnc: row.EngineAPIKeyEnc,
		EngineAPIProtocol: row.EngineAPIProtocol,
	}, nil
}

func (s *GormStore) SaveCandidate(ctx context.Context, taskID uint64, runID int64, answer string) error {
	if s == nil || s.db == nil {
		return ErrStoreNotConfigured
	}
	answer = strings.TrimSpace(answer)
	if taskID == 0 || runID <= 0 || answer == "" {
		return ErrAcceptInputInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task TextTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND run_id = ?", taskID, runID).First(&task).Error; err != nil {
			return err
		}
		if task.Answer != nil {
			if *task.Answer != answer {
				return ErrCandidateConflict
			}
			return nil
		}
		if task.Status != StatusRunning {
			return ErrCandidateConflict
		}
		return tx.Model(&TextTask{}).Where("id = ? AND run_id = ? AND status = ? AND answer IS NULL", taskID, runID, StatusRunning).
			Update("answer", answer).Error
	})
}

func (s *GormStore) MarkExecutionFailure(ctx context.Context, taskID uint64, runID int64, code string, message string) error {
	if s == nil || s.db == nil {
		return ErrStoreNotConfigured
	}
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if taskID == 0 || runID <= 0 || code == "" || message == "" {
		return ErrAcceptInputInvalid
	}
	result := s.db.WithContext(ctx).Model(&TextTask{}).
		Where("id = ? AND run_id = ? AND status = ?", taskID, runID, StatusRunning).
		Updates(map[string]any{"last_error_code": code, "error_message": message, "updated_at": time.Now()})
	return result.Error
}

func (s *GormStore) loadAcceptedReplay(ctx context.Context, userID int64, requestID string) (*TextTask, error) {
	var accepted *TextTask
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task TextTask
		if err := tx.Where("user_id = ? AND request_id = ?", userID, requestID).First(&task).Error; err != nil {
			return err
		}
		locked, err := lockAcceptedTaskGraph(tx, task.ID, userID, requestID)
		if err != nil {
			return err
		}
		accepted = cloneTask(&locked)
		return nil
	})
	return accepted, err
}

func normalizeAcceptInput(input AcceptInput) AcceptInput {
	input.Platform = strings.TrimSpace(input.Platform)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.Kind = strings.TrimSpace(input.Kind)
	input.ModelID = strings.TrimSpace(input.ModelID)
	input.ModelDisplayName = strings.TrimSpace(input.ModelDisplayName)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.InputSnapshot = strings.TrimSpace(input.InputSnapshot)
	input.PricingSnapshotJSON = strings.TrimSpace(input.PricingSnapshotJSON)
	return input
}

func validateAcceptInput(input AcceptInput) error {
	if !enum.IsRegisteredPlatform(input.Platform) || input.UserID <= 0 || input.RequestID == "" || utf8.RuneCountInString(input.RequestID) > 128 ||
		input.RequestFingerprint == ([sha256.Size]byte{}) || (input.Kind != KindText && input.Kind != KindToolDraft) ||
		input.AgentID == 0 || input.ProviderID == 0 || input.ModelID == "" || input.Prompt == "" || input.InputSnapshot == "" ||
		input.PricingSnapshotJSON == "" || input.EffectiveMaxOutputTokens <= 0 {
		return ErrAcceptInputInvalid
	}
	snapshot, err := aigateway.ParsePricingSnapshot(input.PricingSnapshotJSON)
	if err != nil || snapshot.RequestedModelID != input.ModelID || int64(snapshot.EffectiveMaxOutputTokens) != input.EffectiveMaxOutputTokens || snapshot.MultiplierPPM <= 0 {
		return fmt.Errorf("%w: immutable pricing snapshot is invalid", ErrAcceptInputInvalid)
	}
	return nil
}

func lockAcceptedTaskGraph(tx *gorm.DB, taskID uint64, userID int64, requestID string) (TextTask, error) {
	if tx == nil || taskID == 0 || userID <= 0 || strings.TrimSpace(requestID) == "" {
		return TextTask{}, ErrAcceptInputInvalid
	}
	var located TextTask
	if err := tx.Where("id = ? AND user_id = ? AND request_id = ?", taskID, userID, requestID).First(&located).Error; err != nil {
		return TextTask{}, err
	}
	var run airun.Run
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ? AND request_id = ?", located.RunID, userID, requestID).First(&run).Error; err != nil {
		return TextTask{}, err
	}
	var charge billing.UsageCharge
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ? AND user_id = ?", run.ID, userID).First(&charge).Error; err != nil {
		return TextTask{}, err
	}
	var task TextTask
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND run_id = ? AND user_id = ? AND request_id = ?", taskID, run.ID, userID, requestID).First(&task).Error; err != nil {
		return TextTask{}, err
	}
	if task.ID == 0 || task.RunID <= 0 || task.RequestIdentityStatus != string(requestidentity.IdentityStatusReplayable) || task.RequestIdentityMarker != "" ||
		run.RequestIdentityStatus != string(requestidentity.IdentityStatusReplayable) || run.RequestIdentityMarker != "" ||
		len(task.RequestFingerprint) != sha256.Size || len(run.RequestFingerprint) != sha256.Size || !bytes.Equal(task.RequestFingerprint, run.RequestFingerprint) ||
		int64(task.AgentID) != run.AgentID || int64(task.ProviderID) != run.ProviderID || task.ModelID != run.ModelID {
		return TextTask{}, ErrAcceptInputInvalid
	}
	snapshot, err := aigateway.ParsePricingSnapshot(run.PricingSnapshotJSON)
	if err != nil || snapshot.RequestedModelID != run.ModelID || snapshot.MultiplierPPM <= 0 || snapshot.EffectiveMaxOutputTokens <= 0 ||
		charge.RunID != run.ID || charge.UserID != run.UserID || charge.PricingVersion != snapshot.Version || charge.MultiplierPPM != snapshot.MultiplierPPM {
		return TextTask{}, ErrAcceptInputInvalid
	}
	return task, nil
}

func compareFingerprint(task TextTask, incoming [sha256.Size]byte) error {
	if len(task.RequestFingerprint) != sha256.Size {
		return requestidentity.ErrRequestIdentityNotReplayable
	}
	var stored [sha256.Size]byte
	copy(stored[:], task.RequestFingerprint)
	return requestidentity.CompareForReplay(requestidentity.IdentityStatus(task.RequestIdentityStatus), stored, incoming)
}

func compareCanonicalFingerprint(run canonicalRunRow, incoming [sha256.Size]byte) error {
	if run.ID <= 0 || len(run.RequestFingerprint) != sha256.Size || strings.TrimSpace(run.RequestIdentityMarker) != "" {
		return requestidentity.ErrRequestIdentityNotReplayable
	}
	var stored [sha256.Size]byte
	copy(stored[:], run.RequestFingerprint)
	return requestidentity.CompareForReplay(requestidentity.IdentityStatus(run.RequestIdentityStatus), stored, incoming)
}

func textIdempotencyKey(userID int64, requestID string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("ai-text:%d:%s", userID, requestID)))
	return hex.EncodeToString(digest[:])
}

func cloneTask(task *TextTask) *TextTask {
	if task == nil {
		return nil
	}
	copy := *task
	copy.RequestFingerprint = append([]byte(nil), task.RequestFingerprint...)
	if task.Answer != nil {
		answer := *task.Answer
		copy.Answer = &answer
	}
	if task.ErrorMessage != nil {
		message := *task.ErrorMessage
		copy.ErrorMessage = &message
	}
	return &copy
}
