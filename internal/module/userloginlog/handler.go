package userloginlog

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) PageInit(c *gin.Context) {
	result, appErr := h.requireService().PageInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("userloginlog.request.invalid", nil, "用户登录日志列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), ListQuery{
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
	writeResult(c, result, appErr)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

type nilHTTPService struct{}

func (nilHTTPService) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	return nil, apperror.InternalKey("userloginlog.service_missing", nil, "用户登录日志服务未配置")
}

func (nilHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("userloginlog.service_missing", nil, "用户登录日志服务未配置")
}
