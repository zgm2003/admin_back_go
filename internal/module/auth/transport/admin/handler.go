package admin

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/middleware"
	authmodule "admin_back_go/internal/module/auth"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service             authmodule.SessionService
	captchaService      authmodule.CaptchaHTTPService
	sessionAdminService authmodule.SessionAdminHTTPService
	loginLogService     authmodule.LoginLogHTTPService
	allowedOrigins      map[string]struct{}
	now                 func() time.Time
	browserGrants       BrowserGrantIssuer
	routeRegistry       *adminroute.Registry
}

type BrowserGrantIssuer interface {
	IssueRealtimeTicket(context.Context, authmodule.GrantSubject) (*authmodule.BrowserGrant, *apperror.Error)
	IssueQueueMonitorGrant(context.Context, authmodule.GrantSubject) (*authmodule.BrowserGrant, *apperror.Error)
}

type Option func(*Handler)

func WithAllowedOrigins(origins []string) Option {
	return func(handler *Handler) {
		for _, origin := range origins {
			if normalized, ok := normalizeOrigin(origin); ok {
				handler.allowedOrigins[normalized] = struct{}{}
			}
		}
	}
}

func WithNow(now func() time.Time) Option {
	return func(handler *Handler) {
		if now != nil {
			handler.now = now
		}
	}
}

func WithBrowserGrantIssuer(issuer BrowserGrantIssuer) Option {
	return func(handler *Handler) {
		handler.browserGrants = issuer
	}
}

func WithRouteRegistry(registry *adminroute.Registry) Option {
	return func(handler *Handler) {
		handler.routeRegistry = registry
	}
}

func NewHandler(service authmodule.SessionService, captchaService authmodule.CaptchaHTTPService, sessionAdminService authmodule.SessionAdminHTTPService, loginLogService authmodule.LoginLogHTTPService, options ...Option) *Handler {
	handler := &Handler{
		service:             service,
		captchaService:      captchaService,
		sessionAdminService: sessionAdminService,
		loginLogService:     loginLogService,
		allowedOrigins:      make(map[string]struct{}),
		now:                 time.Now,
	}
	for _, option := range options {
		if option != nil {
			option(handler)
		}
	}
	return handler
}

func (h *Handler) LoginConfig(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Unauthorized("登录服务未配置"))
		return
	}

	result, appErr := h.service.LoginConfig(c.Request.Context(), c.GetHeader("platform"))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Captcha(c *gin.Context) {
	if h.captchaService == nil {
		response.Error(c, apperror.InternalKey("captcha.service_missing", nil, "验证码服务未配置"))
		return
	}
	result, appErr := h.captchaService.Generate(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Login(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Unauthorized("登录服务未配置"))
		return
	}
	variant, ok := h.requireClientVariant(c)
	if !ok {
		return
	}
	if variant == authmodule.ClientBrowser && !h.requireAllowedOrigin(c) {
		return
	}
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("登录参数错误"))
		return
	}
	result, appErr := h.service.Login(c.Request.Context(), authmodule.LoginInput{
		LoginAccount: req.LoginAccount,
		LoginType:    req.LoginType,
		Password:     req.Password,
		Code:         req.Code,
		Platform:     c.GetHeader("platform"),
		DeviceID:     c.GetHeader("device-id"),
		ClientIP:     c.ClientIP(),
		UserAgent:    c.GetHeader("User-Agent"),
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if variant == authmodule.ClientBrowser {
		h.setRefreshCookie(c, result.RefreshToken, result.RefreshExpiresIn)
	}
	response.OK(c, presentLogin(result, variant))
}

func (h *Handler) SendCode(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Unauthorized("登录服务未配置"))
		return
	}
	var req SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("验证码参数错误"))
		return
	}
	_, appErr := h.service.SendCode(c.Request.Context(), authmodule.SendCodeInput{
		Account:       req.Account,
		Scene:         req.Scene,
		LoginType:     req.LoginType,
		CaptchaID:     req.CaptchaID,
		CaptchaAnswer: captchaAnswerFromRequest(req.CaptchaAnswer),
		ClientIP:      c.ClientIP(),
		UserAgent:     c.GetHeader("User-Agent"),
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{}, "auth.verify_code.sent", nil, "验证码发送成功")
}

func (h *Handler) ForgetPassword(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Unauthorized("登录服务未配置"))
		return
	}
	var req ForgetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("重置密码参数错误"))
		return
	}
	if appErr := h.service.ForgetPassword(c.Request.Context(), authmodule.ForgetPasswordInput(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Refresh(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Unauthorized("Token认证未配置"))
		return
	}

	variant, ok := h.requireClientVariant(c)
	if !ok {
		return
	}
	refreshToken := ""
	if variant == authmodule.ClientBrowser {
		if !h.requireAllowedOrigin(c) {
			return
		}
		cookie, err := c.Request.Cookie(BrowserRefreshCookieName)
		if err == nil {
			refreshToken = strings.TrimSpace(cookie.Value)
		}
	} else {
		var req RefreshRequest
		if err := c.ShouldBindJSON(&req); err == nil {
			refreshToken = strings.TrimSpace(req.RefreshToken)
		}
	}
	if refreshToken == "" {
		response.Error(c, apperror.Unauthorized("缺少刷新令牌"))
		return
	}

	result, appErr := h.service.Refresh(c.Request.Context(), authmodule.RefreshInput{
		RefreshToken: refreshToken,
		ClientIP:     c.ClientIP(),
		UserAgent:    c.GetHeader("User-Agent"),
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if variant == authmodule.ClientBrowser {
		h.setRefreshCookie(c, result.RefreshToken, result.RefreshExpiresIn)
	}
	response.OK(c, presentCredentials(result, variant))
}

func (h *Handler) Logout(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Unauthorized("Token认证未配置"))
		return
	}

	variant, ok := h.requireClientVariant(c)
	if !ok {
		return
	}
	if variant == authmodule.ClientBrowser && !h.requireAllowedOrigin(c) {
		return
	}
	accessToken, tokenErr := middleware.ParseBearerToken(c.GetHeader("Authorization"))
	if tokenErr != nil {
		response.Error(c, tokenErr)
		return
	}
	if appErr := h.service.Logout(c.Request.Context(), accessToken); appErr != nil {
		response.Error(c, appErr)
		return
	}
	if variant == authmodule.ClientBrowser {
		h.clearRefreshCookie(c)
	}
	response.OKWithMessageKey(c, gin.H{}, "auth.logout.success", nil, "退出成功")
}

func (h *Handler) RealtimeTicket(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || h.browserGrants == nil {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return
	}
	grant, appErr := h.browserGrants.IssueRealtimeTicket(c.Request.Context(), authmodule.GrantSubject{
		SessionID: identity.SessionID,
		UserID:    identity.UserID,
		Platform:  identity.Platform,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, grant)
}

func (h *Handler) QueueMonitorGrant(c *gin.Context) {
	variant, ok := h.requireClientVariant(c)
	if !ok {
		return
	}
	if variant != authmodule.ClientBrowser {
		response.Error(c, apperror.BadRequestKey("auth.browser_variant_required", nil, "该操作仅支持浏览器客户端"))
		return
	}
	if !h.requireAllowedOrigin(c) {
		return
	}
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || h.browserGrants == nil {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return
	}
	grant, appErr := h.browserGrants.IssueQueueMonitorGrant(c.Request.Context(), authmodule.GrantSubject{
		SessionID: identity.SessionID,
		UserID:    identity.UserID,
		Platform:  identity.Platform,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	h.setQueueMonitorGrantCookie(c, grant.Credential, int(grant.ExpiresIn))
	response.OK(c, gin.H{"expires_in": grant.ExpiresIn})
}

const BrowserRefreshCookieName = "__Secure-admin_refresh"

const QueueMonitorGrantCookieName = "__Secure-admin_queue_monitor"

func (h *Handler) requireClientVariant(c *gin.Context) (authmodule.ClientVariant, bool) {
	variant, ok := authmodule.ParseClientVariant(c.GetHeader(authmodule.ClientVariantHeader))
	if !ok {
		response.Error(c, apperror.New(
			"auth.client_variant_invalid",
			apperror.CategoryValidation,
			http.StatusBadRequest,
			apperror.Permanent,
			"auth.client_variant_invalid",
			nil,
			"缺少或无法识别客户端类型",
		))
		return "", false
	}
	return variant, true
}

func (h *Handler) requireAllowedOrigin(c *gin.Context) bool {
	origin, ok := normalizeOrigin(c.GetHeader("Origin"))
	if ok {
		_, ok = h.allowedOrigins[origin]
	}
	if !ok {
		response.Error(c, apperror.New(
			"auth.origin_forbidden",
			apperror.CategoryAuthorization,
			http.StatusForbidden,
			apperror.Permanent,
			"auth.origin_forbidden",
			nil,
			"请求来源不受信任",
		))
		return false
	}
	return true
}

func normalizeOrigin(value string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.User != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), true
}

func (h *Handler) setRefreshCookie(c *gin.Context, credential string, expiresIn int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     BrowserRefreshCookieName,
		Value:    credential,
		Path:     "/api/admin/v1/auth",
		Expires:  h.now().Add(time.Duration(expiresIn) * time.Second),
		MaxAge:   expiresIn,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *Handler) clearRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     BrowserRefreshCookieName,
		Path:     "/api/admin/v1/auth",
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *Handler) setQueueMonitorGrantCookie(c *gin.Context, credential string, expiresIn int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     QueueMonitorGrantCookieName,
		Value:    credential,
		Path:     "/api/admin/v1/queue-monitor-ui",
		Expires:  h.now().Add(time.Duration(expiresIn) * time.Second),
		MaxAge:   expiresIn,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *Handler) SessionPageInit(c *gin.Context) {
	result, appErr := h.requireSessionAdminService().PageInit(c.Request.Context())
	writeSessionAdminResult(c, result, appErr)
}

func (h *Handler) SessionList(c *gin.Context) {
	var req sessionListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("usersession.list.request.invalid", nil, "用户会话列表参数错误"))
		return
	}
	result, appErr := h.requireSessionAdminService().List(c.Request.Context(), authmodule.SessionListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		Username:    req.Username,
		Platform:    req.Platform,
		Status:      req.Status,
	})
	writeSessionAdminResult(c, result, appErr)
}

func (h *Handler) SessionStats(c *gin.Context) {
	result, appErr := h.requireSessionAdminService().Stats(c.Request.Context())
	writeSessionAdminResult(c, result, appErr)
}

func (h *Handler) SessionRevoke(c *gin.Context) {
	id, ok := sessionRouteID(c)
	if !ok {
		return
	}
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.SessionID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return
	}
	result, appErr := h.requireSessionAdminService().Revoke(c.Request.Context(), id, identity.SessionID)
	writeSessionAdminResult(c, result, appErr)
}

func (h *Handler) SessionBatchRevoke(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.SessionID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return
	}
	var req sessionBatchRevokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("usersession.batch_revoke.request.invalid", nil, "批量踢下线参数错误"))
		return
	}
	result, appErr := h.requireSessionAdminService().BatchRevoke(c.Request.Context(), authmodule.SessionBatchRevokeInput{IDs: req.IDs}, identity.SessionID)
	writeSessionAdminResult(c, result, appErr)
}

func (h *Handler) LoginLogPageInit(c *gin.Context) {
	result, appErr := h.requireLoginLogService().PageInit(c.Request.Context())
	writeLoginLogResult(c, result, appErr)
}

func (h *Handler) LoginLogList(c *gin.Context) {
	var req loginLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("userloginlog.request.invalid", nil, "用户登录日志列表参数错误"))
		return
	}
	result, appErr := h.requireLoginLogService().List(c.Request.Context(), authmodule.LoginLogListQuery{
		CurrentPage:  req.CurrentPage,
		PageSize:     req.PageSize,
		UserID:       req.UserID,
		LoginAccount: req.LoginAccount,
		LoginType:    req.LoginType,
		IP:           req.IP,
		Platform:     req.Platform,
		IsSuccess:    req.IsSuccess,
		DateStart:    req.DateStart,
		DateEnd:      req.DateEnd,
	})
	writeLoginLogResult(c, result, appErr)
}

func (h *Handler) requireLoginLogService() authmodule.LoginLogHTTPService {
	if h == nil || h.loginLogService == nil {
		return nilLoginLogService{}
	}
	return h.loginLogService
}

func writeLoginLogResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func captchaAnswerFromRequest(req *captchaAnswerRequest) *authmodule.Answer {
	if req == nil {
		return nil
	}
	return &authmodule.Answer{X: req.X, Y: req.Y}
}

func (h *Handler) requireSessionAdminService() authmodule.SessionAdminHTTPService {
	if h == nil || h.sessionAdminService == nil {
		return nilSessionAdminService{}
	}
	return h.sessionAdminService
}

func writeSessionAdminResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func sessionRouteID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequestKey("usersession.id.invalid", nil, "无效的用户会话ID"))
		return 0, false
	}
	return id, true
}

type nilSessionAdminService struct{}

func (nilSessionAdminService) PageInit(ctx context.Context) (*authmodule.SessionPageInitResponse, *apperror.Error) {
	return nil, apperror.InternalKey("usersession.service_missing", nil, "用户会话服务未配置")
}

func (nilSessionAdminService) List(ctx context.Context, query authmodule.SessionListQuery) (*authmodule.SessionListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("usersession.service_missing", nil, "用户会话服务未配置")
}

func (nilSessionAdminService) Stats(ctx context.Context) (*authmodule.SessionStatsResponse, *apperror.Error) {
	return nil, apperror.InternalKey("usersession.service_missing", nil, "用户会话服务未配置")
}

func (nilSessionAdminService) Revoke(ctx context.Context, id int64, currentSessionID int64) (*authmodule.SessionRevokeResponse, *apperror.Error) {
	return nil, apperror.InternalKey("usersession.service_missing", nil, "用户会话服务未配置")
}

func (nilSessionAdminService) BatchRevoke(ctx context.Context, input authmodule.SessionBatchRevokeInput, currentSessionID int64) (*authmodule.SessionBatchRevokeResponse, *apperror.Error) {
	return nil, apperror.InternalKey("usersession.service_missing", nil, "用户会话服务未配置")
}

type nilLoginLogService struct{}

func (nilLoginLogService) PageInit(ctx context.Context) (*authmodule.LoginLogPageInitResponse, *apperror.Error) {
	return nil, apperror.InternalKey("userloginlog.service_missing", nil, "用户登录日志服务未配置")
}

func (nilLoginLogService) List(ctx context.Context, query authmodule.LoginLogListQuery) (*authmodule.LoginLogListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("userloginlog.service_missing", nil, "用户登录日志服务未配置")
}
