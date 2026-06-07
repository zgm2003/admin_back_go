package asset

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"admin_back_go/internal/shared/apperror"
)

type fakeAssetRepository struct {
	rows            []Asset
	err             error
	detailNil       bool
	listQuery       ListQuery
	detailID        int64
	created         Asset
	updatedID       int64
	updated         Asset
	deletedID       int64
	batchDeletedIDs []int64
	createCalled    bool
	updateCalled    bool
	deleteCalled    bool
}

func (f *fakeAssetRepository) List(ctx context.Context, query ListQuery) ([]Asset, int64, error) {
	f.listQuery = query
	return f.rows, int64(len(f.rows)), f.err
}

func (f *fakeAssetRepository) Detail(ctx context.Context, id int64) (*Asset, error) {
	f.detailID = id
	if f.err != nil {
		return nil, f.err
	}
	if f.detailNil {
		return nil, nil
	}
	if len(f.rows) == 0 {
		return nil, ErrNotFound
	}
	return &f.rows[0], nil
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

func (f *fakeAssetRepository) SoftDelete(ctx context.Context, id int64) error {
	f.deletedID = id
	f.deleteCalled = true
	return f.err
}

func (f *fakeAssetRepository) SoftDeleteBatch(ctx context.Context, ids []int64) error {
	f.batchDeletedIDs = append([]int64(nil), ids...)
	return f.err
}

func TestServicePublicListForcesEnabledActiveRows(t *testing.T) {
	repo := &fakeAssetRepository{rows: []Asset{{ID: 1, Slug: "hero", Type: AssetTypeImage, Title: "Hero", Status: StatusEnabled, IsDel: IsDelActive}}}
	svc := NewService(repo)

	result, appErr := svc.PublicList(context.Background(), ListQuery{Status: StatusDisabled, IsDel: IsDelDeleted})

	if appErr != nil {
		t.Fatalf("PublicList returned error: %#v", appErr)
	}
	if len(result.List) != 1 {
		t.Fatalf("expected one asset, got %#v", result)
	}
	if repo.listQuery.Status != StatusEnabled || repo.listQuery.IsDel != IsDelActive {
		t.Fatalf("PublicList must force enabled active rows, query=%#v", repo.listQuery)
	}
}

func TestServiceCreateRejectsMissingRequiredFieldsAndUnknownType(t *testing.T) {
	svc := NewService(&fakeAssetRepository{})
	for _, input := range []Input{
		{Slug: "", Type: AssetTypeText, Title: "Title"},
		{Slug: "slug", Type: "", Title: "Title"},
		{Slug: "slug", Type: AssetTypeImage, Title: ""},
		{Slug: "slug", Type: "audio", Title: "Title"},
		{Slug: "slug", Type: AssetTypeImage, Title: "Title", Status: 999},
	} {
		_, appErr := svc.Create(context.Background(), input)
		if appErr == nil || appErr.Code != apperror.CodeBadRequest {
			t.Fatalf("expected CodeBadRequest for %#v, got %#v", input, appErr)
		}
	}
}

func TestServiceCreateAcceptsVideoAssetType(t *testing.T) {
	repo := &fakeAssetRepository{}
	svc := NewService(repo)

	id, appErr := svc.Create(context.Background(), Input{Slug: "clip", Type: AssetTypeVideo, Title: "Clip", URL: "https://example.test/clip.mp4", Status: StatusDisabled})

	if appErr != nil || id != 11 {
		t.Fatalf("Create returned id=%d err=%#v", id, appErr)
	}
	if !repo.createCalled || repo.created.Type != AssetTypeVideo || repo.created.Status != StatusDisabled || repo.created.IsDel != IsDelActive {
		t.Fatalf("unexpected created asset: called=%v row=%#v", repo.createCalled, repo.created)
	}
}

func TestServiceUpdateAndDeleteSurfaceRepositoryErrorsAsInternal(t *testing.T) {
	repoErr := errors.New("db down")
	svc := NewService(&fakeAssetRepository{err: repoErr})

	_, appErr := svc.PublicList(context.Background(), ListQuery{})
	if appErr == nil || appErr.Code != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal list error, got %#v", appErr)
	}

	_, appErr = svc.Create(context.Background(), Input{Slug: "hero", Type: AssetTypeImage, Title: "Hero"})
	if appErr == nil || appErr.Code != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal create error, got %#v", appErr)
	}

	appErr = svc.Update(context.Background(), 5, Input{Slug: "hero", Type: AssetTypeImage, Title: "Hero"})
	if appErr == nil || appErr.Code != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal update error, got %#v", appErr)
	}

	appErr = svc.Delete(context.Background(), 5)
	if appErr == nil || appErr.Code != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal delete error, got %#v", appErr)
	}
}

func TestServiceUpdateAndDeleteRejectInvalidID(t *testing.T) {
	repo := &fakeAssetRepository{}
	svc := NewService(repo)

	if appErr := svc.Update(context.Background(), 0, Input{Slug: "hero", Type: AssetTypeImage, Title: "Hero"}); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request update id, got %#v", appErr)
	}
	if appErr := svc.Update(context.Background(), 5, Input{Slug: "hero", Type: AssetTypeImage, Title: "Hero", Status: -1}); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request update status, got %#v", appErr)
	}
	if appErr := svc.Delete(context.Background(), 0); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request delete id, got %#v", appErr)
	}
	if repo.updateCalled || repo.deleteCalled {
		t.Fatalf("invalid IDs must not reach repository: %#v", repo)
	}
}

func TestServicePageInitDetailAndBatchDelete(t *testing.T) {
	repo := &fakeAssetRepository{rows: []Asset{{ID: 7, Slug: "hero", Type: AssetTypeImage, Title: "Hero", Status: StatusEnabled, IsDel: IsDelActive}}}
	svc := NewService(repo)

	initResult, appErr := svc.PageInit(context.Background())
	if appErr != nil || len(initResult.CommonStatusArr) == 0 || len(initResult.AIAssetTypeArr) != 3 {
		t.Fatalf("expected page init options, result=%#v err=%#v", initResult, appErr)
	}

	detail, appErr := svc.Detail(context.Background(), 7)
	if appErr != nil || detail.ID != 7 {
		t.Fatalf("expected detail row, detail=%#v err=%#v", detail, appErr)
	}

	appErr = svc.DeleteOne(context.Background(), 7)
	if appErr != nil || repo.deletedID != 7 {
		t.Fatalf("expected delete one, repo=%#v err=%#v", repo, appErr)
	}

	appErr = svc.DeleteBatch(context.Background(), []int64{3, 4})
	if appErr != nil || !reflect.DeepEqual(repo.batchDeletedIDs, []int64{3, 4}) {
		t.Fatalf("expected delete batch, repo=%#v err=%#v", repo, appErr)
	}
}

func TestServiceRejectsInvalidAssetListStatusAndBatchIDs(t *testing.T) {
	repo := &fakeAssetRepository{}
	svc := NewService(repo)

	if _, appErr := svc.List(context.Background(), ListQuery{Status: 999}); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request list status, got %#v", appErr)
	}
	if _, appErr := svc.Detail(context.Background(), 0); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request detail id, got %#v", appErr)
	}
	if appErr := svc.DeleteBatch(context.Background(), []int64{}); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request empty batch, got %#v", appErr)
	}
	if appErr := svc.DeleteBatch(context.Background(), []int64{3, -1}); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request invalid batch id, got %#v", appErr)
	}
	if appErr := svc.DeleteBatch(context.Background(), []int64{3, 3}); appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request duplicate batch ids, got %#v", appErr)
	}
}

func TestServiceDetailFailsClosedWhenRepositoryReturnsNilRow(t *testing.T) {
	svc := NewService(&fakeAssetRepository{detailNil: true})

	_, appErr := svc.Detail(context.Background(), 7)
	if appErr == nil || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("expected not found for nil detail row, got %#v", appErr)
	}
}
