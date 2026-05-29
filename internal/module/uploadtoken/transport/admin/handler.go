package admin

import (
	"context"

	uploadtokenmodule "admin_back_go/internal/module/uploadtoken"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	Create(ctx context.Context, input uploadtokenmodule.CreateInput) (*uploadtokenmodule.CreateResponse, *apperror.Error)
}

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("uploadtoken.request.invalid", nil, "上传 token 参数错误"))
		return
	}
	result, appErr := h.requireService().Create(c.Request.Context(), uploadtokenmodule.CreateInput(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return failingService{}
	}
	return h.service
}

type failingService struct{}

func (failingService) Create(ctx context.Context, input uploadtokenmodule.CreateInput) (*uploadtokenmodule.CreateResponse, *apperror.Error) {
	return nil, apperror.InternalKey("uploadtoken.service_missing", nil, "上传运行时服务未配置")
}

var _ HTTPService = (*uploadtokenmodule.Service)(nil)
