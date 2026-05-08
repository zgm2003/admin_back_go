package userquickentry

import (
	"context"

	"admin_back_go/internal/apperror"
)

type HTTPService interface {
	Save(ctx context.Context, userID int64, input SaveInput) (*SaveResponse, *apperror.Error)
}

type SaveInput struct {
	PermissionIDs []int64
}

type SaveResponse struct {
	QuickEntry []QuickEntry `json:"quick_entry"`
}

type QuickEntry struct {
	ID           int64 `json:"id"`
	PermissionID int64 `json:"permission_id"`
	Sort         int   `json:"sort"`
}
