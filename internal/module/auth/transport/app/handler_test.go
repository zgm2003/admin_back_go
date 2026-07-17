package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authmodule "admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

type fakeSessionService struct {
	loginInput     authmodule.LoginInput
	configPlatform string
	sendCodeInput  authmodule.SendCodeInput
	logoutToken    string
}

func (f *fakeSessionService) Login(ctx context.Context, input authmodule.LoginInput) (*authmodule.LoginResponse, *apperror.Error) {
	f.loginInput = input
	return &authmodule.LoginResponse{AccessToken: "app-token", UserID: 7}, nil
}
func (f *fakeSessionService) SendCode(ctx context.Context, input authmodule.SendCodeInput) (string, *apperror.Error) {
	f.sendCodeInput = input
	return "", nil
}
func (f *fakeSessionService) ForgetPassword(ctx context.Context, input authmodule.ForgetPasswordInput) *apperror.Error {
	return nil
}
func (f *fakeSessionService) LoginConfig(ctx context.Context, platform string) (*authmodule.LoginConfigResponse, *apperror.Error) {
	f.configPlatform = platform
	return &authmodule.LoginConfigResponse{CaptchaEnabled: true, CaptchaType: authmodule.TypeSlide, AllowRegister: false}, nil
}
func (f *fakeSessionService) Refresh(ctx context.Context, input authmodule.RefreshInput) (*authmodule.TokenResult, *apperror.Error) {
	return &authmodule.TokenResult{}, nil
}
func (f *fakeSessionService) Logout(ctx context.Context, accessToken string) *apperror.Error {
	f.logoutToken = accessToken
	return nil
}

type fakeCaptchaService struct{}

func (fakeCaptchaService) Generate(ctx context.Context) (*authmodule.ChallengeResponse, *apperror.Error) {
	return &authmodule.ChallengeResponse{CaptchaID: "captcha-id", CaptchaType: authmodule.TypeSlide, MasterImage: "master", TileImage: "tile", ExpiresIn: 120}, nil
}

type fakeUserService struct {
	input     user.InitInput
	returnNil bool
}

func (f *fakeUserService) Init(ctx context.Context, input user.InitInput) (*user.InitResponse, *apperror.Error) {
	f.input = input
	if f.returnNil {
		return nil, nil
	}
	return &user.InitResponse{
		UserID: input.UserID, Username: "App User", Avatar: "avatar.png", RoleName: "app",
		Permissions: []permission.MenuItem{{Index: "app_home", Label: "App Home", Path: "/app/home"}},
		Router:      []permission.RouteItem{{Path: "/app/home"}},
		ButtonCodes: []string{"app_access"},
	}, nil
}

func TestAuthRoutesForceConfiguredPlatform(t *testing.T) {
	authService := &fakeSessionService{}
	userService := &fakeUserService{}
	router := newAuthTestRouter(authService, userService)

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
	configData := decodeAuthData(t, configRecorder)
	if configData["allow_register"] != false {
		t.Fatalf("expected login-config to expose allow_register=false, got %#v", configData)
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
	data := decodeAuthData(t, loginRecorder)
	if data["token"] != "app-token" {
		t.Fatalf("expected app token response, got %#v", data)
	}
	if _, ok := data["access_token"]; ok {
		t.Fatalf("app login response must not expose admin token field: %#v", data)
	}
	userData := data["user"].(map[string]any)
	if userData["user_id"] != float64(7) || userData["username"] != "App User" || userData["avatar"] != "avatar.png" || userData["role_name"] != "app" {
		t.Fatalf("unexpected app user payload: %#v", userData)
	}
	if _, ok := userData["permissions"].([]any); !ok {
		t.Fatalf("expected permissions in app login user payload, got %#v", userData["permissions"])
	}
	if !routeSliceEqual(userData["router"], []string{"/app/home"}) {
		t.Fatalf("expected router in app login user payload, got %#v", userData["router"])
	}
	if !stringSliceEqual(userData["buttonCodes"], []string{"app_access"}) {
		t.Fatalf("expected app button codes in login payload, got %#v", userData["buttonCodes"])
	}
	for _, forbidden := range []string{"id", "nickname", "quick_entry", "quickEntry", "permissionCodes", "permission_codes", "button_codes"} {
		if _, ok := userData[forbidden]; ok {
			t.Fatalf("app user payload must not expose fallback/alias field %q: %#v", forbidden, userData)
		}
	}
}

func TestAuthLoginFailsWhenCurrentUserMissing(t *testing.T) {
	router := newAuthTestRouter(&fakeSessionService{}, &fakeUserService{returnNil: true})

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/app/v1/auth/login", strings.NewReader(`{"login_type":"password","login_account":"15671628271","password":"123456","captcha_id":"captcha-id","captcha_answer":{"x":120,"y":80}}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(loginRecorder, loginRequest)

	if loginRecorder.Code != http.StatusInternalServerError {
		t.Fatalf("missing current user must be an internal error, got %d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
}

func TestAuthRoutesExposeCaptchaSendCodeAndLogout(t *testing.T) {
	authService := &fakeSessionService{}
	router := newAuthTestRouter(authService, &fakeUserService{})

	captchaRecorder := httptest.NewRecorder()
	router.ServeHTTP(captchaRecorder, httptest.NewRequest(http.MethodGet, "/api/app/v1/auth/captcha", nil))
	if captchaRecorder.Code != http.StatusOK {
		t.Fatalf("expected captcha status 200, got %d body=%s", captchaRecorder.Code, captchaRecorder.Body.String())
	}

	sendCodeRecorder := httptest.NewRecorder()
	sendCodeRequest := httptest.NewRequest(http.MethodPost, "/api/app/v1/auth/send-code", strings.NewReader(`{"account":"15671628271","scene":"login","login_type":"phone","captcha_id":"captcha-id","captcha_answer":{"x":120,"y":80}}`))
	sendCodeRequest.Header.Set("Content-Type", "application/json")
	sendCodeRequest.Header.Set("User-Agent", "app-agent")
	router.ServeHTTP(sendCodeRecorder, sendCodeRequest)
	if sendCodeRecorder.Code != http.StatusOK {
		t.Fatalf("expected send-code status 200, got %d body=%s", sendCodeRecorder.Code, sendCodeRecorder.Body.String())
	}
	if authService.sendCodeInput.Account != "15671628271" ||
		authService.sendCodeInput.Scene != authmodule.VerifyCodeSceneLogin ||
		authService.sendCodeInput.LoginType != authmodule.LoginTypePhone ||
		authService.sendCodeInput.CaptchaID != "captcha-id" ||
		authService.sendCodeInput.CaptchaAnswer == nil ||
		authService.sendCodeInput.CaptchaAnswer.X != 120 ||
		authService.sendCodeInput.CaptchaAnswer.Y != 80 ||
		authService.sendCodeInput.ClientIP == "" ||
		authService.sendCodeInput.UserAgent != "app-agent" {
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

func newAuthTestRouter(authService authmodule.SessionService, userService UserInitService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	Register(router, Dependencies{
		AuthService:    authService,
		CaptchaService: fakeCaptchaService{},
		UserService:    userService,
	})
	return router
}

func decodeAuthData(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body["data"].(map[string]any)
}

func stringSliceEqual(value any, want []string) bool {
	got, ok := value.([]any)
	if !ok || len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func routeSliceEqual(value any, wantPaths []string) bool {
	got, ok := value.([]any)
	if !ok || len(got) != len(wantPaths) {
		return false
	}
	for i := range wantPaths {
		item, ok := got[i].(map[string]any)
		if !ok || item["path"] != wantPaths[i] {
			return false
		}
	}
	return true
}
