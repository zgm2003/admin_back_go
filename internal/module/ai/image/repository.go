package aiimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/requestidentity"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrRepositoryNotConfigured = errors.New("aiimage repository not configured")

type canonicalImageRunRow struct {
	ID                    int64  `gorm:"column:id"`
	RequestFingerprint    []byte `gorm:"column:request_fingerprint"`
	RequestIdentityStatus string `gorm:"column:request_identity_status"`
	RequestIdentityMarker string `gorm:"column:request_identity_marker"`
	InputSnapshot         string `gorm:"column:input_snapshot"`
	PricingSnapshotJSON   string `gorm:"column:pricing_snapshot_json"`
}

type AcceptedTaskReplay struct {
	Task                ImageTask
	InputSnapshot       string
	PricingSnapshotJSON string
}

type Repository interface {
	ListImageAgents(ctx context.Context, scene string) ([]AgentOption, error)
	ListTasks(ctx context.Context, userID uint64, query ListQuery) ([]ImageTask, int64, error)
	GetTask(ctx context.Context, userID uint64, taskID uint64, platform string) (*ImageTask, error)
	GetTaskForWorker(ctx context.Context, platform string, userID uint64, taskID uint64) (*ImageTask, error)
	LoadTaskFiles(ctx context.Context, taskID uint64) ([]ImageFile, error)
	CreateTaskWithFiles(ctx context.Context, task ImageTask, files TaskFileSet) (uint64, error)
	FindAcceptedTaskByRequestID(ctx context.Context, userID uint64, requestID string) (*AcceptedTaskReplay, error)
	AcceptTaskWithFiles(ctx context.Context, input AcceptTaskInput) (*ImageTask, error)
	DeleteTask(ctx context.Context, userID uint64, taskID uint64, platform string) error
	LoadAgentRuntime(ctx context.Context, agentID uint64) (*AgentRuntime, error)
	ClaimTask(ctx context.Context, platform string, userID uint64, taskID uint64, startedAt time.Time) (bool, error)
	ClaimTaskLease(ctx context.Context, platform string, userID uint64, taskID uint64, owner string, now time.Time, ttl time.Duration) (*TaskLease, error)
	RenewTaskLease(ctx context.Context, taskID uint64, owner string, token uint64, now time.Time, expiresAt time.Time) (bool, error)
	AppendTaskFiles(ctx context.Context, files []ImageFile) error
	FinishTaskSuccess(ctx context.Context, platform string, userID uint64, taskID uint64, actualParamsJSON *string, rawResponseJSON *string, elapsedMS int, finishedAt time.Time) error
	FinishTaskFailed(ctx context.Context, platform string, userID uint64, taskID uint64, message string, elapsedMS int, finishedAt time.Time) error
	LoadUploadConfig(ctx context.Context) (*UploadConfig, error)
}

type AcceptTaskInput struct {
	Task                  ImageTask
	Files                 TaskFileSet
	InputSnapshot         string
	PricingSnapshotJSON   string
	EffectiveOutputTokens int64
	AcceptedAt            time.Time
}

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) ListImageAgents(ctx context.Context, scene string) ([]AgentOption, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	scene = strings.TrimSpace(scene)
	if scene == "" {
		return nil, errors.New("image agent scene is required")
	}
	var rows []AgentOption
	err := r.db.WithContext(ctx).Table("ai_agents AS a").
		Select("a.id AS id, a.name AS name, a.avatar AS avatar").
		Joins("JOIN ai_providers AS p ON p.id = a.provider_id AND p.is_del = ? AND p.status = ?", enum.CommonNo, enum.CommonYes).
		Joins("JOIN ai_provider_models AS m ON m.provider_id = a.provider_id AND m.model_id = a.model_id AND m.status = ? AND m.mapping_status = ?", enum.CommonYes, officialmodel.MappingStatusMapped).
		Where("a.is_del = ? AND a.status = ?", enum.CommonNo, enum.CommonYes).
		Where("JSON_CONTAINS(a.scenes_json, JSON_QUOTE(?))", scene).
		Order("a.id DESC").
		Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) ListTasks(ctx context.Context, userID uint64, query ListQuery) ([]ImageTask, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.tasks(ctx).Where("platform = ? AND user_id = ?", strings.TrimSpace(query.Platform), userID)
	if strings.TrimSpace(query.Status) != "" {
		db = db.Where("status = ?", strings.TrimSpace(query.Status))
	}
	var total int64
	if err := db.Model(&ImageTask{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ImageTask
	err := db.Order("id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) GetTask(ctx context.Context, userID uint64, taskID uint64, platform string) (*ImageTask, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if userID == 0 || taskID == 0 {
		return nil, nil
	}
	var row ImageTask
	err := r.userTask(ctx, userID, taskID, platform).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) GetTaskForWorker(ctx context.Context, platform string, userID uint64, taskID uint64) (*ImageTask, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if userID == 0 || taskID == 0 {
		return nil, nil
	}
	var row ImageTask
	err := workerTaskDB(r.db.WithContext(ctx), platform, userID, taskID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) LoadTaskFiles(ctx context.Context, taskID uint64) ([]ImageFile, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []ImageFile
	err := r.db.WithContext(ctx).Where("task_id = ? AND is_del = ?", taskID, enum.CommonNo).Order("role ASC, sort_order ASC, id ASC").Find(&rows).Error
	if rows == nil {
		rows = []ImageFile{}
	}
	return rows, err
}

func (r *GormRepository) CreateTaskWithFiles(ctx context.Context, task ImageTask, files TaskFileSet) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	task.IsDel = enum.CommonNo
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		inputIDsBySort := make(map[int]uint64, len(files.Inputs))
		for i := range files.Inputs {
			files.Inputs[i].TaskID = task.ID
			files.Inputs[i].IsDel = enum.CommonNo
			if err := tx.Create(&files.Inputs[i]).Error; err != nil {
				return err
			}
			inputIDsBySort[files.Inputs[i].SortOrder] = files.Inputs[i].ID
		}
		if files.Mask != nil {
			mask := files.Mask.File
			mask.TaskID = task.ID
			mask.IsDel = enum.CommonNo
			if relatedID, ok := inputIDsBySort[files.Mask.RelatedSortOrder]; ok {
				mask.RelatedFileID = &relatedID
			}
			if err := tx.Create(&mask).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return task.ID, err
}

func (r *GormRepository) AcceptTaskWithFiles(ctx context.Context, input AcceptTaskInput) (*ImageTask, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	input.Task.RequestID = strings.TrimSpace(input.Task.RequestID)
	input.InputSnapshot = strings.TrimSpace(input.InputSnapshot)
	input.PricingSnapshotJSON = strings.TrimSpace(input.PricingSnapshotJSON)
	if input.Task.UserID == 0 || input.Task.RequestID == "" || len(input.Task.RequestFingerprint) != sha256.Size || input.Task.AgentID == 0 ||
		input.Task.ProviderIDSnapshot == 0 || strings.TrimSpace(input.Task.ModelIDSnapshot) == "" || input.InputSnapshot == "" ||
		input.PricingSnapshotJSON == "" || input.EffectiveOutputTokens <= 0 {
		return nil, errors.New("image durable accept input is invalid")
	}
	snapshot, err := aigateway.ParsePricingSnapshot(input.PricingSnapshotJSON)
	if err != nil || snapshot.RequestedModelID != input.Task.ModelIDSnapshot || int64(snapshot.EffectiveMaxOutputTokens) != input.EffectiveOutputTokens {
		return nil, errors.New("image pricing snapshot is invalid")
	}
	acceptedAt := input.AcceptedAt
	if acceptedAt.IsZero() {
		acceptedAt = time.Now()
	}
	var accepted *ImageTask
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var canonical canonicalImageRunRow
		canonicalErr := canonicalImageRunLookupDB(tx, int64(input.Task.UserID), input.Task.RequestID).Take(&canonical).Error
		if canonicalErr == nil {
			if err := compareCanonicalImageFingerprint(canonical, input.Task.RequestFingerprint); err != nil {
				return err
			}
			var existing ImageTask
			if err := tx.Where("run_id = ? AND user_id = ? AND request_id = ?", canonical.ID, input.Task.UserID, input.Task.RequestID).First(&existing).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return requestidentity.ErrRequestIdentityConflict
				}
				return err
			}
			locked, err := lockAcceptedImageGraph(tx, existing.ID, int64(input.Task.UserID), input.Task.RequestID)
			if err != nil {
				return err
			}
			accepted = cloneImageTask(&locked)
			return nil
		}
		if !errors.Is(canonicalErr, gorm.ErrRecordNotFound) {
			return canonicalErr
		}

		var existing ImageTask
		queryErr := tx.Where("user_id = ? AND request_id = ?", input.Task.UserID, input.Task.RequestID).First(&existing).Error
		if queryErr == nil {
			if err := compareImageFingerprint(existing, input.Task.RequestFingerprint); err != nil {
				return err
			}
			locked, err := lockAcceptedImageGraph(tx, existing.ID, int64(input.Task.UserID), input.Task.RequestID)
			if err != nil {
				return err
			}
			if err := compareImageFingerprint(locked, input.Task.RequestFingerprint); err != nil {
				return err
			}
			accepted = cloneImageTask(&locked)
			return nil
		}
		if !errors.Is(queryErr, gorm.ErrRecordNotFound) {
			return queryErr
		}
		keyDigest := sha256.Sum256([]byte(fmt.Sprintf("ai-image:%d:%s", input.Task.UserID, input.Task.RequestID)))
		key := hex.EncodeToString(keyDigest[:])
		run := airun.Run{
			Platform: input.Task.Platform, RequestID: input.Task.RequestID, RequestFingerprint: append([]byte(nil), input.Task.RequestFingerprint...),
			RequestIdentityStatus: string(requestidentity.IdentityStatusReplayable), RequestIdentityMarker: "", IdempotencyKey: &key,
			UserID: int64(input.Task.UserID), AgentID: int64(input.Task.AgentID), ProviderID: int64(input.Task.ProviderIDSnapshot),
			ModelID: input.Task.ModelIDSnapshot, ModelDisplayName: input.Task.ModelDisplayNameSnapshot, InputSnapshot: input.InputSnapshot,
			PricingSnapshotJSON: input.PricingSnapshotJSON, Status: enum.AIRunStatusRunning, BillingStatus: string(billing.BillingStatusPending),
			BillingReason: string(billing.BillingReasonPending), StartedAt: &acceptedAt, CreatedAt: acceptedAt, UpdatedAt: acceptedAt,
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		if err := tx.Create(&airun.RunEvent{RunID: run.ID, Seq: 1, EventType: enum.AIRunEventStart, Message: enum.AIRunEventLabels[enum.AIRunEventStart], CreatedAt: acceptedAt}).Error; err != nil {
			return err
		}
		if err := tx.Create(&billing.UsageCharge{RunID: run.ID, UserID: run.UserID, Currency: "CNY", PricingVersion: snapshot.Version, MultiplierPPM: snapshot.MultiplierPPM, Status: billing.ChargeStatusOpen, CreatedAt: acceptedAt, UpdatedAt: acceptedAt}).Error; err != nil {
			return err
		}
		task := input.Task
		task.ID = 0
		task.RunID = run.ID
		task.RequestIdentityStatus = string(requestidentity.IdentityStatusReplayable)
		task.RequestIdentityMarker = ""
		task.LastErrorCode = ""
		task.IsDel = enum.CommonNo
		task.CreatedAt = acceptedAt
		task.UpdatedAt = acceptedAt
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		if err := createImageTaskFiles(tx, task.ID, input.Files); err != nil {
			return err
		}
		accepted = cloneImageTask(&task)
		return nil
	})
	if err == nil {
		return accepted, nil
	}
	var replay ImageTask
	if replayErr := r.db.WithContext(ctx).Where("user_id = ? AND request_id = ?", input.Task.UserID, input.Task.RequestID).First(&replay).Error; replayErr == nil {
		if compareErr := compareImageFingerprint(replay, input.Task.RequestFingerprint); compareErr != nil {
			return nil, compareErr
		}
		return cloneImageTask(&replay), nil
	}
	var canonical canonicalImageRunRow
	if canonicalErr := canonicalImageRunLookupDB(r.db.WithContext(ctx), int64(input.Task.UserID), input.Task.RequestID).Take(&canonical).Error; canonicalErr == nil {
		if compareErr := compareCanonicalImageFingerprint(canonical, input.Task.RequestFingerprint); compareErr != nil {
			return nil, compareErr
		}
		return nil, requestidentity.ErrRequestIdentityConflict
	} else if !errors.Is(canonicalErr, gorm.ErrRecordNotFound) {
		return nil, canonicalErr
	}
	return nil, err
}

func (r *GormRepository) FindAcceptedTaskByRequestID(ctx context.Context, userID uint64, requestID string) (*AcceptedTaskReplay, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	requestID = strings.TrimSpace(requestID)
	if userID == 0 || requestID == "" {
		return nil, nil
	}
	var run canonicalImageRunRow
	err := canonicalImageRunLookupDB(r.db.WithContext(ctx), int64(userID), requestID).Take(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if run.ID <= 0 || len(run.RequestFingerprint) != sha256.Size ||
		requestidentity.IdentityStatus(run.RequestIdentityStatus) != requestidentity.IdentityStatusReplayable ||
		strings.TrimSpace(run.RequestIdentityMarker) != "" || strings.TrimSpace(run.InputSnapshot) == "" || strings.TrimSpace(run.PricingSnapshotJSON) == "" {
		return nil, requestidentity.ErrRequestIdentityNotReplayable
	}
	var task ImageTask
	err = r.db.WithContext(ctx).
		Where("run_id = ? AND user_id = ? AND request_id = ?", run.ID, userID, requestID).
		First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, requestidentity.ErrRequestIdentityConflict
	}
	if err != nil {
		return nil, err
	}
	if task.RunID != run.ID || len(task.RequestFingerprint) != sha256.Size || !bytes.Equal(task.RequestFingerprint, run.RequestFingerprint) ||
		task.RequestIdentityStatus != run.RequestIdentityStatus || strings.TrimSpace(task.RequestIdentityMarker) != "" {
		return nil, requestidentity.ErrRequestIdentityNotReplayable
	}
	return &AcceptedTaskReplay{
		Task: *cloneImageTask(&task), InputSnapshot: strings.TrimSpace(run.InputSnapshot),
		PricingSnapshotJSON: strings.TrimSpace(run.PricingSnapshotJSON),
	}, nil
}

func canonicalImageRunLookupDB(db *gorm.DB, userID int64, requestID string) *gorm.DB {
	return db.Table("ai_runs").
		Select("id, request_fingerprint, request_identity_status, request_identity_marker, input_snapshot, pricing_snapshot_json").
		Where("user_id = ? AND request_id = ?", userID, requestID)
}

func (r *GormRepository) FindPendingImages(ctx context.Context, limit int) ([]ImageTask, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var tasks []ImageTask
	if err := pendingImageTasksDB(r.db.WithContext(ctx), limit).Find(&tasks).Error; err != nil {
		return nil, err
	}
	for index := range tasks {
		tasks[index].RequestFingerprint = append([]byte(nil), tasks[index].RequestFingerprint...)
	}
	return tasks, nil
}

func pendingImageTasksDB(db *gorm.DB, limit int) *gorm.DB {
	if limit <= 0 {
		limit = 25
	}
	return db.Table("ai_image_tasks AS t").Select("t.*").
		Joins("JOIN ai_runs r ON r.id = t.run_id").
		Where("(t.status = ? OR (t.status = ? AND (t.lease_expires_at IS NULL OR t.lease_expires_at <= ?))) AND r.status = ? AND r.billing_status IN ?", StatusPending, StatusRunning, time.Now(), enum.AIRunStatusRunning, []billing.BillingStatus{billing.BillingStatusPending, billing.BillingStatusHeld}).
		Order("t.created_at ASC, t.id ASC").Limit(limit)
}

func createImageTaskFiles(tx *gorm.DB, taskID uint64, files TaskFileSet) error {
	inputIDsBySort := make(map[int]uint64, len(files.Inputs))
	for i := range files.Inputs {
		files.Inputs[i].TaskID, files.Inputs[i].IsDel = taskID, enum.CommonNo
		if err := tx.Create(&files.Inputs[i]).Error; err != nil {
			return err
		}
		inputIDsBySort[files.Inputs[i].SortOrder] = files.Inputs[i].ID
	}
	if files.Mask != nil {
		mask := files.Mask.File
		mask.TaskID, mask.IsDel = taskID, enum.CommonNo
		if relatedID, ok := inputIDsBySort[files.Mask.RelatedSortOrder]; ok {
			mask.RelatedFileID = &relatedID
		}
		if err := tx.Create(&mask).Error; err != nil {
			return err
		}
	}
	return nil
}

func compareImageFingerprint(task ImageTask, incoming []byte) error {
	if len(task.RequestFingerprint) != sha256.Size || len(incoming) != sha256.Size {
		return requestidentity.ErrRequestIdentityNotReplayable
	}
	var stored, candidate [sha256.Size]byte
	copy(stored[:], task.RequestFingerprint)
	copy(candidate[:], incoming)
	return requestidentity.CompareForReplay(requestidentity.IdentityStatus(task.RequestIdentityStatus), stored, candidate)
}

func compareCanonicalImageFingerprint(run canonicalImageRunRow, incoming []byte) error {
	if run.ID <= 0 || len(run.RequestFingerprint) != sha256.Size || len(incoming) != sha256.Size || strings.TrimSpace(run.RequestIdentityMarker) != "" {
		return requestidentity.ErrRequestIdentityNotReplayable
	}
	var stored, candidate [sha256.Size]byte
	copy(stored[:], run.RequestFingerprint)
	copy(candidate[:], incoming)
	return requestidentity.CompareForReplay(requestidentity.IdentityStatus(run.RequestIdentityStatus), stored, candidate)
}

func lockAcceptedImageGraph(tx *gorm.DB, taskID uint64, userID int64, requestID string) (ImageTask, error) {
	var task ImageTask
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ? AND request_id = ?", taskID, userID, requestID).First(&task).Error; err != nil {
		return ImageTask{}, err
	}
	var run airun.Run
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ? AND request_id = ?", task.RunID, userID, requestID).First(&run).Error; err != nil {
		return ImageTask{}, err
	}
	var charge billing.UsageCharge
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ? AND user_id = ?", run.ID, userID).First(&charge).Error; err != nil {
		return ImageTask{}, err
	}
	if task.RunID != run.ID || len(run.RequestFingerprint) != sha256.Size || !bytes.Equal(task.RequestFingerprint, run.RequestFingerprint) || charge.RunID != run.ID {
		return ImageTask{}, errors.New("image accepted graph is inconsistent")
	}
	return task, nil
}

func cloneImageTask(task *ImageTask) *ImageTask {
	if task == nil {
		return nil
	}
	copy := *task
	copy.RequestFingerprint = append([]byte(nil), task.RequestFingerprint...)
	return &copy
}

func (r *GormRepository) DeleteTask(ctx context.Context, userID uint64, taskID uint64, platform string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tasks := tx.Model(&ImageTask{}).
			Where("platform = ? AND user_id = ? AND id = ?", strings.TrimSpace(platform), userID, taskID)
		result := tasks.Where("is_del = ?", enum.CommonNo).
			Updates(map[string]any{"is_del": enum.CommonYes, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Model(&ImageFile{}).
			Where("task_id = ? AND is_del = ?", taskID, enum.CommonNo).
			Update("is_del", enum.CommonYes).Error
	})
}

func (r *GormRepository) LoadAgentRuntime(ctx context.Context, agentID uint64) (*AgentRuntime, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if agentID == 0 {
		return nil, nil
	}
	var row AgentRuntime
	err := r.db.WithContext(ctx).Table("ai_agents AS a").
		Select(`a.id AS agent_id,
			a.name AS agent_name,
			a.scenes_json AS scenes_json,
			a.status AS agent_status,
			a.billing_multiplier_ppm AS billing_multiplier_ppm,
			a.provider_id AS provider_id,
			p.name AS provider_name,
			p.engine_type AS engine_type,
			p.base_url AS base_url,
			p.api_key_enc AS api_key_enc,
			p.status AS provider_status,
			a.model_id AS model_id,
			COALESCE(NULLIF(m.display_name, ''), a.model_display_name) AS model_display_name,
			m.status AS model_status,
			m.official_model_id AS official_model_id,
			m.official_catalog_version AS official_catalog_version,
			m.mapping_status AS mapping_status`).
		Joins("JOIN ai_providers AS p ON p.id = a.provider_id AND p.is_del = ?", enum.CommonNo).
		Joins("JOIN ai_provider_models AS m ON m.provider_id = a.provider_id AND m.model_id = a.model_id AND m.status = ? AND m.mapping_status = ?", enum.CommonYes, officialmodel.MappingStatusMapped).
		Where("a.id = ? AND a.is_del = ?", agentID, enum.CommonNo).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) ClaimTask(ctx context.Context, platform string, userID uint64, taskID uint64, startedAt time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	tx := workerTaskDB(r.db.WithContext(ctx), platform, userID, taskID).
		Where("status = ?", StatusPending).
		Updates(map[string]any{"status": StatusRunning, "updated_at": startedAt})
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}

func (r *GormRepository) ClaimTaskLease(ctx context.Context, platform string, userID uint64, taskID uint64, owner string, now time.Time, ttl time.Duration) (*TaskLease, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	owner = strings.TrimSpace(owner)
	if userID == 0 || taskID == 0 || owner == "" || now.IsZero() || ttl <= 0 {
		return nil, errors.New("image task lease input is invalid")
	}
	expiresAt := now.Add(ttl)
	var claim *TaskLease
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task ImageTask
		err := claimableImageTaskDB(tx.Clauses(clause.Locking{Strength: "UPDATE"}), platform, userID, taskID, now).
			First(&task).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		nextToken := task.LeaseToken + 1
		if nextToken == 0 {
			return errors.New("image task lease token overflow")
		}
		result := tx.Model(&ImageTask{}).Where("id = ? AND lease_token = ?", task.ID, task.LeaseToken).Updates(map[string]any{
			"status": StatusRunning, "lease_owner": owner, "lease_token": nextToken, "lease_expires_at": expiresAt, "updated_at": now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		task.Status, task.LeaseOwner, task.LeaseToken, task.LeaseExpiresAt, task.UpdatedAt = StatusRunning, &owner, nextToken, &expiresAt, now
		claim = &TaskLease{Task: *cloneImageTask(&task), Owner: owner, Token: nextToken, ExpiresAt: expiresAt}
		return nil
	})
	return claim, err
}

func claimableImageTaskDB(db *gorm.DB, platform string, userID uint64, taskID uint64, now time.Time) *gorm.DB {
	return workerTaskDB(db, platform, userID, taskID).
		Where("status = ? OR (status = ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?))", StatusPending, StatusRunning, now)
}

func (r *GormRepository) RenewTaskLease(ctx context.Context, taskID uint64, owner string, token uint64, now time.Time, expiresAt time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, ErrRepositoryNotConfigured
	}
	owner = strings.TrimSpace(owner)
	if taskID == 0 || owner == "" || token == 0 || now.IsZero() || !expiresAt.After(now) {
		return false, errors.New("image task lease renewal input is invalid")
	}
	result := r.db.WithContext(ctx).Model(&ImageTask{}).
		Where("id = ? AND status = ? AND lease_owner = ? AND lease_token = ? AND lease_expires_at > ?", taskID, StatusRunning, owner, token, now).
		Updates(map[string]any{"lease_expires_at": expiresAt, "updated_at": now})
	return result.RowsAffected == 1, result.Error
}

func (r *GormRepository) AppendTaskFiles(ctx context.Context, files []ImageFile) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if len(files) == 0 {
		return nil
	}
	for i := range files {
		files[i].IsDel = enum.CommonNo
	}
	return r.db.WithContext(ctx).Create(&files).Error
}

func (r *GormRepository) FinishTaskSuccess(ctx context.Context, platform string, userID uint64, taskID uint64, actualParamsJSON *string, rawResponseJSON *string, elapsedMS int, finishedAt time.Time) error {
	return r.finishTask(ctx, platform, userID, taskID, map[string]any{
		"status":             StatusSuccess,
		"error_message":      "",
		"actual_params_json": actualParamsJSON,
		"raw_response_json":  rawResponseJSON,
		"elapsed_ms":         elapsedMS,
		"finished_at":        finishedAt,
		"updated_at":         finishedAt,
	})
}

func (r *GormRepository) FinishTaskFailed(ctx context.Context, platform string, userID uint64, taskID uint64, message string, elapsedMS int, finishedAt time.Time) error {
	return r.finishTask(ctx, platform, userID, taskID, map[string]any{
		"status":        StatusFailed,
		"error_message": message,
		"elapsed_ms":    elapsedMS,
		"finished_at":   finishedAt,
		"updated_at":    finishedAt,
	})
}

func (r *GormRepository) LoadUploadConfig(ctx context.Context) (*UploadConfig, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row UploadConfig
	err := r.db.WithContext(ctx).
		Table("upload_setting AS s").
		Select(`s.id AS setting_id,
			d.driver, d.secret_id_enc, d.secret_key_enc, d.bucket, d.region, d.appid, d.endpoint, d.bucket_domain`).
		Joins("JOIN upload_driver AS d ON d.id = s.driver_id AND d.is_del = ?", enum.CommonNo).
		Joins("JOIN upload_rule AS rule ON rule.id = s.rule_id AND rule.is_del = ?", enum.CommonNo).
		Where("s.status = ?", enum.CommonYes).
		Where("s.is_del = ?", enum.CommonNo).
		Order("s.id DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.SettingID == 0 {
		return nil, nil
	}
	return &row, nil
}

func (r *GormRepository) tasks(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Model(&ImageTask{}).Where("is_del = ?", enum.CommonNo)
}

func (r *GormRepository) userTask(ctx context.Context, userID uint64, taskID uint64, platform string) *gorm.DB {
	return userTaskDB(r.db.WithContext(ctx), platform, userID, taskID)
}

func userTaskDB(db *gorm.DB, platform string, userID uint64, taskID uint64) *gorm.DB {
	return db.Model(&ImageTask{}).
		Where("is_del = ? AND platform = ? AND user_id = ? AND id = ?", enum.CommonNo, strings.TrimSpace(platform), userID, taskID)
}

func workerTaskDB(db *gorm.DB, platform string, userID uint64, taskID uint64) *gorm.DB {
	return db.Model(&ImageTask{}).
		Where("platform = ? AND user_id = ? AND id = ?", strings.TrimSpace(platform), userID, taskID)
}

func (r *GormRepository) finishTask(ctx context.Context, platform string, userID uint64, taskID uint64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	tx := r.tasks(ctx).Where("platform = ? AND user_id = ? AND id = ?", strings.TrimSpace(platform), userID, taskID).Updates(fields)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
