package operationlog

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/middleware"
)

type fakeRepository struct {
	row       Log
	listQuery ListQuery
	listRows  []ListRow
	listTotal int64
	deleted   []int64
	err       error
}

func (f *fakeRepository) Create(ctx context.Context, row Log) error {
	f.row = row
	return f.err
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.listQuery = query
	return f.listRows, f.listTotal, f.err
}

func (f *fakeRepository) Delete(ctx context.Context, ids []int64) error {
	f.deleted = ids
	return f.err
}

func TestRecorderPersistsOperationMetadataWithoutRequestBodyFallback(t *testing.T) {
	repo := &fakeRepository{}
	recorder := NewRecorder(repo)

	err := recorder(context.Background(), middleware.OperationInput{
		UserID:    1,
		SessionID: 10,
		Platform:  "admin",
		Method:    "POST",
		Path:      "/api/admin/v1/permissions",
		Module:    "permission",
		Action:    "create",
		Title:     "新增菜单",
		RequestID: "rid-1",
		ClientIP:  "127.0.0.1",
		Status:    200,
		Success:   true,
		LatencyMs: 12,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.row.UserID != 1 || repo.row.Action != "新增菜单" || repo.row.IsSuccess != 1 || repo.row.IsDel != 2 {
		t.Fatalf("log row mismatch: %#v", repo.row)
	}

	var request map[string]any
	if err := json.Unmarshal([]byte(repo.row.RequestData), &request); err != nil {
		t.Fatalf("invalid request data json: %v", err)
	}
	if request["method"] != "POST" || request["path"] != "/api/admin/v1/permissions" || request["module"] != "permission" {
		t.Fatalf("request metadata mismatch: %#v", request)
	}
	if _, ok := request["body"]; ok {
		t.Fatalf("operation log must not invent request body fallback: %#v", request)
	}
}

func TestRecorderMasksExplicitSensitivePayloadFields(t *testing.T) {
	repo := &fakeRepository{}
	recorder := NewRecorder(repo)

	err := recorder(context.Background(), middleware.OperationInput{
		UserID:  1,
		Method:  "POST",
		Path:    "/api/admin/v1/auth/login",
		Module:  "auth",
		Action:  "login",
		Title:   "登录",
		Status:  400,
		Success: false,
		RequestPayload: map[string]any{
			"login_account":  "15671628271",
			"password":       "123456",
			"secret_id_enc":  "cipher-id",
			"secret_key_enc": "cipher-key",
			"captcha_answer": map[string]any{
				"x": 99,
			},
			"nested": []any{
				map[string]any{"refresh_token": "token"},
			},
			"code": "123456",
		},
		ResponsePayload: map[string]any{
			"code": 100,
			"msg":  "验证码错误或已过期",
			"data": map[string]any{"access_token": "token"},
		},
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var request map[string]any
	if err := json.Unmarshal([]byte(repo.row.RequestData), &request); err != nil {
		t.Fatalf("invalid request data json: %v", err)
	}
	payload := request["payload"].(map[string]any)
	if payload["password"] != "******" || payload["code"] != "******" {
		t.Fatalf("request sensitive fields were not masked: %#v", payload)
	}
	if payload["secret_id_enc"] != "******" || payload["secret_key_enc"] != "******" {
		t.Fatalf("request encrypted secret fields were not masked: %#v", payload)
	}
	captcha := payload["captcha_answer"].(map[string]any)
	if captcha["x"] != "******" {
		t.Fatalf("captcha answer was not masked recursively: %#v", captcha)
	}
	nested := payload["nested"].([]any)[0].(map[string]any)
	if nested["refresh_token"] != "******" {
		t.Fatalf("nested refresh token was not masked: %#v", nested)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(repo.row.ResponseData), &response); err != nil {
		t.Fatalf("invalid response data json: %v", err)
	}
	responsePayload := response["payload"].(map[string]any)
	if responsePayload["code"] != float64(100) {
		t.Fatalf("response business code should stay visible: %#v", responsePayload)
	}
	responseData := responsePayload["data"].(map[string]any)
	if responseData["access_token"] != "******" {
		t.Fatalf("response token was not masked: %#v", responseData)
	}
	if repo.row.IsSuccess != CommonNo {
		t.Fatalf("expected failed operation flag, got %#v", repo.row)
	}
}

func TestServiceListReturnsUserMetadataAndPagination(t *testing.T) {
	repo := &fakeRepository{
		listRows: []ListRow{{
			ID:           7,
			UserID:       1,
			UserName:     "admin",
			UserEmail:    "admin@example.com",
			Action:       "编辑用户",
			RequestData:  "{}",
			ResponseData: `{"status":200}`,
			IsSuccess:    CommonYes,
			CreatedAt:    mustTime(t, "2026-05-04 12:00:01"),
		}},
		listTotal: 21,
	}
	svc := NewService(repo)

	got, appErr := svc.List(context.Background(), ListQuery{
		CurrentPage: 2,
		PageSize:    10,
		UserID:      1,
		Action:      " 编辑 ",
		DateRange:   []string{"2026-05-01", "2026-05-04"},
	})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if repo.listQuery.Action != "编辑" || repo.listQuery.UserID != 1 {
		t.Fatalf("query not normalized: %#v", repo.listQuery)
	}
	if got.Page.CurrentPage != 2 || got.Page.PageSize != 10 || got.Page.Total != 21 || got.Page.TotalPage != 3 {
		t.Fatalf("page mismatch: %#v", got.Page)
	}
	if len(got.List) != 1 || got.List[0].UserName != "admin" || got.List[0].CreatedAt != "2026-05-04 12:00:01" {
		t.Fatalf("list item mismatch: %#v", got.List)
	}
}

func TestServiceDeleteNormalizesIDs(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)

	appErr := svc.Delete(context.Background(), []int64{0, 9, 9, 10})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if !reflect.DeepEqual(repo.deleted, []int64{9, 10}) {
		t.Fatalf("delete ids mismatch: %#v", repo.deleted)
	}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation(timeLayout, value, time.Local)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return parsed
}
