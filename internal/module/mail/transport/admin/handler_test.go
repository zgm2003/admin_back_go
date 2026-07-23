package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mailmodule "admin_back_go/internal/module/mail"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

type fakeMailHTTPService struct {
	pageInitResult *mailmodule.PageInitResponse
	configResult   *mailmodule.ConfigResponse
	logsResult     *mailmodule.LogListResponse
	logResult      *mailmodule.LogDTO
	logsErr        *apperror.Error
	logErr         *apperror.Error
	logsCalls      int
	logCalls       int
	savedConfig    mailmodule.SaveConfigInput
	deletedIDs     []uint64
}

func (f *fakeMailHTTPService) PageInit(ctx context.Context) (*mailmodule.PageInitResponse, *apperror.Error) {
	if f.pageInitResult != nil {
		return f.pageInitResult, nil
	}
	return (&mailmodule.Service{}).PageInit(ctx)
}

func (f *fakeMailHTTPService) Config(ctx context.Context) (*mailmodule.ConfigResponse, *apperror.Error) {
	return f.configResult, nil
}

func (f *fakeMailHTTPService) SaveConfig(ctx context.Context, input mailmodule.SaveConfigInput) *apperror.Error {
	f.savedConfig = input
	return nil
}
func (f *fakeMailHTTPService) DeleteConfig(ctx context.Context) *apperror.Error { return nil }
func (f *fakeMailHTTPService) TestSend(ctx context.Context, input mailmodule.TestInput) *apperror.Error {
	return nil
}
func (f *fakeMailHTTPService) Templates(ctx context.Context) ([]mailmodule.TemplateDTO, *apperror.Error) {
	return nil, nil
}
func (f *fakeMailHTTPService) CreateTemplate(ctx context.Context, input mailmodule.SaveTemplateInput) (uint64, *apperror.Error) {
	return 1, nil
}
func (f *fakeMailHTTPService) UpdateTemplate(ctx context.Context, id uint64, input mailmodule.SaveTemplateInput) *apperror.Error {
	return nil
}
func (f *fakeMailHTTPService) ChangeTemplateStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return nil
}
func (f *fakeMailHTTPService) DeleteTemplate(ctx context.Context, id uint64) *apperror.Error {
	return nil
}
func (f *fakeMailHTTPService) Logs(ctx context.Context, query mailmodule.LogQuery) (*mailmodule.LogListResponse, *apperror.Error) {
	f.logsCalls++
	if f.logsErr != nil {
		return nil, f.logsErr
	}
	if f.logsResult != nil {
		return f.logsResult, nil
	}
	return &mailmodule.LogListResponse{}, nil
}
func (f *fakeMailHTTPService) Log(ctx context.Context, id uint64) (*mailmodule.LogDTO, *apperror.Error) {
	f.logCalls++
	return f.logResult, f.logErr
}
func (f *fakeMailHTTPService) DeleteLogs(ctx context.Context, ids []uint64) *apperror.Error {
	f.deletedIDs = ids
	return nil
}

func TestHandlerPageInitReturnsDataDict(t *testing.T) {
	router := newMailTestRouter(&fakeMailHTTPService{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/mail/page-init", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			Dict mailmodule.PageInitDict `json:"dict"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 || len(body.Data.Dict.MailSceneArr) != 4 || body.Data.Dict.DefaultEndpoint != mailmodule.DefaultEndpoint {
		t.Fatalf("unexpected page-init response: %#v", body)
	}
}

func TestHandlerConfigResponseDoesNotExposeEncryptedSecrets(t *testing.T) {
	service := &fakeMailHTTPService{configResult: &mailmodule.ConfigResponse{Configured: true, SecretIDHint: "***t-id", SecretKeyHint: "***-key", Region: mailmodule.DefaultRegion, Endpoint: mailmodule.DefaultEndpoint, FromEmail: "noreply@example.com", Status: enum.CommonYes}}
	router := newMailTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/mail/config", nil)
	router.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, body)
	}
	for _, forbidden := range []string{"secret_id_enc", "secret_key_enc", "cipher", "AKID"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("config response leaked %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, "secret_id_hint") || !strings.Contains(body, "secret_key_hint") {
		t.Fatalf("config response must expose only secret hints: %s", body)
	}
}

func TestHandlerLogResponseDoesNotExposeTemplateDataOrVerifyCode(t *testing.T) {
	service := &fakeMailHTTPService{logResult: &mailmodule.LogDTO{ID: 7, Scene: enum.VerifyCodeSceneLogin, ToEmail: "user@example.com", Subject: "Login", Status: enum.MailLogStatusSuccess, TencentRequestID: "req"}}
	router := newMailTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/mail/logs/7", nil)
	router.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, body)
	}
	for _, forbidden := range []string{"template_data", "TemplateData", "verify_code", "654321"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("log response leaked %q: %s", forbidden, body)
		}
	}
}

func TestHandlerSaveConfigBindsPublicSecretFields(t *testing.T) {
	service := &fakeMailHTTPService{}
	router := newMailTestRouter(service)
	payload := `{"secret_id":"AKID-input","secret_key":"SECRET-input","region":"ap-guangzhou","endpoint":"ses.tencentcloudapi.com","from_email":"noreply@example.com","from_name":"Admin","reply_to":"reply@example.com","status":1,"verify_code_ttl_minutes":9}`

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/mail/config", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.savedConfig.SecretID != "AKID-input" ||
		service.savedConfig.SecretKey != "SECRET-input" ||
		service.savedConfig.FromEmail != "noreply@example.com" ||
		service.savedConfig.VerifyCodeTTLMinutes != 9 {
		t.Fatalf("unexpected saved config input: %#v", service.savedConfig)
	}
}

func TestHandlerLogsNoStore(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		service *fakeMailHTTPService
	}{
		{name: "success", target: "/api/admin/v1/mail/logs", service: &fakeMailHTTPService{}},
		{name: "bind error", target: "/api/admin/v1/mail/logs?status=not-a-number", service: &fakeMailHTTPService{}},
		{name: "query error", target: "/api/admin/v1/mail/logs?created_at_start=invalid", service: &fakeMailHTTPService{}},
		{name: "service error", target: "/api/admin/v1/mail/logs", service: &fakeMailHTTPService{logsErr: apperror.Internal("mail log read failed")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newMailTestRouter(tt.service)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.target, nil))
			assertMailDiagnosticNoStore(t, recorder)
		})
	}
}

func TestHandlerLogNoStore(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		service *fakeMailHTTPService
	}{
		{name: "success", target: "/api/admin/v1/mail/logs/7", service: &fakeMailHTTPService{logResult: &mailmodule.LogDTO{ID: 7}}},
		{name: "id error", target: "/api/admin/v1/mail/logs/invalid", service: &fakeMailHTTPService{}},
		{name: "service error", target: "/api/admin/v1/mail/logs/7", service: &fakeMailHTTPService{logErr: apperror.Internal("mail log read failed")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newMailTestRouter(tt.service)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.target, nil))
			assertMailDiagnosticNoStore(t, recorder)
		})
	}
}

func assertMailDiagnosticNoStore(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if got := recorder.Header().Get("Cache-Control"); got != "no-store, private" {
		t.Fatalf("Cache-Control=%q, want %q", got, "no-store, private")
	}
	if got := recorder.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma=%q, want %q", got, "no-cache")
	}
}

func newMailTestRouter(service HTTPService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, service)
	return router
}

var _ HTTPService = (*fakeMailHTTPService)(nil)
