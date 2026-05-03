package operationlog

import (
	"context"
	"encoding/json"

	"admin_back_go/internal/middleware"
)

func NewRecorder(repository Repository) middleware.OperationRecorder {
	return func(ctx context.Context, input middleware.OperationInput) error {
		if repository == nil {
			return ErrRepositoryNotConfigured
		}

		requestData, err := json.Marshal(map[string]any{
			"method":     input.Method,
			"path":       input.Path,
			"module":     input.Module,
			"action":     input.Action,
			"request_id": input.RequestID,
			"client_ip":  input.ClientIP,
			"session_id": input.SessionID,
			"platform":   input.Platform,
			"latency_ms": input.LatencyMs,
		})
		if err != nil {
			return err
		}

		responseData, err := json.Marshal(map[string]any{
			"status":  input.Status,
			"success": input.Success,
		})
		if err != nil {
			return err
		}

		isSuccess := CommonNo
		if input.Success {
			isSuccess = CommonYes
		}

		action := input.Title
		if action == "" {
			action = input.Module + "." + input.Action
		}

		return repository.Create(ctx, Log{
			UserID:       input.UserID,
			Action:       action,
			RequestData:  string(requestData),
			ResponseData: string(responseData),
			IsDel:        CommonNo,
			IsSuccess:    isSuccess,
		})
	}
}
