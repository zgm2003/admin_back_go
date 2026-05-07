package usersession

import (
	"context"
	"testing"
	"time"
)

type fakeRepository struct {
	listQuery ListQuery
	listRows  []ListRow
	listTotal int64
	statsRows []StatsRow
	listErr   error
	statsErr  error
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.listQuery = query
	return f.listRows, f.listTotal, f.listErr
}

func (f *fakeRepository) Stats(ctx context.Context, now time.Time) ([]StatsRow, error) {
	return f.statsRows, f.statsErr
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
