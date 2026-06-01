package profile

import (
	"context"

	usermodule "admin_back_go/internal/module/user"
	"admin_back_go/internal/shared/apperror"
)

type ProfileResponse = usermodule.ProfileResponse
type ProfileDetail = usermodule.ProfileDetail
type ProfileDict = usermodule.ProfileDict
type UpdateProfileInput = usermodule.UpdateProfileInput
type UpdatePasswordInput = usermodule.UpdatePasswordInput
type UpdateEmailInput = usermodule.UpdateEmailInput
type UpdatePhoneInput = usermodule.UpdatePhoneInput

type AddressTreeNode = usermodule.AddressTreeNode
type SexOption = usermodule.SexOption

type HTTPService interface {
	Profile(ctx context.Context, userID int64, currentUserID int64) (*ProfileResponse, *apperror.Error)
	UpdateProfile(ctx context.Context, input UpdateProfileInput) *apperror.Error
	UpdatePassword(ctx context.Context, input UpdatePasswordInput) *apperror.Error
	UpdateEmail(ctx context.Context, input UpdateEmailInput) *apperror.Error
	UpdatePhone(ctx context.Context, input UpdatePhoneInput) *apperror.Error
}

type AppService interface {
	Profile(ctx context.Context, userID int64, currentUserID int64) (*ProfileResponse, *apperror.Error)
	UpdateProfile(ctx context.Context, input UpdateProfileInput) *apperror.Error
}
