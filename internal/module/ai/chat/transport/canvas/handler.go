package canvas

import (
	"context"
	"strings"

	"admin_back_go/internal/middleware"
	aichatmodule "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/module/ai/internal/canvasrequest"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service aichatmodule.HTTPService
}

func NewHandler(service aichatmodule.HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ChatCompletions(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req chatCompletionRequest
	if !canvasrequest.BindAgentOwnedJSON(c, &req, "canvas.ai.chat.request.invalid", "文本生成参数错误") {
		return
	}
	result, appErr := h.requireService().CanvasCompletion(c.Request.Context(), aichatmodule.CanvasCompletionInput{
		UserID:  userID,
		AgentID: req.AgentID,
		Message: req.Message,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if result == nil || strings.TrimSpace(result.Content) == "" {
		response.Error(c, apperror.InternalKey("canvas.ai.chat.result_invalid", nil, "Canvas文本生成结果无效"))
		return
	}
	response.OK(c, result)
}

func (h *Handler) requireService() aichatmodule.HTTPService {
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

func (nilHTTPService) CanvasCompletion(ctx context.Context, input aichatmodule.CanvasCompletionInput) (*aichatmodule.CanvasCompletionResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.ai.chat.service_missing", nil, "Canvas文本生成服务未配置")
}
