package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/captcha"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/module/user"

	"github.com/gin-gonic/gin"
)

type fakePlatformSessionService struct {
	loginInput     auth.LoginInput
	configPlatform string
	sendCodeInput  auth.SendCodeInput
	logoutToken    string
}

func (f *fakePlatformSessionService) Login(ctx context.Context, input auth.LoginInput) (*auth.LoginResponse, *apperror.Error) {
	f.loginInput = input
	return &auth.LoginResponse{AccessToken: "app-token", UserID: 7}, nil
}
func (f *fakePlatformSessionService) SendCode(ctx context.Context, input auth.SendCodeInput) (string, *apperror.Error) {
	f.sendCodeInput = input
	return "", nil
}
func (f *fakePlatformSessionService) ForgetPassword(ctx context.Context, input auth.ForgetPasswordInput) *apperror.Error {
	return nil
}
func (f *fakePlatformSessionService) LoginConfig(ctx context.Context, platform string) (*auth.LoginConfigResponse, *apperror.Error) {
	f.configPlatform = platform
	return &auth.LoginConfigResponse{CaptchaEnabled: true, CaptchaType: captcha.TypeSlide}, nil
}
func (f *fakePlatformSessionService) Refresh(ctx context.Context, input session.RefreshInput) (*session.TokenResult, *apperror.Error) {
	return &session.TokenResult{}, nil
}
func (f *fakePlatformSessionService) Logout(ctx context.Context, accessToken string) *apperror.Error {
	f.logoutToken = accessToken
	return nil
}

type fakePlatformCaptchaService struct{}

func (fakePlatformCaptchaService) Generate(ctx context.Context) (*captcha.ChallengeResponse, *apperror.Error) {
	return &captcha.ChallengeResponse{CaptchaID: "captcha-id", CaptchaType: captcha.TypeSlide, MasterImage: "master", TileImage: "tile", ExpiresIn: 120}, nil
}

type fakePlatformUserService struct {
	input user.InitInput
}

func (f *fakePlatformUserService) Init(ctx context.Context, input user.InitInput) (*user.InitResponse, *apperror.Error) {
	f.input = input
	return &user.InitResponse{
		UserID: input.UserID, Username: "App User", Avatar: "avatar.png", RoleName: "app",
		Permissions: []permission.MenuItem{}, Router: []permission.RouteItem{}, ButtonCodes: []string{},
	}, nil
}

func TestPlatformAuthRoutesForceConfiguredPlatform(t *testing.T) {
	authService := &fakePlatformSessionService{}
	userService := &fakePlatformUserService{}
	router := newPlatformAuthTestRouter(authService, userService)

	configRecorder := httptest.NewRecorder()
	configRequest := httptest.NewRequest(http.MethodGet, "/api/app/v1/auth/login-config", nil)
	configRequest.Header.Set("platform", enum.PlatformAdmin)
	router.ServeHTTP(configRecorder, configRequest)
	if configRecorder.Code != http.StatusOK {
		t.Fatalf("expected login-config status 200, got %d body=%s", configRecorder.Code, configRecorder.Body.String())
	}
	if authService.configPlatform != enum.PlatformApp {
		t.Fatalf("expected app platform, got %q", authService.configPlatform)
	}

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/app/v1/auth/login", strings.NewReader(`{"login_type":"password","login_account":"15671628271","password":"123456","captcha_id":"captcha-id","captcha_answer":{"x":120,"y":80}}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRequest.Header.Set("platform", enum.PlatformAdmin)
	loginRequest.Header.Set("device-id", "ios-1")
	loginRequest.Header.Set("User-Agent", "agent")
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("expected login status 200, got %d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
	if authService.loginInput.Platform != enum.PlatformApp || authService.loginInput.DeviceID != "ios-1" {
		t.Fatalf("unexpected login input: %#v", authService.loginInput)
	}
	if userService.input.UserID != 7 || userService.input.Platform != enum.PlatformApp {
		t.Fatalf("unexpected init input: %#v", userService.input)
	}
	data := decodePlatformAuthData(t, loginRecorder)
	if data["token"] != "app-token" {
		t.Fatalf("expected app token response, got %#v", data)
	}
	if _, ok := data["access_token"]; ok {
		t.Fatalf("app login response must not expose admin token field: %#v", data)
	}
	userData := data["user"].(map[string]any)
	if userData["nickname"] != "App User" || userData["avatar"] != "avatar.png" {
		t.Fatalf("unexpected app user payload: %#v", userData)
	}
}

func TestPlatformAuthRoutesExposeCaptchaSendCodeAndLogout(t *testing.T) {
	authService := &fakePlatformSessionService{}
	router := newPlatformAuthTestRouter(authService, &fakePlatformUserService{})

	captchaRecorder := httptest.NewRecorder()
	router.ServeHTTP(captchaRecorder, httptest.NewRequest(http.MethodGet, "/api/app/v1/auth/captcha", nil))
	if captchaRecorder.Code != http.StatusOK {
		t.Fatalf("expected captcha status 200, got %d body=%s", captchaRecorder.Code, captchaRecorder.Body.String())
	}

	sendCodeRecorder := httptest.NewRecorder()
	sendCodeRequest := httptest.NewRequest(http.MethodPost, "/api/app/v1/auth/send-code", strings.NewReader(`{"account":"15671628271","scene":"login"}`))
	sendCodeRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(sendCodeRecorder, sendCodeRequest)
	if sendCodeRecorder.Code != http.StatusOK {
		t.Fatalf("expected send-code status 200, got %d body=%s", sendCodeRecorder.Code, sendCodeRecorder.Body.String())
	}
	if authService.sendCodeInput.Account != "15671628271" || authService.sendCodeInput.Scene != auth.VerifyCodeSceneLogin {
		t.Fatalf("unexpected send-code input: %#v", authService.sendCodeInput)
	}

	logoutRecorder := httptest.NewRecorder()
	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/app/v1/auth/logout", nil)
	logoutRequest.Header.Set("Authorization", "Bearer app-token")
	router.ServeHTTP(logoutRecorder, logoutRequest)
	if logoutRecorder.Code != http.StatusOK {
		t.Fatalf("expected logout status 200, got %d body=%s", logoutRecorder.Code, logoutRecorder.Body.String())
	}
	if authService.logoutToken != "app-token" {
		t.Fatalf("expected logout token app-token, got %q", authService.logoutToken)
	}
}

func newPlatformAuthTestRouter(authService auth.SessionService, userService auth.PlatformUserInitService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	auth.RegisterPlatformRoutes(router, auth.PlatformRouteOptions{
		Prefix:         "/api/app/v1/auth",
		Platform:       enum.PlatformApp,
		AuthService:    authService,
		CaptchaService: fakePlatformCaptchaService{},
		UserService:    userService,
	})
	return router
}

func decodePlatformAuthData(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body["data"].(map[string]any)
}
