package clientversion

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	rows          []Version
	total         int64
	byID          map[int64]Version
	latest        map[string]Version
	exists        bool
	created       *Version
	updates       []map[string]any
	deleted       []int64
	cleared       []string
	latestIDs     []int64
	txCalls       int
	currentChecks map[string]Version
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Version, int64, error) {
	return f.rows, f.total, nil
}

func (f *fakeRepository) Get(ctx context.Context, id int64) (*Version, error) {
	if f.byID == nil {
		return nil, nil
	}
	row, ok := f.byID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeRepository) Latest(ctx context.Context, platform string) (*Version, error) {
	if f.latest == nil {
		return nil, nil
	}
	row, ok := f.latest[platform]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeRepository) FindByVersionPlatform(ctx context.Context, version string, platform string) (*Version, error) {
	if f.currentChecks == nil {
		return nil, nil
	}
	row, ok := f.currentChecks[version+"|"+platform]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeRepository) ExistsByVersionPlatform(ctx context.Context, version string, platform string, excludeID int64) (bool, error) {
	return f.exists, nil
}

func (f *fakeRepository) Create(ctx context.Context, row Version) (int64, error) {
	f.created = &row
	return 99, nil
}

func (f *fakeRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	f.updates = append(f.updates, fields)
	return nil
}

func (f *fakeRepository) SoftDelete(ctx context.Context, id int64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeRepository) ClearLatestByPlatform(ctx context.Context, platform string) error {
	f.cleared = append(f.cleared, platform)
	return nil
}

func (f *fakeRepository) SetLatest(ctx context.Context, id int64) error {
	f.latestIDs = append(f.latestIDs, id)
	return nil
}

func (f *fakeRepository) WithTransaction(ctx context.Context, fn func(ctx context.Context, repo Repository) error) error {
	f.txCalls++
	return fn(ctx, f)
}

type fakePublisher struct {
	fail     error
	platform string
	body     string
	calls    int
}

func (f *fakePublisher) Publish(ctx context.Context, platform string, body []byte) error {
	f.calls++
	f.platform = platform
	f.body = string(body)
	return f.fail
}

func TestCreateRejectsDuplicateVersionPlatform(t *testing.T) {
	service := NewService(&fakeRepository{exists: true}, nil)

	_, appErr := service.Create(context.Background(), CreateInput{
		Version: "1.0.8", Platform: enum.ClientPlatformWindowsX8664, FileURL: "https://example.com/app.exe", Signature: "sig",
	})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.MessageID != "clientversion.version.duplicate" {
		t.Fatalf("expected duplicate version error, got %#v", appErr)
	}
}

func TestCreateDefaultsLatestAndForceUpdate(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, nil)

	id, appErr := service.Create(context.Background(), CreateInput{
		Version: " 1.0.8 ", Platform: enum.ClientPlatformWindowsX8664, FileURL: "https://example.com/app.exe", Signature: " sig ",
	})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 99 || repo.created == nil {
		t.Fatalf("expected created row, id=%d row=%#v", id, repo.created)
	}
	if repo.created.Version != "1.0.8" || repo.created.Signature != "sig" || repo.created.IsLatest != enum.CommonNo || repo.created.ForceUpdate != enum.CommonNo || repo.created.IsDel != enum.CommonNo {
		t.Fatalf("unexpected created row: %#v", repo.created)
	}
}

func TestSetLatestClearsSamePlatformAndPublishesManifest(t *testing.T) {
	updatedAt := time.Date(2026, 5, 6, 20, 0, 0, 0, time.FixedZone("CST", 8*3600))
	repo := &fakeRepository{byID: map[int64]Version{8: {
		ID: 8, Version: "1.0.7", Platform: enum.ClientPlatformWindowsX8664, Notes: "release", FileURL: "https://example.com/app.exe", Signature: "sig", UpdatedAt: updatedAt,
	}}}
	publisher := &fakePublisher{}
	service := NewService(repo, publisher)

	appErr := service.SetLatest(context.Background(), 8)
	if appErr != nil {
		t.Fatalf("expected set latest to succeed, got %v", appErr)
	}
	if repo.txCalls != 1 || len(repo.cleared) != 1 || repo.cleared[0] != enum.ClientPlatformWindowsX8664 || len(repo.latestIDs) != 1 || repo.latestIDs[0] != 8 {
		t.Fatalf("unexpected latest transaction, tx=%d cleared=%#v latest=%#v", repo.txCalls, repo.cleared, repo.latestIDs)
	}
	if publisher.calls != 1 || publisher.platform != enum.ClientPlatformWindowsX8664 {
		t.Fatalf("expected one manifest publish, publisher=%#v", publisher)
	}
	if !strings.Contains(publisher.body, `"version":"1.0.7"`) || !strings.Contains(publisher.body, `"windows-x86_64"`) || !strings.Contains(publisher.body, `"signature":"sig"`) {
		t.Fatalf("unexpected manifest body: %s", publisher.body)
	}
}

func TestSetLatestPublishFailureReturnsError(t *testing.T) {
	repo := &fakeRepository{byID: map[int64]Version{8: {ID: 8, Version: "1.0.7", Platform: enum.ClientPlatformWindowsX8664, FileURL: "https://example.com/app.exe", Signature: "sig"}}}
	service := NewService(repo, &fakePublisher{fail: errors.New("cos down")})

	appErr := service.SetLatest(context.Background(), 8)
	if appErr == nil || appErr.Code != apperror.CodeInternal || appErr.MessageID != "clientversion.manifest_publish_failed" {
		t.Fatalf("expected publish failure, got %#v", appErr)
	}
}

func TestDeleteRejectsLatestAndSoftDeletesNonLatest(t *testing.T) {
	repo := &fakeRepository{byID: map[int64]Version{
		1: {ID: 1, IsLatest: enum.CommonYes},
		2: {ID: 2, IsLatest: enum.CommonNo},
	}}
	service := NewService(repo, nil)

	appErr := service.Delete(context.Background(), 1)
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.MessageID != "clientversion.latest_delete_forbidden" {
		t.Fatalf("expected latest delete rejection, got %#v", appErr)
	}
	appErr = service.Delete(context.Background(), 2)
	if appErr != nil {
		t.Fatalf("expected non-latest delete to succeed, got %v", appErr)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != 2 {
		t.Fatalf("expected soft delete id 2, got %#v", repo.deleted)
	}
}

func TestForceUpdateValidatesValueAndUpdates(t *testing.T) {
	repo := &fakeRepository{byID: map[int64]Version{3: {ID: 3}}}
	service := NewService(repo, nil)

	appErr := service.ForceUpdate(context.Background(), 3, 9)
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.MessageID != "clientversion.force_update.invalid" {
		t.Fatalf("expected invalid force_update rejection, got %#v", appErr)
	}
	appErr = service.ForceUpdate(context.Background(), 3, enum.CommonYes)
	if appErr != nil {
		t.Fatalf("expected force update to succeed, got %v", appErr)
	}
	if len(repo.updates) != 1 || repo.updates[0]["force_update"] != enum.CommonYes {
		t.Fatalf("unexpected force update fields: %#v", repo.updates)
	}
}

func TestUpdateJsonUsesLatestManifestShape(t *testing.T) {
	updatedAt := time.Date(2026, 5, 6, 20, 0, 0, 0, time.FixedZone("CST", 8*3600))
	repo := &fakeRepository{latest: map[string]Version{enum.ClientPlatformWindowsX8664: {
		ID: 8, Version: "1.0.7", Platform: enum.ClientPlatformWindowsX8664, Notes: "notes", FileURL: "https://example.com/app.exe", Signature: "sig", UpdatedAt: updatedAt,
	}}}
	service := NewService(repo, nil)

	got, appErr := service.UpdateJSON(context.Background(), enum.ClientPlatformWindowsX8664)
	if appErr != nil {
		t.Fatalf("expected manifest to succeed, got %v", appErr)
	}
	manifest, ok := got.(ManifestPayload)
	if !ok {
		t.Fatalf("expected manifest payload, got %#v", got)
	}
	if manifest.Version != "1.0.7" || manifest.Platforms[enum.ClientPlatformWindowsX8664].URL != "https://example.com/app.exe" || manifest.PubDate != "2026-05-06T20:00:00+08:00" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestCurrentCheckMissingReturnsFalse(t *testing.T) {
	service := NewService(&fakeRepository{}, nil)

	got, appErr := service.CurrentCheck(context.Background(), CurrentCheckQuery{Version: "1.0.0", Platform: enum.ClientPlatformWindowsX8664})
	if appErr != nil {
		t.Fatalf("expected current check to succeed, got %v", appErr)
	}
	if got.ForceUpdate {
		t.Fatalf("missing row must not force update")
	}
}

func TestUpdateLatestPublishesManifest(t *testing.T) {
	repo := &fakeRepository{byID: map[int64]Version{8: {
		ID: 8, Version: "1.0.7", Platform: enum.ClientPlatformWindowsX8664, IsLatest: enum.CommonYes, FileURL: "https://example.com/old.exe", Signature: "old",
	}}}
	publisher := &fakePublisher{}
	service := NewService(repo, publisher)

	appErr := service.Update(context.Background(), 8, UpdateInput{
		Version: "1.0.8", Platform: enum.ClientPlatformWindowsX8664, FileURL: "https://example.com/new.exe", Signature: "new", ForceUpdate: enum.CommonNo,
	})
	if appErr != nil {
		t.Fatalf("expected update latest to succeed, got %v", appErr)
	}
	if repo.txCalls != 1 || len(repo.updates) != 1 || publisher.calls != 1 {
		t.Fatalf("expected update in tx and publish, tx=%d updates=%#v publisher=%#v", repo.txCalls, repo.updates, publisher)
	}
	if !strings.Contains(publisher.body, `"version":"1.0.8"`) || !strings.Contains(publisher.body, `"url":"https://example.com/new.exe"`) {
		t.Fatalf("unexpected published manifest: %s", publisher.body)
	}
}
