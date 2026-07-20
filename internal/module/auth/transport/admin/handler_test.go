package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/middleware"
	authmodule "admin_back_go/internal/module/auth"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	projecti18n "admin_back_go/internal/shared/i18n"

	"github.com/gin-gonic/gin"
)

type fakeSessionService struct {
	loginCalls          int
	loginInput          authmodule.LoginInput
	loginResult         *authmodule.LoginResponse
	loginErr            *apperror.Error
	sendCodeInput       authmodule.SendCodeInput
	sendCodeMsg         string
	sendCodeErr         *apperror.Error
	forgetPasswordInput authmodule.ForgetPasswordInput
	forgetPasswordErr   *apperror.Error
	configPlatform      string
	configResult        *authmodule.LoginConfigResponse
	configErr           *apperror.Error
	refreshCalls        int
	refreshInput        authmodule.RefreshInput
	refreshResult       *authmodule.TokenResult
	refreshErr          *apperror.Error
	logoutCalls         int
	logoutToken         string
	logoutErr           *apperror.Error
}

func (f *fakeSessionService) Login(ctx context.Context, input authmodule.LoginInput) (*authmodule.LoginResponse, *apperror.Error) {
	f.loginCalls++
	f.loginInput = input
	return f.loginResult, f.loginErr
}

func (f *fakeSessionService) SendCode(ctx context.Context, input authmodule.SendCodeInput) (string, *apperror.Error) {
	f.sendCodeInput = input
	return f.sendCodeMsg, f.sendCodeErr
}

func (f *fakeSessionService) ForgetPassword(ctx context.Context, input authmodule.ForgetPasswordInput) *apperror.Error {
	f.forgetPasswordInput = input
	return f.forgetPasswordErr
}

func (f *fakeSessionService) LoginConfig(ctx context.Context, platform string) (*authmodule.LoginConfigResponse, *apperror.Error) {
	f.configPlatform = platform
	return f.configResult, f.configErr
}

func (f *fakeSessionService) Refresh(ctx context.Context, input authmodule.RefreshInput) (*authmodule.TokenResult, *apperror.Error) {
	f.refreshCalls++
	f.refreshInput = input
	return f.refreshResult, f.refreshErr
}

func (f *fakeSessionService) Logout(ctx context.Context, accessToken string) *apperror.Error {
	f.logoutCalls++
	f.logoutToken = accessToken
	return f.logoutErr
}

type fakeCaptchaService struct {
	result *authmodule.ChallengeResponse
	err    *apperror.Error
}

type fakeBrowserGrantIssuer struct {
	realtimeSubject authmodule.GrantSubject
	queueSubject    authmodule.GrantSubject
}

func (f *fakeBrowserGrantIssuer) IssueRealtimeTicket(_ context.Context, subject authmodule.GrantSubject) (*authmodule.BrowserGrant, *apperror.Error) {
	f.realtimeSubject = subject
	return &authmodule.BrowserGrant{Credential: "realtime-ticket", ExpiresIn: 30}, nil
}

func (f *fakeBrowserGrantIssuer) IssueQueueMonitorGrant(_ context.Context, subject authmodule.GrantSubject) (*authmodule.BrowserGrant, *apperror.Error) {
	f.queueSubject = subject
	return &authmodule.BrowserGrant{Credential: "queue-grant", ExpiresIn: 60}, nil
}

func (f fakeCaptchaService) Generate(ctx context.Context) (*authmodule.ChallengeResponse, *apperror.Error) {
	return f.result, f.err
}

func TestHandlerCaptchaReturnsSlideChallenge(t *testing.T) {
	service := &fakeSessionService{}
	captchaService := fakeCaptchaService{result: &authmodule.ChallengeResponse{
		CaptchaID:   "captcha-id",
		CaptchaType: authmodule.TypeSlide,
		MasterImage: "data:image/jpeg;base64,master",
		TileImage:   "data:image/png;base64,tile",
		TileX:       7,
		TileY:       53,
		TileWidth:   62,
		TileHeight:  62,
		ImageWidth:  300,
		ImageHeight: 220,
		ExpiresIn:   120,
	}}
	router := newAuthTestRouterWithCaptcha(service, captchaService)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth/captcha", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeAuthBody(t, recorder)
	data := body["data"].(map[string]any)
	if data["captcha_id"] != "captcha-id" || data["captcha_type"] != authmodule.TypeSlide {
		t.Fatalf("unexpected captcha response: %#v", data)
	}
}

func TestHandlerCaptchaLocalizesMissingService(t *testing.T) {
	router := newAuthTestRouterWithCaptcha(&fakeSessionService{}, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth/captcha", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeAuthBody(t, recorder)
	if body["msg"] != "Captcha service is not configured" {
		t.Fatalf("expected localized msg, got %#v", body["msg"])
	}
}

func TestHandlerRefreshRejectsEveryRequestBodyBeforeCredentialWork(t *testing.T) {
	for _, body := range []string{`{}`, `{"refresh_token":"legacy"}`} {
		t.Run(body, func(t *testing.T) {
			service := &fakeSessionService{}
			router := newAuthTestRouter(service)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "http://localhost:5173")
			request.AddCookie(&http.Cookie{Name: BrowserRefreshCookieName, Value: "cookie-refresh"})
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d body=%s", recorder.Code, recorder.Body.String())
			}
			responseBody := decodeAuthBody(t, recorder)
			errorBody := responseBody["error"].(map[string]any)
			if errorBody["code"] != "request.invalid" || responseBody["msg"] != "刷新请求体必须为空" {
				t.Fatalf("unexpected body rejection: %#v", responseBody)
			}
			if service.refreshCalls != 0 {
				t.Fatalf("refresh service called %d time(s)", service.refreshCalls)
			}
		})
	}
}

func TestHandlerRefreshRequiresSecureCookie(t *testing.T) {
	service := &fakeSessionService{}
	router := newAuthTestRouter(service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.refreshCalls != 0 {
		t.Fatalf("refresh service called %d time(s)", service.refreshCalls)
	}
}

func TestHandlerLoginConfigUsesTrustedAdminPlatform(t *testing.T) {
	service := &fakeSessionService{configResult: &authmodule.LoginConfigResponse{
		LoginTypeArr:   []authmodule.LoginTypeOption{{Label: "密码登录", Value: "password"}},
		CaptchaEnabled: true,
		CaptchaType:    authmodule.TypeSlide,
	}}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth/login-config", nil)
	request.Header.Set("platform", "partner_portal")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.configPlatform != enum.PlatformAdmin {
		t.Fatalf("admin handler trusted client platform header: %q", service.configPlatform)
	}
	body := decodeAuthBody(t, recorder)
	data := body["data"].(map[string]any)
	options := data["login_type_arr"].([]any)
	if len(options) != 1 || options[0].(map[string]any)["value"] != "password" {
		t.Fatalf("unexpected login config: %#v", data)
	}
	if data["captcha_enabled"] != true || data["captcha_type"] != authmodule.TypeSlide {
		t.Fatalf("unexpected captcha config: %#v", data)
	}
}

func TestHandlerSendCodeUsesGoRestContract(t *testing.T) {
	service := &fakeSessionService{sendCodeMsg: "验证码发送成功"}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/send-code", strings.NewReader(`{"account":"15671628271","scene":"login","login_type":"phone","captcha_id":"captcha-id","captcha_answer":{"x":120,"y":80}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.sendCodeInput.Account != "15671628271" ||
		service.sendCodeInput.Scene != authmodule.VerifyCodeSceneLogin ||
		service.sendCodeInput.LoginType != authmodule.LoginTypePhone ||
		service.sendCodeInput.CaptchaID != "captcha-id" ||
		service.sendCodeInput.CaptchaAnswer == nil ||
		service.sendCodeInput.CaptchaAnswer.X != 120 ||
		service.sendCodeInput.CaptchaAnswer.Y != 80 ||
		service.sendCodeInput.ClientIP == "" ||
		service.sendCodeInput.UserAgent != "test-agent" {
		t.Fatalf("unexpected send code input: %#v", service.sendCodeInput)
	}
	body := decodeAuthBody(t, recorder)
	if body["msg"] != "验证码发送成功" {
		t.Fatalf("unexpected send-code message: %#v", body)
	}
}

func TestHandlerForgetPasswordUsesGoRestContract(t *testing.T) {
	service := &fakeSessionService{}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/forgot-password", strings.NewReader(`{"account":"15671628271","code":"123456","new_password":"new-secret","confirm_password":"new-secret"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.forgetPasswordInput.Account != "15671628271" ||
		service.forgetPasswordInput.Code != "123456" ||
		service.forgetPasswordInput.NewPassword != "new-secret" ||
		service.forgetPasswordInput.ConfirmPassword != "new-secret" {
		t.Fatalf("unexpected forget password input: %#v", service.forgetPasswordInput)
	}
}

func TestHandlerLoginReturnsTokenResult(t *testing.T) {
	service := &fakeSessionService{loginResult: &authmodule.LoginResponse{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/login", strings.NewReader(`{"login_account":"15671628271","login_type":"password","password":"123456"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("platform", "partner_portal")
	request.Header.Set("device-id", "device-1")
	request.Header.Set("User-Agent", "test-agent")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.loginInput.LoginAccount != "15671628271" ||
		service.loginInput.LoginType != "password" ||
		service.loginInput.Password != "123456" ||
		service.loginInput.CaptchaID != "" ||
		service.loginInput.CaptchaAnswer != nil ||
		service.loginInput.Platform != enum.PlatformAdmin ||
		service.loginInput.DeviceID != "device-1" ||
		service.loginInput.UserAgent != "test-agent" {
		t.Fatalf("unexpected login input: %#v", service.loginInput)
	}
	body := decodeAuthBody(t, recorder)
	data := body["data"].(map[string]any)
	if len(data) != 2 || data["access_token"] != "access-token" || data["expires_in"] != float64(14400) {
		t.Fatalf("unexpected token response: %#v", data)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != BrowserRefreshCookieName || cookies[0].Value != "refresh-token" {
		t.Fatalf("login did not issue refresh cookie: %#v", cookies)
	}
}

func TestHandlerCodeLoginDoesNotRequirePasswordCaptchaFields(t *testing.T) {
	service := &fakeSessionService{loginResult: &authmodule.LoginResponse{
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
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("platform", "admin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.loginInput.LoginType != authmodule.LoginTypePhone || service.loginInput.Code != "123456" || service.loginInput.Password != "" || service.loginInput.CaptchaID != "" || service.loginInput.CaptchaAnswer != nil {
		t.Fatalf("unexpected code login input: %#v", service.loginInput)
	}
	_ = decodeAuthBody(t, recorder)
}

func TestHandlerLoginRejectsInvalidEnumInputBeforeService(t *testing.T) {
	service := &fakeSessionService{}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/login", strings.NewReader(`{"login_account":"15671628271","login_type":"wechat","password":"123456"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:5173")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.loginInput.LoginAccount != "" {
		t.Fatalf("service should not be called for invalid login_type: %#v", service.loginInput)
	}
}

func TestHandlerRefreshReturnsTokenResult(t *testing.T) {
	service := &fakeSessionService{refreshResult: &authmodule.TokenResult{
		AccessToken:      "new-access",
		RefreshToken:     "new-refresh",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("User-Agent", "test-agent")
	request.AddCookie(&http.Cookie{Name: BrowserRefreshCookieName, Value: "old-refresh"})
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.refreshInput.RefreshToken != "old-refresh" || service.refreshInput.UserAgent != "test-agent" {
		t.Fatalf("unexpected refresh input: %#v", service.refreshInput)
	}
	body := decodeAuthBody(t, recorder)
	data := body["data"].(map[string]any)
	if len(data) != 2 || data["access_token"] != "new-access" || data["expires_in"] != float64(14400) {
		t.Fatalf("unexpected token response: %#v", data)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != "new-refresh" {
		t.Fatalf("refresh did not rotate cookie: %#v", cookies)
	}
}

func TestHandlerLogoutParsesBearerToken(t *testing.T) {
	service := &fakeSessionService{}
	router := newAuthTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/logout", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Origin", "http://localhost:5173")
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
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != BrowserRefreshCookieName || cookies[0].MaxAge != -1 {
		t.Fatalf("logout did not expire refresh cookie: %#v", cookies)
	}
}

func TestHandlerLogoutRejectsRequestBodyBeforeSessionRevocation(t *testing.T) {
	service := &fakeSessionService{}
	router := newAuthTestRouter(service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/logout", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer access-token")
	request.Header.Set("Origin", "http://localhost:5173")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeAuthBody(t, recorder)
	if body["error"].(map[string]any)["code"] != "request.invalid" || body["msg"] != "退出请求体必须为空" {
		t.Fatalf("unexpected logout body rejection: %#v", body)
	}
	if service.logoutCalls != 0 {
		t.Fatalf("logout service called %d time(s)", service.logoutCalls)
	}
}

func TestBrowserLoginSetsSecureRefreshCookieAndOmitsRefreshCredential(t *testing.T) {
	service := &fakeSessionService{loginResult: &authmodule.LoginResponse{
		AccessToken:      "access-token",
		RefreshToken:     "browser-refresh",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}}
	router := newAuthTestRouterWithOptions(service, WithAllowedOrigins([]string{"http://localhost:5173"}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/login", strings.NewReader(`{"login_account":"admin","login_type":"password","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("platform", "admin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeAuthBody(t, recorder)
	data := body["data"].(map[string]any)
	if len(data) != 2 || data["access_token"] != "access-token" || data["expires_in"] != float64(14400) {
		t.Fatalf("login response is not the closed credential shape: %#v", data)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%#v, want one refresh cookie", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != BrowserRefreshCookieName || cookie.Value != "browser-refresh" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/api/admin/v1/auth" {
		t.Fatalf("unexpected browser refresh cookie: %#v", cookie)
	}
}

func TestLoginDoesNotInterpretObsoleteVariantHeader(t *testing.T) {
	service := &fakeSessionService{loginResult: &authmodule.LoginResponse{
		AccessToken:      "access-token",
		RefreshToken:     "cookie-refresh",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}}
	router := newAuthTestRouterWithOptions(service, WithAllowedOrigins([]string{"http://localhost:5173"}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/login", strings.NewReader(`{"login_account":"admin","login_type":"password","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Admin-Client-Variant", "desktop")
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("platform", "admin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeAuthBody(t, recorder)
	data := body["data"].(map[string]any)
	if len(data) != 2 || data["access_token"] != "access-token" {
		t.Fatalf("obsolete header changed response shape: %#v", data)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != "cookie-refresh" {
		t.Fatalf("obsolete header changed cookie transport: %#v", cookies)
	}
}

func TestBrowserRefreshReadsOnlySecureCookieAndChecksExactOrigin(t *testing.T) {
	service := &fakeSessionService{refreshResult: &authmodule.TokenResult{
		AccessToken:      "new-access",
		RefreshToken:     "new-browser-refresh",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}}
	router := newAuthTestRouterWithOptions(service, WithAllowedOrigins([]string{"http://localhost:5173"}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", nil)
	request.Header.Set("Origin", "http://localhost:5173/")
	request.AddCookie(&http.Cookie{Name: BrowserRefreshCookieName, Value: "cookie-refresh"})
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.refreshInput.RefreshToken != "cookie-refresh" {
		t.Fatalf("refresh input=%#v, want cookie credential", service.refreshInput)
	}
	data := decodeAuthBody(t, recorder)["data"].(map[string]any)
	if _, exposed := data["refresh_token"]; exposed {
		t.Fatalf("browser refresh exposed rotating credential: %#v", data)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != "new-browser-refresh" {
		t.Fatalf("browser refresh did not rotate cookie: %#v", cookies)
	}
}

func TestBrowserRefreshRejectsDisallowedOriginBeforeService(t *testing.T) {
	service := &fakeSessionService{}
	router := newAuthTestRouterWithOptions(service, WithAllowedOrigins([]string{"http://localhost:5173"}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/refresh", nil)
	request.Header.Set("Origin", "http://evil.invalid")
	request.AddCookie(&http.Cookie{Name: BrowserRefreshCookieName, Value: "cookie-refresh"})
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.refreshInput.RefreshToken != "" {
		t.Fatalf("service invoked before origin rejection: %#v", service.refreshInput)
	}
}

func TestAuthLoginRejectsMissingUnlistedAndTauriOriginsBeforeService(t *testing.T) {
	for _, origin := range []string{"", "http://evil.invalid", "tauri://localhost"} {
		t.Run(origin, func(t *testing.T) {
			service := &fakeSessionService{}
			router := newAuthTestRouterWithOptions(service, WithAllowedOrigins([]string{"http://localhost:5173"}))
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/login", strings.NewReader(`{"login_account":"admin","login_type":"password","password":"secret"}`))
			request.Header.Set("Content-Type", "application/json")
			if origin != "" {
				request.Header.Set("Origin", origin)
			}
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			responseBody := decodeAuthBody(t, recorder)
			if responseBody["error"].(map[string]any)["code"] != "auth.origin_forbidden" {
				t.Fatalf("unexpected origin failure: %#v", responseBody)
			}
			if service.loginCalls != 0 {
				t.Fatalf("login service called %d time(s)", service.loginCalls)
			}
		})
	}
}

func TestRealtimeTicketEndpointIssuesBoundOpaqueTicket(t *testing.T) {
	issuer := &fakeBrowserGrantIssuer{}
	router := newAuthGrantTestRouter(issuer)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/realtime-tickets", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if issuer.realtimeSubject != (authmodule.GrantSubject{SessionID: 42, UserID: 7, Platform: "admin"}) {
		t.Fatalf("ticket subject=%#v", issuer.realtimeSubject)
	}
	data := decodeAuthBody(t, recorder)["data"].(map[string]any)
	if data["ticket"] != "realtime-ticket" || data["expires_in"] != float64(30) {
		t.Fatalf("ticket response=%#v", data)
	}
}

func TestQueueMonitorGrantEndpointSetsPathScopedSecureCookie(t *testing.T) {
	issuer := &fakeBrowserGrantIssuer{}
	router := newAuthGrantTestRouter(issuer)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth/queue-monitor-grants", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%#v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != QueueMonitorGrantCookieName || cookie.Value != "queue-grant" || cookie.Path != "/api/admin/v1/queue-monitor-ui" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != 60 {
		t.Fatalf("queue monitor grant cookie=%#v", cookie)
	}
}

func newAuthGrantTestRouter(issuer BrowserGrantIssuer) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{SessionID: 42, UserID: 7, Platform: "admin"})
		c.Next()
	})
	Register(router, &fakeSessionService{}, nil, nil, nil,
		WithAllowedOrigins([]string{"http://localhost:5173"}),
		WithBrowserGrantIssuer(issuer),
	)
	return router
}

func newAuthTestRouter(service authmodule.SessionService) *gin.Engine {
	return newAuthTestRouterWithOptions(service, WithAllowedOrigins([]string{"http://localhost:5173"}))
}

func newAuthTestRouterWithOptions(service authmodule.SessionService, options ...Option) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	Register(router, service, nil, nil, nil, options...)
	return router
}

func newAuthTestRouterWithCaptcha(service authmodule.SessionService, captchaService authmodule.CaptchaHTTPService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	Register(router, service, captchaService, nil, nil)
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

// Session admin transport tests merged from usersession/handler_test.go.
type fakeSessionAdminHTTPService struct {
	pageInitResult *authmodule.SessionPageInitResponse
	listQuery      authmodule.SessionListQuery
	listResult     *authmodule.SessionListResponse
	statsResult    *authmodule.SessionStatsResponse
	revokeID       int64
	batchInput     authmodule.SessionBatchRevokeInput
	currentSession int64
	revokeResult   *authmodule.SessionRevokeResponse
	batchResult    *authmodule.SessionBatchRevokeResponse
	err            *apperror.Error
}

func (f *fakeSessionAdminHTTPService) PageInit(ctx context.Context) (*authmodule.SessionPageInitResponse, *apperror.Error) {
	return f.pageInitResult, f.err
}

func (f *fakeSessionAdminHTTPService) List(ctx context.Context, query authmodule.SessionListQuery) (*authmodule.SessionListResponse, *apperror.Error) {
	f.listQuery = query
	return f.listResult, f.err
}

func (f *fakeSessionAdminHTTPService) Stats(ctx context.Context) (*authmodule.SessionStatsResponse, *apperror.Error) {
	return f.statsResult, f.err
}

func (f *fakeSessionAdminHTTPService) Revoke(ctx context.Context, id int64, currentSessionID int64) (*authmodule.SessionRevokeResponse, *apperror.Error) {
	f.revokeID = id
	f.currentSession = currentSessionID
	if f.revokeResult != nil {
		return f.revokeResult, f.err
	}
	return &authmodule.SessionRevokeResponse{ID: id, Revoked: true}, f.err
}

func (f *fakeSessionAdminHTTPService) BatchRevoke(ctx context.Context, input authmodule.SessionBatchRevokeInput, currentSessionID int64) (*authmodule.SessionBatchRevokeResponse, *apperror.Error) {
	f.batchInput = input
	f.currentSession = currentSessionID
	if f.batchResult != nil {
		return f.batchResult, f.err
	}
	return &authmodule.SessionBatchRevokeResponse{Count: int64(len(input.IDs))}, f.err
}

func TestHandlerRoutesUserSessionReadOnlyEndpointsViaAuthTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeSessionAdminHTTPService{
		pageInitResult: &authmodule.SessionPageInitResponse{},
		listResult:     &authmodule.SessionListResponse{Page: authmodule.SessionPage{PageSize: 30, CurrentPage: 2, Total: 1, TotalPage: 1}},
		statsResult:    &authmodule.SessionStatsResponse{TotalActive: 1, PlatformDistribution: map[string]int64{"admin": 1, "app": 0}},
	}
	router := gin.New()
	Register(router, nil, nil, service, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/v1/user-sessions?current_page=2&page_size=30&username=test&platform=admin&status=active", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", resp.Code, resp.Body.String())
	}
	if service.listQuery.CurrentPage != 2 || service.listQuery.PageSize != 30 || service.listQuery.Username != "test" || service.listQuery.Platform != "admin" || service.listQuery.Status != "active" {
		t.Fatalf("list query mismatch: %#v", service.listQuery)
	}

	for _, path := range []string{"/api/admin/v1/user-sessions/page-init", "/api/admin/v1/user-sessions/stats"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		resp = httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, resp.Code, resp.Body.String())
		}
	}
}

func TestHandlerSessionRevokeUsesCurrentSessionIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeSessionAdminHTTPService{revokeResult: &authmodule.SessionRevokeResponse{ID: 77, Revoked: true}}
	router := newSessionAdminTestRouter(service, &middleware.AuthIdentity{UserID: 1, SessionID: 55, Platform: "admin"})

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/user-sessions/77/revoke", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", resp.Code, resp.Body.String())
	}
	if service.revokeID != 77 || service.currentSession != 55 {
		t.Fatalf("revoke service input mismatch: id=%d current=%d", service.revokeID, service.currentSession)
	}
}

func TestHandlerSessionBatchRevokeUsesCurrentSessionIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeSessionAdminHTTPService{batchResult: &authmodule.SessionBatchRevokeResponse{Count: 2, SkippedCurrent: 1}}
	router := newSessionAdminTestRouter(service, &middleware.AuthIdentity{UserID: 1, SessionID: 55, Platform: "admin"})

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/user-sessions/revoke", bytes.NewBufferString(`{"ids":[77,55,78]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("batch revoke status=%d body=%s", resp.Code, resp.Body.String())
	}
	if service.currentSession != 55 || len(service.batchInput.IDs) != 3 || service.batchInput.IDs[1] != 55 {
		t.Fatalf("batch revoke service input mismatch: current=%d input=%#v", service.currentSession, service.batchInput)
	}
}

func TestHandlerSessionListLocalizesInvalidRequest(t *testing.T) {
	router := newSessionAdminLocalizedTestRouter(&fakeSessionAdminHTTPService{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/v1/user-sessions?current_page=abc", nil)
	req.Header.Set("Accept-Language", "en-US")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("list status=%d body=%s", resp.Code, resp.Body.String())
	}
	body := decodeSessionAdminBody(t, resp)
	if body["msg"] != "Invalid user session list request" {
		t.Fatalf("expected localized list request error, got %#v", body["msg"])
	}
}

func TestHandlerSessionRevokeLocalizesMissingIdentity(t *testing.T) {
	router := newSessionAdminLocalizedTestRouter(&fakeSessionAdminHTTPService{}, nil)

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/user-sessions/77/revoke", nil)
	req.Header.Set("Accept-Language", "en-US")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("revoke status=%d body=%s", resp.Code, resp.Body.String())
	}
	body := decodeSessionAdminBody(t, resp)
	if body["msg"] != "Token is invalid or expired" {
		t.Fatalf("expected localized missing identity error, got %#v", body["msg"])
	}
}

func newSessionAdminTestRouter(service authmodule.SessionAdminHTTPService, identity *middleware.AuthIdentity) *gin.Engine {
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextAuthIdentity, identity)
			c.Next()
		})
	}
	Register(router, nil, nil, service, nil)
	return router
}

func newSessionAdminLocalizedTestRouter(service authmodule.SessionAdminHTTPService, identity *middleware.AuthIdentity) *gin.Engine {
	router := gin.New()
	router.Use(projecti18n.Localize())
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextAuthIdentity, identity)
			c.Next()
		})
	}
	Register(router, nil, nil, service, nil)
	return router
}

func decodeSessionAdminBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}

// Login-log admin transport tests merged from userloginlog/handler_test.go.
type fakeLoginLogHTTPService struct {
	pageInitResult *authmodule.LoginLogPageInitResponse
	listQuery      authmodule.LoginLogListQuery
	listResult     *authmodule.LoginLogListResponse
	err            *apperror.Error
}

func (f *fakeLoginLogHTTPService) PageInit(ctx context.Context) (*authmodule.LoginLogPageInitResponse, *apperror.Error) {
	return f.pageInitResult, f.err
}

func (f *fakeLoginLogHTTPService) List(ctx context.Context, query authmodule.LoginLogListQuery) (*authmodule.LoginLogListResponse, *apperror.Error) {
	f.listQuery = query
	return f.listResult, f.err
}

func TestHandlerRoutesUserLoginLogReadOnlyEndpointsViaAuthTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeLoginLogHTTPService{
		pageInitResult: &authmodule.LoginLogPageInitResponse{},
		listResult: &authmodule.LoginLogListResponse{
			List: []authmodule.LoginLogListItem{{ID: 1}},
			Page: authmodule.LoginLogPage{PageSize: 30, CurrentPage: 2, Total: 1, TotalPage: 1},
		},
	}
	router := gin.New()
	Register(router, nil, nil, nil, service)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/login-logs/page-init", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("page-init status=%d body=%s", resp.Code, resp.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/login-logs?current_page=2&page_size=30&user_id=44&login_account=adm&login_type=password&ip=127&platform=admin&is_success=1&date_start=2026-05-01&date_end=2026-05-08", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", resp.Code, resp.Body.String())
	}
	if service.listQuery.CurrentPage != 2 || service.listQuery.PageSize != 30 || service.listQuery.UserID != 44 {
		t.Fatalf("pagination/user filters mismatch: %#v", service.listQuery)
	}
	if service.listQuery.LoginAccount != "adm" || service.listQuery.LoginType != "password" || service.listQuery.IP != "127" || service.listQuery.Platform != "admin" {
		t.Fatalf("string filters mismatch: %#v", service.listQuery)
	}
	if service.listQuery.IsSuccess == nil || *service.listQuery.IsSuccess != 1 || service.listQuery.DateStart != "2026-05-01" || service.listQuery.DateEnd != "2026-05-08" {
		t.Fatalf("result/date filters mismatch: %#v", service.listQuery)
	}
}

func TestHandlerLoginLogListLocalizesInvalidRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	Register(router, nil, nil, nil, &fakeLoginLogHTTPService{})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/login-logs?is_success=abc", nil)
	req.Header.Set("Accept-Language", "en-US")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("list status=%d body=%s", resp.Code, resp.Body.String())
	}
	body := decodeAuthBody(t, resp)
	if body["msg"] != "Invalid user login log list request" {
		t.Fatalf("expected localized msg, got %#v", body["msg"])
	}
}
