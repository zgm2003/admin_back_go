package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/session"

	"github.com/gin-gonic/gin"
)

type fakeSessionService struct {
	loginInput     LoginInput
	loginResult    *LoginResponse
	loginErr       *apperror.Error
	sendCodeInput  SendCodeInput
	sendCodeMsg    string
	sendCodeErr    *apperror.Error
	configPlatform string
	configResult   *LoginConfigResponse
	configErr      *apperror.Error
	refreshInput   session.RefreshInput
	refreshResult  *session.TokenResult
	refreshErr     *apperror.Error
	logoutToken    string
	logoutErr      *apperror.Error
}

func (f *fakeSessionService) Login(ctx context.Context, input LoginInput) (*LoginResponse, *apperror.Error) {
	f.loginInput = input
	return f.loginResult, f.loginErr
}

func (f *fakeSessionService) SendCode(ctx context.Context, input SendCodeInput) (string, *apperror.Error) {
	f.sendCodeInput = input
	return f.sendCodeMsg, f.sendCodeErr
}

func (f *fakeSessionService) LoginConfig(ctx context.Context, platform string) (*LoginConfigResponse, *apperror.Error) {
	f.configPlatform = platform
	return f.configResult, f.configErr
}

func (f *fakeSessionService) Refresh(ctx context.Context, input session.RefreshInput) (*session.TokenResult, *apperror.Error) {
	f.refreshInput = input
	return f.refreshResult, f.refreshErr
}

func (f *fakeSessionService) Logout(ctx context.Context, accessToken string) *apperror.Error {
	f.logoutToken = accessToken
	return f.logoutErr
}

func TestHandlerRefreshRequiresRefreshToken(t *testing.T) {
	router := newAuthTestRouter(&fakeSessionService{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeAuthBody(t, recorder)
	if body["msg"] != "缺少刷新令牌" {
		t.Fatalf("expected missing refresh token message, got %#v", body["msg"])
	}
}

func TestHandlerLoginConfigUsesPlatformHeader(t *testing.T) {
	service := &fakeSessionService{configResult: &LoginConfigResponse{
		LoginTypeArr:   []LoginTypeOption{{Label: "密码登录", Value: "password"}},
		CaptchaEnabled: true,
		CaptchaType:    captcha.TypeSlide,
	}}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth/login-config", nil)
	request.Header.Set("platform", "admin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.configPlatform != "admin" {
		t.Fatalf("expected platform header admin, got %q", service.configPlatform)
	}
	body := decodeAuthBody(t, recorder)
	data := body["data"].(map[string]any)
	options := data["login_type_arr"].([]any)
	if len(options) != 1 || options[0].(map[string]any)["value"] != "password" {
		t.Fatalf("unexpected login config: %#v", data)
	}
	if data["captcha_enabled"] != true || data["captcha_type"] != captcha.TypeSlide {
		t.Fatalf("unexpected captcha config: %#v", data)
	}
}

func TestHandlerSendCodeUsesGoRestContract(t *testing.T) {
	service := &fakeSessionService{sendCodeMsg: "验证码发送成功(测试:123456)"}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/send-code", strings.NewReader(`{"account":"15671628271","scene":"login"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.sendCodeInput.Account != "15671628271" || service.sendCodeInput.Scene != VerifyCodeSceneLogin {
		t.Fatalf("unexpected send code input: %#v", service.sendCodeInput)
	}
	body := decodeAuthBody(t, recorder)
	if body["msg"] != "验证码发送成功(测试:123456)" {
		t.Fatalf("unexpected send-code message: %#v", body)
	}
}

func TestHandlerLoginReturnsTokenResult(t *testing.T) {
	service := &fakeSessionService{loginResult: &LoginResponse{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/login", strings.NewReader(`{"login_account":"15671628271","login_type":"password","password":"123456","captcha_id":"captcha-id","captcha_answer":{"x":120,"y":80}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("platform", "admin")
	request.Header.Set("device-id", "device-1")
	request.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.loginInput.LoginAccount != "15671628271" ||
		service.loginInput.LoginType != "password" ||
		service.loginInput.Password != "123456" ||
		service.loginInput.CaptchaID != "captcha-id" ||
		service.loginInput.CaptchaAnswer == nil ||
		service.loginInput.CaptchaAnswer.X != 120 ||
		service.loginInput.CaptchaAnswer.Y != 80 ||
		service.loginInput.Platform != "admin" ||
		service.loginInput.DeviceID != "device-1" ||
		service.loginInput.UserAgent != "test-agent" {
		t.Fatalf("unexpected login input: %#v", service.loginInput)
	}
	body := decodeAuthBody(t, recorder)
	data := body["data"].(map[string]any)
	if data["access_token"] != "access-token" || data["refresh_token"] != "refresh-token" {
		t.Fatalf("unexpected token response: %#v", data)
	}
}

func TestHandlerCodeLoginDoesNotRequirePasswordCaptchaFields(t *testing.T) {
	service := &fakeSessionService{loginResult: &LoginResponse{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
		IsNewUser:        true,
	}}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/login", strings.NewReader(`{"login_account":"15671628271","login_type":"phone","code":"123456"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("platform", "admin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.loginInput.LoginType != LoginTypePhone || service.loginInput.Code != "123456" || service.loginInput.Password != "" || service.loginInput.CaptchaID != "" || service.loginInput.CaptchaAnswer != nil {
		t.Fatalf("unexpected code login input: %#v", service.loginInput)
	}
	body := decodeAuthBody(t, recorder)
	data := body["data"].(map[string]any)
	if data["is_new_user"] != true {
		t.Fatalf("expected is_new_user true, got %#v", data)
	}
}

func TestHandlerLoginRejectsInvalidEnumInputBeforeService(t *testing.T) {
	service := &fakeSessionService{}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/login", strings.NewReader(`{"login_account":"15671628271","login_type":"wechat","password":"123456","captcha_id":"captcha-id","captcha_answer":{"x":120,"y":80}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.loginInput.LoginAccount != "" {
		t.Fatalf("service should not be called for invalid login_type: %#v", service.loginInput)
	}
}

func TestHandlerRefreshReturnsTokenResult(t *testing.T) {
	service := &fakeSessionService{refreshResult: &session.TokenResult{
		AccessToken:      "new-access",
		RefreshToken:     "new-refresh",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", strings.NewReader(`{"refresh_token":"old-refresh"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.refreshInput.RefreshToken != "old-refresh" || service.refreshInput.UserAgent != "test-agent" {
		t.Fatalf("unexpected refresh input: %#v", service.refreshInput)
	}
	body := decodeAuthBody(t, recorder)
	data := body["data"].(map[string]any)
	if data["access_token"] != "new-access" || data["refresh_token"] != "new-refresh" {
		t.Fatalf("unexpected token response: %#v", data)
	}
}

func TestHandlerLogoutParsesBearerToken(t *testing.T) {
	service := &fakeSessionService{}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/logout", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.logoutToken != "access-token" {
		t.Fatalf("expected logout token access-token, got %q", service.logoutToken)
	}
	body := decodeAuthBody(t, recorder)
	if body["msg"] != "退出成功" {
		t.Fatalf("expected logout success message, got %#v", body["msg"])
	}
}

func newAuthTestRouter(service SessionService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	RegisterRoutes(router, service)
	return router
}

func decodeAuthBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}
