package prompt

import (
	"context"
	"errors"
	"testing"

	"admin_back_go/internal/shared/apperror"
)

type fakePromptRepository struct {
	rows         []Prompt
	err          error
	listQuery    ListQuery
	created      Prompt
	createCalled bool
}

func (f *fakePromptRepository) List(ctx context.Context, query ListQuery) ([]Prompt, int64, error) {
	f.listQuery = query
	return f.rows, int64(len(f.rows)), f.err
}

func (f *fakePromptRepository) Create(ctx context.Context, row Prompt) (int64, error) {
	f.created = row
	f.createCalled = true
	return 1, f.err
}

func (f *fakePromptRepository) SoftDelete(ctx context.Context, id int64) error { return f.err }

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
		if appErr == nil || appErr.Code != apperror.CodeBadRequest {
			t.Fatalf("expected CodeBadRequest for %#v, got %#v", input, appErr)
		}
	}
}

func TestServiceRepositoryErrorsReturnInternal(t *testing.T) {
	repoErr := errors.New("db down")
	svc := NewService(&fakePromptRepository{err: repoErr})

	_, appErr := svc.PublicList(context.Background(), ListQuery{})
	if appErr == nil || appErr.Code != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal list error, got %#v", appErr)
	}

	_, appErr = svc.Create(context.Background(), Input{Slug: "cat", Title: "Cat", Prompt: "draw cat"})
	if appErr == nil || appErr.Code != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped CodeInternal create error, got %#v", appErr)
	}
}
