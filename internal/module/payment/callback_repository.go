package payment

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type callbackRepository interface {
	AcquireCallbackEvent(ctx context.Context, event CallbackEvent) (*CallbackEvent, bool, error)
	ResolveCallbackEvent(ctx context.Context, resolution CallbackEventResolution) (*CallbackEventResolutionResult, error)
}

func (r *GormRepository) CreateCallbackEvent(ctx context.Context, event CallbackEvent) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	normalizeCallbackEvent(&event)
	if err := r.db.WithContext(ctx).Create(&event).Error; err != nil {
		return 0, err
	}
	return event.ID, nil
}

func (r *GormRepository) AcquireCallbackEvent(ctx context.Context, event CallbackEvent) (*CallbackEvent, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, ErrRepositoryNotConfigured
	}
	normalizeCallbackEvent(&event)
	if len(event.DedupeKey) != 32 || event.Provider == "" || event.ProcessStatus != callbackProcessPending {
		return nil, false, ErrCallbackStateChanged
	}
	event.IsDel = enum.CommonNo
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "dedupe_key"}},
		DoNothing: true,
	}).Create(&event)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return &event, true, nil
	}

	var existing CallbackEvent
	if err := r.db.WithContext(ctx).Where("dedupe_key = ?", event.DedupeKey).First(&existing).Error; err != nil {
		return nil, false, err
	}
	return &existing, false, nil
}

func (r *GormRepository) UpdateCallbackEventProcessed(ctx context.Context, id int64, signatureValid int, status string, message string, processedAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&CallbackEvent{}).Where("id = ?", id).Updates(map[string]any{
		"signature_valid": signatureValid,
		"process_status":  strings.TrimSpace(status),
		"process_message": trimMax(message, 512),
		"processed_at":    processedAt,
	}).Error
}

func (r *GormRepository) ResolveCallbackEvent(ctx context.Context, resolution CallbackEventResolution) (*CallbackEventResolutionResult, error) {
	if r == nil || r.db == nil || r.walletParticipant == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if err := validateCallbackEventResolution(resolution); err != nil {
		return nil, err
	}

	var resolved CallbackEventResolutionResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var event CallbackEvent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND dedupe_key = ?", resolution.EventID, resolution.DedupeKey).
			First(&event).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCallbackStateChanged
			}
			return err
		}
		if event.IsDel != enum.CommonNo {
			return ErrCallbackStateChanged
		}
		if event.ProcessStatus == callbackProcessSuccess || event.ProcessStatus == callbackProcessIgnored {
			resolved = CallbackEventResolutionResult{Event: &event, Replay: true}
			return nil
		}
		if event.ProcessStatus != callbackProcessPending && event.ProcessStatus != callbackProcessFailed {
			return ErrCallbackStateChanged
		}

		var paidOrder *PaidOrderFinalization
		if resolution.PaidOrderID > 0 {
			fact, err := r.finalizePaidOrderInTx(
				ctx,
				tx,
				resolution.PaidOrderID,
				resolution.AlipayTradeNo,
				resolution.PaidAt,
				resolution.ProcessedAt,
			)
			if err != nil {
				return err
			}
			paidOrder = fact
		}

		result := tx.Model(&CallbackEvent{}).
			Where("id = ? AND dedupe_key = ? AND is_del = ? AND process_status IN ?", event.ID, resolution.DedupeKey, enum.CommonNo, []string{callbackProcessPending, callbackProcessFailed}).
			Updates(map[string]any{
				"signature_valid": resolution.SignatureValid,
				"process_status":  resolution.ProcessStatus,
				"process_message": trimMax(resolution.ProcessMessage, 512),
				"processed_at":    resolution.ProcessedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCallbackStateChanged
		}
		event.SignatureValid = resolution.SignatureValid
		event.ProcessStatus = resolution.ProcessStatus
		event.ProcessMessage = trimMax(resolution.ProcessMessage, 512)
		event.ProcessedAt = &resolution.ProcessedAt
		resolved = CallbackEventResolutionResult{Event: &event, PaidOrder: paidOrder}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &resolved, nil
}

func normalizeCallbackEvent(event *CallbackEvent) {
	event.Provider = strings.TrimSpace(event.Provider)
	event.NotifyID = strings.TrimSpace(event.NotifyID)
	event.OutTradeNo = strings.TrimSpace(event.OutTradeNo)
	event.TradeNo = strings.TrimSpace(event.TradeNo)
	event.TradeStatus = strings.TrimSpace(event.TradeStatus)
	event.AppID = strings.TrimSpace(event.AppID)
	event.ProcessStatus = strings.TrimSpace(event.ProcessStatus)
	event.ProcessMessage = trimMax(event.ProcessMessage, 512)
}

func validateCallbackEventResolution(resolution CallbackEventResolution) error {
	status := strings.TrimSpace(resolution.ProcessStatus)
	if resolution.EventID <= 0 || len(resolution.DedupeKey) != 32 || resolution.ProcessedAt.IsZero() {
		return ErrCallbackStateChanged
	}
	if resolution.SignatureValid != enum.CommonYes && resolution.SignatureValid != enum.CommonNo {
		return ErrCallbackStateChanged
	}
	if status != callbackProcessSuccess && status != callbackProcessFailed && status != callbackProcessIgnored {
		return ErrCallbackStateChanged
	}
	if status == callbackProcessSuccess {
		if resolution.SignatureValid != enum.CommonYes || resolution.PaidOrderID <= 0 || strings.TrimSpace(resolution.AlipayTradeNo) == "" || resolution.PaidAt.IsZero() {
			return ErrCallbackStateChanged
		}
		return nil
	}
	if resolution.PaidOrderID != 0 || strings.TrimSpace(resolution.AlipayTradeNo) != "" || !resolution.PaidAt.IsZero() {
		return ErrCallbackStateChanged
	}
	return nil
}

func callbackEventFactsEqual(left *CallbackEvent, right *CallbackEvent) bool {
	if left == nil || right == nil {
		return false
	}
	return left.Provider == right.Provider &&
		bytes.Equal(left.DedupeKey, right.DedupeKey) &&
		left.NotifyID == right.NotifyID &&
		left.OutTradeNo == right.OutTradeNo &&
		left.TradeNo == right.TradeNo &&
		left.TradeStatus == right.TradeStatus &&
		left.AppID == right.AppID &&
		left.TotalAmountCents == right.TotalAmountCents
}
