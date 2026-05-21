package payment

import (
	"context"
	"strings"
	"time"
)

func (r *GormRepository) CreateCallbackEvent(ctx context.Context, event CallbackEvent) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	event.Provider = strings.TrimSpace(event.Provider)
	event.NotifyID = strings.TrimSpace(event.NotifyID)
	event.OutTradeNo = strings.TrimSpace(event.OutTradeNo)
	event.TradeNo = strings.TrimSpace(event.TradeNo)
	event.TradeStatus = strings.TrimSpace(event.TradeStatus)
	event.AppID = strings.TrimSpace(event.AppID)
	event.ProcessStatus = strings.TrimSpace(event.ProcessStatus)
	event.ProcessMessage = trimMax(event.ProcessMessage, 512)
	if err := r.db.WithContext(ctx).Create(&event).Error; err != nil {
		return 0, err
	}
	return event.ID, nil
}

func (r *GormRepository) UpdateCallbackEventProcessed(ctx context.Context, id int64, signatureValid int, status string, message string, processedAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&CallbackEvent{}).Where("id = ?", id).Updates(map[string]any{
		"signature_valid": signatureValid,
		"process_status":  strings.TrimSpace(status),
		"process_message":  trimMax(message, 512),
		"processed_at":     processedAt,
	}).Error
}
