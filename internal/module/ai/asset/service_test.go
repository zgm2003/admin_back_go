package asset

import (
	"context"
	"errors"
	"testing"

	"admin_back_go/internal/shared/apperror"
)

type fakeAssetRepository struct {
	rows         []Asset
	err          error
	listQuery    ListQuery
	created      Asset
	updatedID    int64
	updated      Asset
	deletedID    int64
	deletedUser  uint64
	createCalled bool
	updateCalled bool
	deleteCalled bool
}

func (f *fakeAssetRepository) List(ctx context.Context, query ListQuery) ([]Asset, int64, error) {
	f.listQuery = query
	return f.rows, int64(len(f.rows)), f.err
}

func (f *fakeAssetRepository) Create(ctx context.Context, row Asset) (int64, error) {
	f.created = row
	f.createCalled = true
	return 11, f.err
}

func (f *fakeAssetRepository) Update(ctx context.Context, id int64, row Asset) error {
	f.updatedID = id
	f.updated = row
	f.updateCalled = true
	return f.err
}

func (f *fakeAssetRepository) SoftDelete(ctx context.Context, id int64, userID uint64) error {
	f.deletedID = id
	f.deletedUser = userID
	f.deleteCalled = true
	return f.err
}

func TestServiceUserListForcesEnabledActiveRowsAndOwner(t *testing.T) {
	repo := &fakeAssetRepository{rows: []Asset{{ID: 1, UserID: 9, Slug: "hero", Type: AssetTypeImage, Title: "Hero", Status: StatusEnabled, IsDel: IsDelActive}}}
	svc := NewService(repo)

	result, appErr := svc.UserList(context.Background(), 9, ListQuery{Status: StatusDisabled, IsDel: IsDelDeleted})

	if appErr != nil {
		t.Fatalf("UserList returned error: %#v", appErr)
	}
	if len(result.List) != 1 || result.List[0].UserID != 9 {
		t.Fatalf("expected one user-owned asset, got %#v", result)
	}
	if repo.listQuery.UserID != 9 || repo.listQuery.Status != StatusEnabled || repo.listQuery.IsDel != IsDelActive {
		t.Fatalf("UserList must force owner/enabled/active rows, query=%#v", repo.listQuery)
	}
}

func TestServiceUserCreateRejectsMissingOwnerAndInvalidFields(t *testing.T) {
	svc := NewService(&fakeAssetRepository{})
	for _, input := range []Input{
		{UserID: 0, Slug: "slug", Type: AssetTypeImage, Title: "Title"},
		{UserID: 9, Slug: "", Type: AssetTypeText, Title: "Title"},
		{UserID: 9, Slug: "slug", Type: "", Title: "Title"},
		{UserID: 9, Slug: "slug", Type: AssetTypeImage, Title: ""},
		{UserID: 9, Slug: "slug", Type: "audio", Title: "Title"},
		{UserID: 9, Slug: "slug", Type: AssetTypeImage, Title: "Title", Status: 999},
	} {
		_, appErr := svc.Create(context.Background(), input)
		if appErr == nil || appErr.Code != apperror.CodeBadRequest {
			t.Fatalf("expected CodeBadRequest for %#v, got %#v", input, appErr)
		}
	}
}

func TestServiceUserCreateAcceptsVideoAssetType(t *testing.T) {
	repo := &fakeAssetRepository{}
	svc := NewService(repo)

	id, appErr := svc.UserCreate(context.Background(), 9, Input{Slug: "clip", Type: AssetTypeVideo, Title: "Clip", URL: "https://example.test/clip.mp4", Status: StatusDisabled})

	if appErr != nil || id != 11 {
		t.Fatalf("UserCreate returned id=%d err=%#v", id, appErr)
	}
	if !repo.createCalled || repo.created.UserID != 9 || repo.created.Type != AssetTypeVideo || repo.created.Status != StatusDisabled || repo.created.IsDel != IsDelActive {
		t.Fatalf("unexpected created asset: called=%v row=%#v", repo.createCalled, repo.created)
	}
}

func TestServiceUserUpdateAndDeleteSurfaceRepositoryErrorsAsInternal(t *testing.T) {
	repoErr := errors.New("db down")
	svc := NewService(&fakeAssetRepository{err: repoErr})

	_, appErr := svc.UserList(context.Background(), 9, ListQuery{})
	if appErr == nil || appErr.Code != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal list error, got %#v", appErr)
	}

	_, appErr = svc.UserCreate(context.Background(), 9, Input{Slug: "hero", Type: AssetTypeImage, Title: "Hero"})
	if appErr == nil || appErr.Code != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal create error, got %#v", appErr)
	}

	appErr = svc.UserUpdate(context.Background(), 9, 5, Input{Slug: "hero", Type: AssetTypeImage, Title: "Hero"})
	if appErr == nil || appErr.Code != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal update error, got %#v", appErr)
	}

	appErr = svc.UserDelete(context.Background(), 9, 5)
	if appErr == nil || appErr.Code != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal delete error, got %#v", appErr)
	}
}

func TestServiceUserUpdateAndDeleteRejectInvalidIDOrOwner(t *testing.T) {
	repo := &fakeAssetRepository{}
	svc := NewService(repo)

	if _, appErr := svc.UserList(context.Background(), 0, ListQuery{}); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request list owner, got %#v", appErr)
	}
	if appErr := svc.UserUpdate(context.Background(), 9, 0, Input{Slug: "hero", Type: AssetTypeImage, Title: "Hero"}); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request update id, got %#v", appErr)
	}
	if appErr := svc.UserUpdate(context.Background(), 9, 5, Input{Slug: "hero", Type: AssetTypeImage, Title: "Hero", Status: -1}); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request update status, got %#v", appErr)
	}
	if appErr := svc.UserDelete(context.Background(), 0, 5); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request delete owner, got %#v", appErr)
	}
	if appErr := svc.UserDelete(context.Background(), 9, 0); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request delete id, got %#v", appErr)
	}
	if repo.updateCalled || repo.deleteCalled {
		t.Fatalf("invalid input must not reach repository: %#v", repo)
	}
}

func TestServicePageInitKeepsAssetTypeDictionary(t *testing.T) {
	svc := NewService(&fakeAssetRepository{})

	initResult, appErr := svc.PageInit(context.Background())
	if appErr != nil || len(initResult.CommonStatusArr) == 0 || len(initResult.AIAssetTypeArr) != 3 {
		t.Fatalf("expected page init options, result=%#v err=%#v", initResult, appErr)
	}
}

func TestServiceRejectsInvalidAssetListStatus(t *testing.T) {
	svc := NewService(&fakeAssetRepository{})

	if _, appErr := svc.List(context.Background(), ListQuery{UserID: 9, Status: 999}); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request list status, got %#v", appErr)
	}
}
