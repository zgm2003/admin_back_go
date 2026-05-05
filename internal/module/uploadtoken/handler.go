package uploadtoken

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	Create(ctx context.Context, input CreateInput) (*CreateResponse, *apperror.Error)
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
		response.Error(c, apperror.BadRequest("上传 token 参数错误"))
		return
	}
	result, appErr := h.requireService().Create(c.Request.Context(), CreateInput{
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

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return failingService{}
	}
	return h.service
}

type failingService struct{}

func (failingService) Create(ctx context.Context, input CreateInput) (*CreateResponse, *apperror.Error) {
	return nil, apperror.Internal("上传运行时服务未配置")
}
