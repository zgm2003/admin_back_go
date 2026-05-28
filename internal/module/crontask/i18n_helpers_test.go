package crontask

import (
	"context"
	"errors"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

func assertCronTaskMessageID(t *testing.T, appErr *apperror.Error, want string) {
	t.Helper()
	if appErr == nil {
		t.Fatalf("expected app error %q, got nil", want)
	}
	if appErr.MessageID != want {
		t.Fatalf("expected message id %q, got %#v", want, appErr)
	}
}

func validCronTaskInput() SaveInput {
	return SaveInput{
		Name:   "demo_task",
		Title:  "Demo task",
		Cron:   "0 * * * * *",
		Status: enum.CommonYes,
	}
}

type failingCreateRepository struct {
	fakeRepository
	createErr error
}

func (f *failingCreateRepository) Create(ctx context.Context, row Task) (int64, error) {
	if f.createErr != nil {
		return 0, f.createErr
	}
	return 0, errors.New("create failed")
}

type failingUpdateRepository struct{ fakeRepository }

func (f *failingUpdateRepository) Get(ctx context.Context, id int64) (*Task, error) {
	input := validCronTaskInput()
	return &Task{ID: id, Name: input.Name, Title: input.Title, Cron: input.Cron, Status: input.Status}, nil
}

func (f *failingUpdateRepository) Update(ctx context.Context, id int64, row Task) error {
	return errors.New("update failed")
}

type failingStatusRepository struct{ fakeRepository }

func (f *failingStatusRepository) Get(ctx context.Context, id int64) (*Task, error) {
	input := validCronTaskInput()
	return &Task{ID: id, Name: input.Name, Title: input.Title, Cron: input.Cron, Status: input.Status}, nil
}

func (f *failingStatusRepository) UpdateStatus(ctx context.Context, id int64, status int) error {
	return errors.New("status update failed")
}

type failingDeleteRepository struct{ fakeRepository }

func (f *failingDeleteRepository) Get(ctx context.Context, id int64) (*Task, error) {
	input := validCronTaskInput()
	return &Task{ID: id, Name: input.Name, Title: input.Title, Cron: input.Cron, Status: input.Status}, nil
}

func (f *failingDeleteRepository) Delete(ctx context.Context, ids []int64) error {
	return errors.New("delete failed")
}
