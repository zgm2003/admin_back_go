package user

import (
	"context"

	"admin_back_go/internal/apperror"
)

type InitService interface {
	Init(ctx context.Context, input InitInput) (*InitResponse, *apperror.Error)
}

type HTTPService interface {
	InitService
	PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error)
	Profile(ctx context.Context, userID int64, currentUserID int64) (*ProfileResponse, *apperror.Error)
	UpdateProfile(ctx context.Context, input UpdateProfileInput) *apperror.Error
	UpdatePassword(ctx context.Context, input UpdatePasswordInput) *apperror.Error
	UpdateEmail(ctx context.Context, input UpdateEmailInput) *apperror.Error
	UpdatePhone(ctx context.Context, input UpdatePhoneInput) *apperror.Error
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Export(ctx context.Context, input ExportInput) (*ExportResponse, *apperror.Error)
	Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error
	ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error
	Delete(ctx context.Context, ids []int64) *apperror.Error
	BatchUpdateProfile(ctx context.Context, input BatchProfileUpdate) *apperror.Error
}
