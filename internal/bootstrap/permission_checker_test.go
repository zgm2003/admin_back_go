package bootstrap

import (
	"context"
	"testing"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/shared/apperror"
)

type fakePrincipalAuthorizer struct {
	userID   int64
	platform string
	code     string
	err      *apperror.Error
}

func (f *fakePrincipalAuthorizer) Authorize(_ context.Context, userID int64, platform string, code string) *apperror.Error {
	f.userID = userID
	f.platform = platform
	f.code = code
	return f.err
}

func TestPermissionCheckerDelegatesToVersionedPrincipalService(t *testing.T) {
	authorizer := &fakePrincipalAuthorizer{}
	checker := PermissionCheckerFor(authorizer)

	if appErr := checker(context.Background(), middleware.PermissionInput{UserID: 12, Platform: "admin", Code: "user_list"}); appErr != nil {
		t.Fatalf("checker error = %v", appErr)
	}
	if authorizer.userID != 12 || authorizer.platform != "admin" || authorizer.code != "user_list" {
		t.Fatalf("authorizer input = user:%d platform:%q code:%q", authorizer.userID, authorizer.platform, authorizer.code)
	}
}

func TestPermissionCheckerPreservesFailClosedPrincipalError(t *testing.T) {
	want := apperror.New("permission.principal_cache_unavailable", apperror.CategoryDependency, 0, apperror.Retryable, "permission.principal_cache_unavailable", nil, "权限缓存不可用")
	checker := PermissionCheckerFor(&fakePrincipalAuthorizer{err: want})
	if got := checker(context.Background(), middleware.PermissionInput{UserID: 12, Platform: "admin", Code: "user_list"}); got != want {
		t.Fatalf("checker error = %#v, want %#v", got, want)
	}
}

func TestPermissionCheckerWithoutPrincipalServiceFailsClosed(t *testing.T) {
	checker := PermissionCheckerFor(nil)
	if appErr := checker(context.Background(), middleware.PermissionInput{UserID: 12, Platform: "admin", Code: "user_list"}); appErr == nil || appErr.LegacyCode != apperror.CodeInternal {
		t.Fatalf("checker error = %#v, want internal failure", appErr)
	}
}
