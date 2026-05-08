package userquickentry

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeRepository struct {
	activeIDs       map[int64]struct{}
	replacedUserID  int64
	replacedIDs     []int64
	replaceCalls    int
	replaceResponse []QuickEntry
	err             error
}

func (f *fakeRepository) ActiveAdminPagePermissionIDs(ctx context.Context, ids []int64) (map[int64]struct{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	result := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := f.activeIDs[id]; ok {
			result[id] = struct{}{}
		}
	}
	return result, nil
}

func (f *fakeRepository) ReplaceForUser(ctx context.Context, userID int64, permissionIDs []int64) ([]QuickEntry, error) {
	f.replaceCalls++
	f.replacedUserID = userID
	f.replacedIDs = append([]int64(nil), permissionIDs...)
	if f.err != nil {
		return nil, f.err
	}
	return f.replaceResponse, nil
}

func TestSaveRejectsMissingUser(t *testing.T) {
	service := NewService(&fakeRepository{})

	if _, appErr := service.Save(context.Background(), 0, SaveInput{PermissionIDs: []int64{1}}); appErr == nil {
		t.Fatalf("expected missing user to fail")
	}
}

func TestSaveRejectsMoreThanSixPermissionIDsAfterDeduplication(t *testing.T) {
	service := NewService(&fakeRepository{})

	_, appErr := service.Save(context.Background(), 12, SaveInput{PermissionIDs: []int64{1, 2, 3, 4, 5, 6, 7}})
	if appErr == nil {
		t.Fatalf("expected more than six permission ids to fail")
	}
}

func TestSaveDeduplicatesPermissionIDsPreservingOrder(t *testing.T) {
	repo := &fakeRepository{
		activeIDs: map[int64]struct{}{3: {}, 1: {}, 2: {}},
		replaceResponse: []QuickEntry{
			{ID: 10, PermissionID: 3, Sort: 1},
			{ID: 11, PermissionID: 1, Sort: 2},
			{ID: 12, PermissionID: 2, Sort: 3},
		},
	}
	service := NewService(repo)

	got, appErr := service.Save(context.Background(), 44, SaveInput{PermissionIDs: []int64{3, 1, 3, 2, 1}})
	if appErr != nil {
		t.Fatalf("expected save to succeed, got %v", appErr)
	}
	if repo.replacedUserID != 44 {
		t.Fatalf("user id mismatch: %d", repo.replacedUserID)
	}
	if !reflect.DeepEqual(repo.replacedIDs, []int64{3, 1, 2}) {
		t.Fatalf("permission ids were not deduplicated in order: %#v", repo.replacedIDs)
	}
	if len(got.QuickEntry) != 3 || got.QuickEntry[0].PermissionID != 3 || got.QuickEntry[2].Sort != 3 {
		t.Fatalf("quick_entry response mismatch: %#v", got.QuickEntry)
	}
}

func TestSaveRejectsInactiveOrNonPagePermissions(t *testing.T) {
	repo := &fakeRepository{activeIDs: map[int64]struct{}{1: {}, 3: {}}}
	service := NewService(repo)

	if _, appErr := service.Save(context.Background(), 44, SaveInput{PermissionIDs: []int64{1, 2, 3}}); appErr == nil {
		t.Fatalf("expected invalid permission id to fail")
	}
	if repo.replaceCalls != 0 {
		t.Fatalf("invalid input must not replace entries, calls=%d", repo.replaceCalls)
	}
}

func TestSaveWrapsRepositoryError(t *testing.T) {
	service := NewService(&fakeRepository{err: errors.New("db down")})

	if _, appErr := service.Save(context.Background(), 44, SaveInput{PermissionIDs: []int64{1}}); appErr == nil {
		t.Fatalf("expected repository error to fail")
	}
}
