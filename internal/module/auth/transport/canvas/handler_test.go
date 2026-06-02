package canvas

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
	refreshInput   authmodule.RefreshInput
	configPlatform string
	sendCodeInput  authmodule.SendCodeInput
	logoutToken    string
}

func (f *fakeSessionService) Login(ctx context.Context, input authmodule.LoginInput) (*authmodule.LoginResponse, *apperror.Error) {
	f.loginInput = input
	return &authmodule.LoginResponse{AccessToken: "canvas-token", RefreshToken: "canvas-refresh", ExpiresIn: 14400, RefreshExpiresIn: 1209600, UserID: 9}, nil
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
	return &authmodule.LoginConfigResponse{CaptchaEnabled: true, CaptchaType: authmodule.TypeSlide, AllowRegister: true}, nil
}
func (f *fakeSessionService) Refresh(ctx context.Context, input authmodule.RefreshInput) (*authmodule.TokenResult, *apperror.Error) {
	f.refreshInput = input
	return &authmodule.TokenResult{AccessToken: "next-canvas-token", RefreshToken: "next-canvas-refresh", ExpiresIn: 14400, RefreshExpiresIn: 1209600}, nil
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
		UserID: input.UserID, Username: "Canvas User", Avatar: "canvas.png", RoleName: "canvas",
		Permissions: []permission.MenuItem{},
		Router: []permission.RouteItem{
			{Path: "/canvas", Meta: map[string]string{"code": "canvas_page"}},
			{Path: "/prompts", Meta: map[string]string{"code": "canvas_prompts_page"}},
		},
		ButtonCodes: []string{"canvas_access", "canvas_prompt_read"},
	}, nil
}

func TestCanvasAuthRoutesForceCanvasPlatform(t *testing.T) {
	authService := &fakeSessionService{}
	userService := &fakeUserService{}
	router := newCanvasAuthTestRouter(authService, userService)

	configRecorder := httptest.NewRecorder()
	configRequest := httptest.NewRequest(http.MethodGet, "/api/canvas/v1/auth/login-config", nil)
	configRequest.Header.Set("platform", enum.PlatformAdmin)
	router.ServeHTTP(configRecorder, configRequest)
	if configRecorder.Code != http.StatusOK {
		t.Fatalf("expected login-config status 200, got %d body=%s", configRecorder.Code, configRecorder.Body.String())
	}
	if authService.configPlatform != enum.PlatformCanvas {
		t.Fatalf("expected canvas platform, got %q", authService.configPlatform)
	}
	configData := decodeAuthData(t, configRecorder)
	if configData["allow_register"] != true {
		t.Fatalf("expected login-config to expose allow_register=true, got %#v", configData)
	}

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/auth/login", strings.NewReader(`{"login_type":"password","login_account":"15671628271","password":"123456","captcha_id":"captcha-id","captcha_answer":{"x":120,"y":80}}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRequest.Header.Set("platform", enum.PlatformAdmin)
	loginRequest.Header.Set("device-id", "web-1")
	loginRequest.Header.Set("User-Agent", "agent")
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("expected login status 200, got %d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
	if authService.loginInput.Platform != enum.PlatformCanvas || authService.loginInput.DeviceID != "web-1" {
		t.Fatalf("unexpected login input: %#v", authService.loginInput)
	}
	if userService.input.UserID != 9 || userService.input.Platform != enum.PlatformCanvas {
		t.Fatalf("unexpected init input: %#v", userService.input)
	}
	data := decodeAuthData(t, loginRecorder)
	if data["token"] != "canvas-token" {
		t.Fatalf("expected canvas token response, got %#v", data)
	}
	if data["refresh_token"] != "canvas-refresh" || data["expires_in"] != float64(14400) || data["refresh_expires_in"] != float64(1209600) {
		t.Fatalf("expected canvas refresh token metadata, got %#v", data)
	}
	if _, ok := data["access_token"]; ok {
		t.Fatalf("canvas login response must not expose admin token field: %#v", data)
	}
	userData := data["user"].(map[string]any)
	if userData["user_id"] != float64(9) || userData["username"] != "Canvas User" || userData["avatar"] != "canvas.png" || userData["role_name"] != "canvas" {
		t.Fatalf("unexpected canvas user payload: %#v", userData)
	}
	for _, forbidden := range []string{"id", "nickname", "quick_entry", "quickEntry", "display_name", "avatar_url", "permissionCodes", "permission_codes", "button_codes"} {
		if _, ok := userData[forbidden]; ok {
			t.Fatalf("canvas user payload must not expose fallback/alias field %q: %#v", forbidden, userData)
		}
	}
	if _, ok := userData["permissions"].([]any); !ok {
		t.Fatalf("expected permissions in canvas login user payload, got %#v", userData["permissions"])
	}
	if !routeSliceEqual(userData["router"], []string{"/canvas", "/prompts"}) {
		t.Fatalf("expected router in canvas login user payload, got %#v", userData["router"])
	}
	if !stringSliceEqual(userData["buttonCodes"], []string{"canvas_access", "canvas_prompt_read"}) {
		t.Fatalf("expected canvas button codes in login payload, got %#v", userData["buttonCodes"])
	}
}

func TestCanvasAuthLoginFailsWhenCurrentUserMissing(t *testing.T) {
	router := newCanvasAuthTestRouter(&fakeSessionService{}, &fakeUserService{returnNil: true})

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/auth/login", strings.NewReader(`{"login_type":"password","login_account":"15671628271","password":"123456","captcha_id":"captcha-id","captcha_answer":{"x":120,"y":80}}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(loginRecorder, loginRequest)

	if loginRecorder.Code != http.StatusInternalServerError {
		t.Fatalf("missing current user must be an internal error, got %d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
}

func TestCanvasAuthRoutesExposeCaptchaSendCodeAndLogout(t *testing.T) {
	authService := &fakeSessionService{}
	router := newCanvasAuthTestRouter(authService, &fakeUserService{})

	captchaRecorder := httptest.NewRecorder()
	router.ServeHTTP(captchaRecorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/auth/captcha", nil))
	if captchaRecorder.Code != http.StatusOK {
		t.Fatalf("expected captcha status 200, got %d body=%s", captchaRecorder.Code, captchaRecorder.Body.String())
	}

	sendCodeRecorder := httptest.NewRecorder()
	sendCodeRequest := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/auth/send-code", strings.NewReader(`{"account":"15671628271","scene":"login"}`))
	sendCodeRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(sendCodeRecorder, sendCodeRequest)
	if sendCodeRecorder.Code != http.StatusOK {
		t.Fatalf("expected send-code status 200, got %d body=%s", sendCodeRecorder.Code, sendCodeRecorder.Body.String())
	}
	if authService.sendCodeInput.Account != "15671628271" || authService.sendCodeInput.Scene != authmodule.VerifyCodeSceneLogin {
		t.Fatalf("unexpected send-code input: %#v", authService.sendCodeInput)
	}

	refreshRecorder := httptest.NewRecorder()
	refreshRequest := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/auth/refresh", strings.NewReader(`{"refresh_token":"old-canvas-refresh"}`))
	refreshRequest.Header.Set("Content-Type", "application/json")
	refreshRequest.Header.Set("User-Agent", "canvas-agent")
	router.ServeHTTP(refreshRecorder, refreshRequest)
	if refreshRecorder.Code != http.StatusOK {
		t.Fatalf("expected refresh status 200, got %d body=%s", refreshRecorder.Code, refreshRecorder.Body.String())
	}
	if authService.refreshInput.RefreshToken != "old-canvas-refresh" || authService.refreshInput.UserAgent != "canvas-agent" {
		t.Fatalf("unexpected refresh input: %#v", authService.refreshInput)
	}
	refreshData := decodeAuthData(t, refreshRecorder)
	if refreshData["token"] != "next-canvas-token" || refreshData["refresh_token"] != "next-canvas-refresh" {
		t.Fatalf("expected canvas refresh response, got %#v", refreshData)
	}
	if _, ok := refreshData["access_token"]; ok {
		t.Fatalf("canvas refresh response must not expose admin token field: %#v", refreshData)
	}

	logoutRecorder := httptest.NewRecorder()
	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/auth/logout", nil)
	logoutRequest.Header.Set("Authorization", "Bearer canvas-token")
	router.ServeHTTP(logoutRecorder, logoutRequest)
	if logoutRecorder.Code != http.StatusOK {
		t.Fatalf("expected logout status 200, got %d body=%s", logoutRecorder.Code, logoutRecorder.Body.String())
	}
	if authService.logoutToken != "canvas-token" {
		t.Fatalf("expected logout token canvas-token, got %q", authService.logoutToken)
	}
}

func newCanvasAuthTestRouter(authService authmodule.SessionService, userService UserInitService) *gin.Engine {
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
