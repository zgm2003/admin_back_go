package userloginlog

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	query ListQuery
	rows  []ListRow
	total int64
	err   error
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.query = query
	return f.rows, f.total, f.err
}

func TestPageInitReturnsPlatformAndLoginTypeDicts(t *testing.T) {
	service := NewService(&fakeRepository{})

	got, appErr := service.PageInit(context.Background())
	if appErr != nil {
		t.Fatalf("expected page-init to succeed, got %v", appErr)
	}
	if len(got.Dict.PlatformArr) == 0 || got.Dict.PlatformArr[0].Value != "admin" {
		t.Fatalf("platform dict mismatch: %#v", got.Dict.PlatformArr)
	}
	if len(got.Dict.LoginTypeArr) != 3 || got.Dict.LoginTypeArr[0].Value != "email" || got.Dict.LoginTypeArr[2].Value != "password" {
		t.Fatalf("login type dict mismatch: %#v", got.Dict.LoginTypeArr)
	}
}

func TestListNormalizesQueryAndDateBounds(t *testing.T) {
	userID := int64(12)
	createdAt := time.Date(2026, 5, 8, 9, 30, 0, 0, time.Local)
	repo := &fakeRepository{
		total: 1,
		rows: []ListRow{{
			ID: 7, UserID: &userID, Username: "admin", LoginAccount: "admin@example.com",
			LoginType: "password", Platform: "admin", IP: "127.0.0.1", UserAgent: "ua",
			IsSuccess: 1, Reason: "", CreatedAt: createdAt,
		}},
	}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), ListQuery{
		CurrentPage:  0,
		PageSize:     999,
		UserID:       userID,
		LoginAccount: " admin ",
		LoginType:    "password",
		IP:           " 127.0 ",
		Platform:     "admin",
		IsSuccess:    intPtr(1),
		DateStart:    "2026-05-01",
		DateEnd:      "2026-05-08",
	})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if repo.query.CurrentPage != 1 || repo.query.PageSize != 50 {
		t.Fatalf("pagination was not normalized: %#v", repo.query)
	}
	if repo.query.LoginAccount != "admin" || repo.query.IP != "127.0" {
		t.Fatalf("string filters were not trimmed: %#v", repo.query)
	}
	if repo.query.CreatedStart != "2026-05-01 00:00:00" || repo.query.CreatedEnd != "2026-05-08 23:59:59" {
		t.Fatalf("date bounds mismatch: %#v", repo.query)
	}
	if got.Page.Total != 1 || got.Page.TotalPage != 1 {
		t.Fatalf("page mismatch: %#v", got.Page)
	}
	if len(got.List) != 1 || got.List[0].UserName != "admin" || got.List[0].LoginTypeName != "密码登录" || got.List[0].PlatformName != "admin" {
		t.Fatalf("list item mismatch: %#v", got.List)
	}
}

func TestListAllowsMissingUserNameAsEmptyString(t *testing.T) {
	repo := &fakeRepository{rows: []ListRow{{ID: 8, LoginType: "email", Platform: "app", IsSuccess: 2}}, total: 1}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if got.List[0].UserName != "" {
		t.Fatalf("missing user should map to empty string, got %#v", got.List[0])
	}
}

func TestListRejectsInvalidFilters(t *testing.T) {
	service := NewService(&fakeRepository{})

	if _, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20, LoginType: "sms"}); appErr == nil {
		t.Fatalf("expected invalid login_type to fail")
	}
	if _, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20, Platform: "mini"}); appErr == nil {
		t.Fatalf("expected invalid platform to fail")
	}
	if _, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20, IsSuccess: intPtr(9)}); appErr == nil {
		t.Fatalf("expected invalid is_success to fail")
	}
	if _, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20, DateStart: "2026/05/01"}); appErr == nil {
		t.Fatalf("expected invalid date_start to fail")
	}
}

func TestListWrapsRepositoryError(t *testing.T) {
	service := NewService(&fakeRepository{err: errors.New("db down")})

	if _, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20}); appErr == nil {
		t.Fatalf("expected repository error to fail")
	}
}

func intPtr(value int) *int {
	return &value
}
