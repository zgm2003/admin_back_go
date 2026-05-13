package usersession

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/module/session"
)

type fakeRepository struct {
	listQuery    ListQuery
	listRows     []ListRow
	listTotal    int64
	statsRows    []StatsRow
	recordsByID  map[int64]SessionRecord
	markedIDs    []int64
	markedAt     time.Time
	markAffected int64
	listErr      error
	statsErr     error
	recordErr    error
	markErr      error
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.listQuery = query
	return f.listRows, f.listTotal, f.listErr
}

func (f *fakeRepository) Stats(ctx context.Context, now time.Time) ([]StatsRow, error) {
	return f.statsRows, f.statsErr
}

func (f *fakeRepository) GetByID(ctx context.Context, id int64) (*SessionRecord, error) {
	if f.recordErr != nil {
		return nil, f.recordErr
	}
	row, ok := f.recordsByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeRepository) GetByIDs(ctx context.Context, ids []int64) ([]SessionRecord, error) {
	if f.recordErr != nil {
		return nil, f.recordErr
	}
	rows := make([]SessionRecord, 0, len(ids))
	for _, id := range ids {
		if row, ok := f.recordsByID[id]; ok {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (f *fakeRepository) MarkRevoked(ctx context.Context, ids []int64, revokedAt time.Time) (int64, error) {
	f.markedIDs = append([]int64(nil), ids...)
	f.markedAt = revokedAt
	if f.markErr != nil {
		return 0, f.markErr
	}
	if f.markAffected == 0 {
		return int64(len(ids)), nil
	}
	return f.markAffected, nil
}

type fakeCacheRevoker struct {
	rows  []session.Session
	calls int
	err   error
}

func (f *fakeCacheRevoker) RevokeCache(ctx context.Context, row session.Session) error {
	f.calls++
	f.rows = append(f.rows, row)
	return f.err
}

func (f *fakeCacheRevoker) RevokeCaches(ctx context.Context, rows []session.Session) error {
	f.calls++
	f.rows = append(f.rows, rows...)
	return f.err
}

func TestListNormalizesQueryAndDerivesStatus(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	expiredAt := now.Add(-time.Minute)
	activeAt := now.Add(time.Hour)
	revokedAt := now.Add(-2 * time.Hour)
	repo := &fakeRepository{
		listTotal: 3,
		listRows: []ListRow{
			{
				ID: 1, UserID: 11, Username: "active-user", Platform: "admin", DeviceID: "dev-1",
				IP: "127.0.0.1", UserAgent: "ua-1", LastSeenAt: now,
				ExpiresAt: activeAt, RefreshExpiresAt: activeAt, CreatedAt: now.Add(-24 * time.Hour),
			},
			{
				ID: 2, UserID: 12, Username: "expired-user", Platform: "app", DeviceID: "dev-2",
				IP: "127.0.0.2", UserAgent: "ua-2", LastSeenAt: now.Add(-time.Hour),
				ExpiresAt: expiredAt, RefreshExpiresAt: expiredAt, CreatedAt: now.Add(-48 * time.Hour),
			},
			{
				ID: 3, UserID: 13, Username: "revoked-user", Platform: "admin", DeviceID: "dev-3",
				IP: "::1", UserAgent: "ua-3", LastSeenAt: now.Add(-2 * time.Hour),
				ExpiresAt: activeAt, RefreshExpiresAt: activeAt, RevokedAt: &revokedAt, CreatedAt: now.Add(-72 * time.Hour),
			},
		},
	}
	service := NewService(repo, WithNow(func() time.Time { return now }))

	got, appErr := service.List(context.Background(), ListQuery{
		CurrentPage: -1,
		PageSize:    999,
		Username:    " active-user ",
		Platform:    "admin",
		Status:      "active",
	})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if repo.listQuery.CurrentPage != 1 || repo.listQuery.PageSize != 50 {
		t.Fatalf("query was not normalized: %#v", repo.listQuery)
	}
	if !repo.listQuery.Now.Equal(now) {
		t.Fatalf("query now mismatch: %s", repo.listQuery.Now)
	}
	if repo.listQuery.Username != "active-user" || repo.listQuery.Platform != "admin" || repo.listQuery.Status != "active" {
		t.Fatalf("query filters mismatch: %#v", repo.listQuery)
	}
	if got.Page.Total != 3 || got.Page.TotalPage != 1 {
		t.Fatalf("page mismatch: %#v", got.Page)
	}
	if got.List[0].Status != SessionStatusActive || got.List[1].Status != SessionStatusExpired || got.List[2].Status != SessionStatusRevoked {
		t.Fatalf("status mismatch: %#v", got.List)
	}
	if got.List[0].PlatformName != "admin" || got.List[1].PlatformName != "app" {
		t.Fatalf("platform name mismatch: %#v", got.List)
	}
}

func TestListRejectsInvalidStatusAndPlatform(t *testing.T) {
	service := NewService(&fakeRepository{}, WithNow(func() time.Time {
		return time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	}))

	if _, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20, Status: "bad"}); appErr == nil {
		t.Fatalf("expected invalid status to fail")
	}
	if _, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20, Platform: "mini"}); appErr == nil {
		t.Fatalf("expected invalid platform to fail")
	}
}

func TestStatsAlwaysReturnsAdminAndAppKeys(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	repo := &fakeRepository{statsRows: []StatsRow{
		{Platform: "admin", Total: 2},
	}}
	service := NewService(repo, WithNow(func() time.Time { return now }))

	got, appErr := service.Stats(context.Background())
	if appErr != nil {
		t.Fatalf("expected stats to succeed, got %v", appErr)
	}
	if got.TotalActive != 2 {
		t.Fatalf("total_active mismatch: %d", got.TotalActive)
	}
	if got.PlatformDistribution["admin"] != 2 || got.PlatformDistribution["app"] != 0 {
		t.Fatalf("platform_distribution mismatch: %#v", got.PlatformDistribution)
	}
}

func TestRevokeRejectsCurrentSession(t *testing.T) {
	service := NewService(&fakeRepository{}, WithNow(func() time.Time {
		return time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	}))

	if _, appErr := service.Revoke(context.Background(), 55, 55); appErr == nil {
		t.Fatalf("expected current session revoke to fail")
	}
}

func TestRevokeReturnsFalseForAlreadyRevokedSession(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	revokedAt := now.Add(-time.Hour)
	repo := &fakeRepository{recordsByID: map[int64]SessionRecord{
		77: {ID: 77, UserID: 44, Platform: "admin", AccessTokenHash: "hash-77", RevokedAt: &revokedAt},
	}}
	revoker := &fakeCacheRevoker{}
	service := NewService(repo, WithNow(func() time.Time { return now }), WithCacheRevoker(revoker))

	got, appErr := service.Revoke(context.Background(), 77, 55)
	if appErr != nil {
		t.Fatalf("expected already revoked session to be idempotent, got %v", appErr)
	}
	if got.ID != 77 || got.Revoked {
		t.Fatalf("unexpected revoke response: %#v", got)
	}
	if len(repo.markedIDs) != 0 || revoker.calls != 0 {
		t.Fatalf("already revoked session must not touch db/cache, marked=%#v calls=%d", repo.markedIDs, revoker.calls)
	}
}

func TestRevokeMarksSessionAndRevokesCache(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	repo := &fakeRepository{recordsByID: map[int64]SessionRecord{
		77: {ID: 77, UserID: 44, Platform: "admin", AccessTokenHash: "hash-77"},
	}}
	revoker := &fakeCacheRevoker{}
	service := NewService(repo, WithNow(func() time.Time { return now }), WithCacheRevoker(revoker))

	got, appErr := service.Revoke(context.Background(), 77, 55)
	if appErr != nil {
		t.Fatalf("expected revoke to succeed, got %v", appErr)
	}
	if got.ID != 77 || !got.Revoked {
		t.Fatalf("unexpected revoke response: %#v", got)
	}
	if len(repo.markedIDs) != 1 || repo.markedIDs[0] != 77 || !repo.markedAt.Equal(now) {
		t.Fatalf("mark revoked mismatch: ids=%#v at=%s", repo.markedIDs, repo.markedAt)
	}
	if revoker.calls != 1 || len(revoker.rows) != 1 || revoker.rows[0].ID != 77 || revoker.rows[0].Platform != "admin" || revoker.rows[0].UserID != 44 {
		t.Fatalf("cache revoker mismatch: calls=%d rows=%#v", revoker.calls, revoker.rows)
	}
}

func TestBatchRevokeDeduplicatesAndSkipsCurrentAndAlreadyRevoked(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	revokedAt := now.Add(-time.Hour)
	repo := &fakeRepository{recordsByID: map[int64]SessionRecord{
		1: {ID: 1, UserID: 11, Platform: "admin", AccessTokenHash: "hash-1"},
		2: {ID: 2, UserID: 12, Platform: "admin", AccessTokenHash: "hash-2", RevokedAt: &revokedAt},
		3: {ID: 3, UserID: 13, Platform: "app", AccessTokenHash: "hash-3"},
	}}
	revoker := &fakeCacheRevoker{}
	service := NewService(repo, WithNow(func() time.Time { return now }), WithCacheRevoker(revoker))

	got, appErr := service.BatchRevoke(context.Background(), BatchRevokeInput{IDs: []int64{1, 2, 1, 3, 99}}, 3)
	if appErr != nil {
		t.Fatalf("expected batch revoke to succeed, got %v", appErr)
	}
	if got.Count != 1 || got.SkippedCurrent != 1 || got.SkippedAlreadyRevoked != 1 {
		t.Fatalf("batch response mismatch: %#v", got)
	}
	if len(repo.markedIDs) != 1 || repo.markedIDs[0] != 1 {
		t.Fatalf("only session 1 should be marked, got %#v", repo.markedIDs)
	}
	if revoker.calls != 1 || len(revoker.rows) != 1 || revoker.rows[0].ID != 1 {
		t.Fatalf("only session 1 cache should be revoked, calls=%d rows=%#v", revoker.calls, revoker.rows)
	}
}
