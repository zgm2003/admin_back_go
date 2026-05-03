package operationlog

import (
	"context"
	"encoding/json"
	"testing"

	"admin_back_go/internal/middleware"
)

type fakeRepository struct {
	row Log
	err error
}

func (f *fakeRepository) Create(ctx context.Context, row Log) error {
	f.row = row
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
		Path:      "/api/v1/permissions",
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
	if request["method"] != "POST" || request["path"] != "/api/v1/permissions" || request["module"] != "permission" {
		t.Fatalf("request metadata mismatch: %#v", request)
	}
	if _, ok := request["body"]; ok {
		t.Fatalf("operation log must not invent request body fallback: %#v", request)
	}
}
