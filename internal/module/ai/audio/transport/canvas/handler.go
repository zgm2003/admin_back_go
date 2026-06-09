package canvas

import (
	"context"
	"net/http"
	"strings"

	"admin_back_go/internal/middleware"
	aiaudiomodule "admin_back_go/internal/module/ai/audio"
	"admin_back_go/internal/module/ai/internal/canvasrequest"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service aiaudiomodule.HTTPService }

func NewHandler(service aiaudiomodule.HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) AudioGenerations(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req audioGenerationRequest
	if !canvasrequest.BindAgentOwnedJSON(c, &req, "canvas.ai.audio.request.invalid", "音频生成参数错误") {
		return
	}
	result, appErr := h.requireService().Generate(c.Request.Context(), aiaudiomodule.GenerateInput{
		UserID: userID, AgentID: req.AgentID, Prompt: req.Prompt,
		Voice: req.Voice, ResponseFormat: req.ResponseFormat, Speed: req.Speed, Instructions: req.Instructions,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if result == nil || len(result.Body) == 0 {
		response.Error(c, apperror.BadRequestKey("canvas.ai.audio.content_empty", nil, "Canvas音频内容为空"))
		return
	}
	contentType := strings.TrimSpace(result.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Data(http.StatusOK, contentType, result.Body)
}

func (h *Handler) requireService() aiaudiomodule.HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func currentUserID(c *gin.Context) (int64, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return 0, false
	}
	if identity.Platform != enum.PlatformCanvas {
		response.Error(c, apperror.UnauthorizedKey("auth.platform.invalid", map[string]any{"platform": identity.Platform}, "Token平台不匹配"))
		return 0, false
	}
	return identity.UserID, true
}

type nilHTTPService struct{}

func (nilHTTPService) Generate(ctx context.Context, input aiaudiomodule.GenerateInput) (*aiaudiomodule.GenerateResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.ai.audio.service_missing", nil, "Canvas音频生成服务未配置")
}
