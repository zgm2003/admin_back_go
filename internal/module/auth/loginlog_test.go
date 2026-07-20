package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"
)

type fakeLoginLogRepository struct {
	query LoginLogListQuery
	rows  []LoginLogListRow
	total int64
	err   error
}

func (f *fakeLoginLogRepository) List(ctx context.Context, query LoginLogListQuery) ([]LoginLogListRow, int64, error) {
	f.query = query
	return f.rows, f.total, f.err
}

func TestLoginLogPageInitReturnsPlatformAndLoginTypeDicts(t *testing.T) {
	service := NewLoginLogService(&fakeLoginLogRepository{})

	got, appErr := service.PageInit(context.Background())
	if appErr != nil {
		t.Fatalf("expected page-init to succeed, got %v", appErr)
	}
	if len(got.Dict.PlatformArr) != 1 || got.Dict.PlatformArr[0].Value != enum.PlatformAdmin {
		t.Fatalf("platform dict mismatch: %#v", got.Dict.PlatformArr)
	}
	if len(got.Dict.LoginTypeArr) != 3 || got.Dict.LoginTypeArr[0].Value != "email" || got.Dict.LoginTypeArr[2].Value != "password" {
		t.Fatalf("login type dict mismatch: %#v", got.Dict.LoginTypeArr)
	}
}

func TestLoginLogListNormalizesQueryAndDateBounds(t *testing.T) {
	userID := int64(12)
	createdAt := time.Date(2026, 5, 8, 9, 30, 0, 0, time.Local)
	repo := &fakeLoginLogRepository{
		total: 1,
		rows: []LoginLogListRow{{
			ID: 7, UserID: &userID, Username: "admin", LoginAccount: "admin@example.com",
			LoginType: "password", Platform: "admin", IP: "127.0.0.1", UserAgent: "ua",
			IsSuccess: 1, Reason: "", CreatedAt: createdAt,
		}},
	}
	service := NewLoginLogService(repo)

	got, appErr := service.List(context.Background(), LoginLogListQuery{
		CurrentPage:  0,
		PageSize:     999,
		UserID:       userID,
		LoginAccount: " admin ",
		LoginType:    "password",
		IP:           " 127.0 ",
		Platform:     "admin",
		IsSuccess:    loginLogIntPtr(1),
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

func TestLoginLogListAllowsMissingUserNameAsEmptyString(t *testing.T) {
	repo := &fakeLoginLogRepository{rows: []LoginLogListRow{{ID: 8, LoginType: "email", Platform: "app", IsSuccess: 2}}, total: 1}
	service := NewLoginLogService(repo)

	got, appErr := service.List(context.Background(), LoginLogListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if got.List[0].UserName != "" {
		t.Fatalf("missing user should map to empty string, got %#v", got.List[0])
	}
}

func TestLoginLogListRejectsInvalidFilters(t *testing.T) {
	service := NewLoginLogService(&fakeLoginLogRepository{})

	if _, appErr := service.List(context.Background(), LoginLogListQuery{CurrentPage: 1, PageSize: 20, LoginType: "sms"}); appErr == nil || appErr.MessageID != "userloginlog.login_type.invalid" {
		t.Fatalf("expected keyed invalid login_type error, got %#v", appErr)
	}
	if _, appErr := service.List(context.Background(), LoginLogListQuery{CurrentPage: 1, PageSize: 20, Platform: "mini"}); appErr == nil || appErr.MessageID != "userloginlog.platform.invalid" {
		t.Fatalf("expected keyed invalid platform error, got %#v", appErr)
	}
	if _, appErr := service.List(context.Background(), LoginLogListQuery{CurrentPage: 1, PageSize: 20, IsSuccess: loginLogIntPtr(9)}); appErr == nil || appErr.MessageID != "userloginlog.result.invalid" {
		t.Fatalf("expected keyed invalid is_success error, got %#v", appErr)
	}
	if _, appErr := service.List(context.Background(), LoginLogListQuery{CurrentPage: 1, PageSize: 20, DateStart: "2026/05/01"}); appErr == nil || appErr.MessageID != "userloginlog.date_start.invalid" {
		t.Fatalf("expected keyed invalid date_start error, got %#v", appErr)
	}
	if _, appErr := service.List(context.Background(), LoginLogListQuery{CurrentPage: 1, PageSize: 20, DateEnd: "2026/05/08"}); appErr == nil || appErr.MessageID != "userloginlog.date_end.invalid" {
		t.Fatalf("expected keyed invalid date_end error, got %#v", appErr)
	}
}

func TestLoginLogListWrapsRepositoryError(t *testing.T) {
	service := NewLoginLogService(&fakeLoginLogRepository{err: errors.New("db down")})

	if _, appErr := service.List(context.Background(), LoginLogListQuery{CurrentPage: 1, PageSize: 20}); appErr == nil || appErr.MessageID != "userloginlog.query_failed" {
		t.Fatalf("expected keyed repository error, got %#v", appErr)
	}
}

func loginLogIntPtr(value int) *int {
	return &value
}
