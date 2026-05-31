package admin

import (
	"context"
	"strconv"

	aibilling "admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service aibilling.HTTPService }

func NewHandler(service aibilling.HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) PageInit(c *gin.Context) {
	result, appErr := h.requireService().PageInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("aibilling.request.invalid", nil, "AI计费规则参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), aibilling.ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Scene: req.Scene, Unit: req.Unit, Status: req.Status})
	writeResult(c, result, appErr)
}

func (h *Handler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("aibilling.request.invalid", nil, "AI计费规则参数错误"))
		return
	}
	id, appErr := h.requireService().CreateRule(c.Request.Context(), aibilling.CreateRuleInput{Scene: req.Scene, Unit: req.Unit, UnitPriceCents: req.UnitPriceCents, Status: req.Status})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("aibilling.request.invalid", nil, "AI计费规则参数错误"))
		return
	}
	if appErr := h.requireService().UpdateRule(c.Request.Context(), id, aibilling.UpdateRuleInput{Unit: req.Unit, UnitPriceCents: req.UnitPriceCents, Status: req.Status}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("aibilling.request.invalid", nil, "AI计费规则参数错误"))
		return
	}
	if appErr := h.requireService().ChangeStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DeleteOne(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.requireService().DeleteRule(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) requireService() aibilling.HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func routeID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperror.BadRequestKey("aibilling.rule.id.invalid", nil, "AI计费规则ID无效"))
		return 0, false
	}
	return id, true
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

type nilHTTPService struct{}

func (nilHTTPService) PageInit(ctx context.Context) (*aibilling.PageInitResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aibilling.service_missing", nil, "AI计费服务未配置")
}
func (nilHTTPService) List(ctx context.Context, query aibilling.ListQuery) (*aibilling.ListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aibilling.service_missing", nil, "AI计费服务未配置")
}
func (nilHTTPService) CreateRule(ctx context.Context, input aibilling.CreateRuleInput) (uint64, *apperror.Error) {
	return 0, apperror.InternalKey("aibilling.service_missing", nil, "AI计费服务未配置")
}
func (nilHTTPService) UpdateRule(ctx context.Context, id uint64, input aibilling.UpdateRuleInput) *apperror.Error {
	return apperror.InternalKey("aibilling.service_missing", nil, "AI计费服务未配置")
}
func (nilHTTPService) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return apperror.InternalKey("aibilling.service_missing", nil, "AI计费服务未配置")
}
func (nilHTTPService) DeleteRule(ctx context.Context, id uint64) *apperror.Error {
	return apperror.InternalKey("aibilling.service_missing", nil, "AI计费服务未配置")
}
func (nilHTTPService) EnabledRule(ctx context.Context, scene string) (*aibilling.RuleDTO, *apperror.Error) {
	return nil, apperror.InternalKey("aibilling.service_missing", nil, "AI计费服务未配置")
}
