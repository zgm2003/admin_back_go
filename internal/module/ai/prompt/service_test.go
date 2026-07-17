package prompt

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"admin_back_go/internal/shared/apperror"
)

type fakePromptRepository struct {
	rows            []Prompt
	err             error
	detailNil       bool
	listQuery       ListQuery
	detailID        int64
	created         Prompt
	updatedID       int64
	updated         Prompt
	statusID        int64
	status          int
	deletedID       int64
	batchDeletedIDs []int64
	createCalled    bool
}

func (f *fakePromptRepository) List(ctx context.Context, query ListQuery) ([]Prompt, int64, error) {
	f.listQuery = query
	return f.rows, int64(len(f.rows)), f.err
}

func (f *fakePromptRepository) Detail(ctx context.Context, id int64) (*Prompt, error) {
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

func (f *fakePromptRepository) Create(ctx context.Context, row Prompt) (int64, error) {
	f.created = row
	f.createCalled = true
	return 1, f.err
}

func (f *fakePromptRepository) Update(ctx context.Context, id int64, row Prompt) error {
	f.updatedID = id
	f.updated = row
	return f.err
}

func (f *fakePromptRepository) ChangeStatus(ctx context.Context, id int64, status int) error {
	f.statusID = id
	f.status = status
	return f.err
}

func (f *fakePromptRepository) SoftDelete(ctx context.Context, id int64) error {
	f.deletedID = id
	return f.err
}

func (f *fakePromptRepository) SoftDeleteBatch(ctx context.Context, ids []int64) error {
	f.batchDeletedIDs = append([]int64(nil), ids...)
	return f.err
}

func TestServicePublicListForcesEnabledActiveRows(t *testing.T) {
	repo := &fakePromptRepository{rows: []Prompt{{ID: 1, Slug: "cat", Title: "Cat", Prompt: "draw cat", Status: StatusEnabled, IsDel: IsDelActive}}}
	svc := NewService(repo)

	result, appErr := svc.PublicList(context.Background(), ListQuery{Status: StatusDisabled, IsDel: IsDelDeleted})

	if appErr != nil {
		t.Fatalf("PublicList returned error: %#v", appErr)
	}
	if len(result.List) != 1 {
		t.Fatalf("expected one prompt, got %#v", result)
	}
	if repo.listQuery.Status != StatusEnabled || repo.listQuery.IsDel != IsDelActive {
		t.Fatalf("PublicList must force enabled active rows, query=%#v", repo.listQuery)
	}
}

func TestServiceCreateRejectsMissingRequiredFields(t *testing.T) {
	svc := NewService(&fakePromptRepository{})
	for _, input := range []Input{
		{Slug: "", Title: "Title", Prompt: "Prompt"},
		{Slug: "slug", Title: "", Prompt: "Prompt"},
		{Slug: "slug", Title: "Title", Prompt: ""},
	} {
		_, appErr := svc.Create(context.Background(), input)
		if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
			t.Fatalf("expected CodeBadRequest for %#v, got %#v", input, appErr)
		}
	}
}

func TestServiceRepositoryErrorsReturnInternal(t *testing.T) {
	repoErr := errors.New("db down")
	svc := NewService(&fakePromptRepository{err: repoErr})

	_, appErr := svc.PublicList(context.Background(), ListQuery{})
	if appErr == nil || appErr.LegacyCode != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal list error, got %#v", appErr)
	}

	_, appErr = svc.Create(context.Background(), Input{Slug: "cat", Title: "Cat", Prompt: "draw cat"})
	if appErr == nil || appErr.LegacyCode != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal create error, got %#v", appErr)
	}
}

func TestServicePageInitDetailUpdateStatusAndBatchDelete(t *testing.T) {
	repo := &fakePromptRepository{rows: []Prompt{{ID: 7, Slug: "cat", Title: "Cat", Prompt: "draw cat", Status: StatusEnabled, IsDel: IsDelActive}}}
	svc := NewService(repo)

	initResult, appErr := svc.PageInit(context.Background())
	if appErr != nil || len(initResult.CommonStatusArr) == 0 {
		t.Fatalf("expected page init status options, result=%#v err=%#v", initResult, appErr)
	}

	detail, appErr := svc.Detail(context.Background(), 7)
	if appErr != nil || detail.ID != 7 {
		t.Fatalf("expected detail row, detail=%#v err=%#v", detail, appErr)
	}

	appErr = svc.Update(context.Background(), 7, Input{Slug: "cat", Title: "Cat", Prompt: "draw cat", Status: StatusDisabled})
	if appErr != nil || repo.updatedID != 7 || repo.updated.Status != StatusDisabled {
		t.Fatalf("expected update row, repo=%#v err=%#v", repo, appErr)
	}

	appErr = svc.ChangeStatus(context.Background(), 7, StatusDisabled)
	if appErr != nil || repo.statusID != 7 || repo.status != StatusDisabled {
		t.Fatalf("expected status change, repo=%#v err=%#v", repo, appErr)
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

func TestServiceRejectsInvalidPromptStatusAndIDs(t *testing.T) {
	repo := &fakePromptRepository{}
	svc := NewService(repo)

	if _, appErr := svc.List(context.Background(), ListQuery{Status: 999}); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request list status, got %#v", appErr)
	}
	if _, appErr := svc.Create(context.Background(), Input{Slug: "cat", Title: "Cat", Prompt: "draw cat", Status: 999}); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request create status, got %#v", appErr)
	}
	if appErr := svc.Update(context.Background(), 0, Input{Slug: "cat", Title: "Cat", Prompt: "draw cat"}); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request update id, got %#v", appErr)
	}
	if appErr := svc.ChangeStatus(context.Background(), 7, 999); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request change status, got %#v", appErr)
	}
	if appErr := svc.DeleteOne(context.Background(), 0); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request delete id, got %#v", appErr)
	}
	if appErr := svc.DeleteBatch(context.Background(), []int64{3, 0}); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request batch ids, got %#v", appErr)
	}
	if appErr := svc.DeleteBatch(context.Background(), []int64{3, 3}); appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("expected bad request duplicate batch ids, got %#v", appErr)
	}
}

func TestServiceDetailFailsClosedWhenRepositoryReturnsNilRow(t *testing.T) {
	svc := NewService(&fakePromptRepository{detailNil: true})

	_, appErr := svc.Detail(context.Background(), 7)
	if appErr == nil || appErr.LegacyCode != apperror.CodeNotFound {
		t.Fatalf("expected not found for nil detail row, got %#v", appErr)
	}
}
