package appauth

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type UploadTokenService interface {
	Create(ctx context.Context, input uploadtoken.CreateInput) (*uploadtoken.CreateResponse, *apperror.Error)
}

type Handler struct {
	uploadTokenService UploadTokenService
}

func NewHandler(uploadTokenService UploadTokenService) *Handler {
	return &Handler{uploadTokenService: uploadTokenService}
}

func (h *Handler) CreateUploadToken(c *gin.Context) {
	if _, ok := h.currentIdentity(c); !ok {
		return
	}
	if h.uploadTokenService == nil {
		response.Error(c, apperror.InternalKey("uploadtoken.service_missing", nil, "上传运行时服务未配置"))
		return
	}
	var req uploadTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("uploadtoken.request.invalid", nil, "上传 token 参数错误"))
		return
	}
	result, appErr := h.uploadTokenService.Create(c.Request.Context(), uploadtoken.CreateInput{
		Folder:   req.Folder,
		FileName: req.FileName,
		FileSize: req.FileSize,
		FileKind: req.FileKind,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) currentIdentity(c *gin.Context) (*middleware.AuthIdentity, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return nil, false
	}
	if identity.Platform != "" && identity.Platform != enum.PlatformApp {
		response.Error(c, apperror.UnauthorizedKey("auth.platform.invalid", map[string]any{"platform": identity.Platform}, "Token平台不匹配"))
		return nil, false
	}
	return identity, true
}
