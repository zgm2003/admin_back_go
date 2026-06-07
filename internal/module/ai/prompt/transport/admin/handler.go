package admin

import (
	"context"
	"strconv"

	promptmodule "admin_back_go/internal/module/ai/prompt"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	PageInit(ctx context.Context) (*promptmodule.PageInitResponse, *apperror.Error)
	List(ctx context.Context, query promptmodule.ListQuery) (*promptmodule.ListResponse, *apperror.Error)
	Detail(ctx context.Context, id int64) (*promptmodule.Item, *apperror.Error)
	Create(ctx context.Context, input promptmodule.Input) (int64, *apperror.Error)
	Update(ctx context.Context, id int64, input promptmodule.Input) *apperror.Error
	ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error
	DeleteOne(ctx context.Context, id int64) *apperror.Error
	DeleteBatch(ctx context.Context, ids []int64) *apperror.Error
}

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) PageInit(c *gin.Context) {
	result, appErr := h.requireService().PageInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) List(c *gin.Context) {
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("ai.prompt.request.invalid", nil, "提示词列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), promptmodule.ListQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Keyword: req.Keyword, Category: req.Category, Status: req.Status})
	writeResult(c, result, appErr)
}

func (h *Handler) Detail(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().Detail(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) Create(c *gin.Context) {
	var req promptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("ai.prompt.request.invalid", nil, "提示词参数错误"))
		return
	}
	id, appErr := h.requireService().Create(c.Request.Context(), input(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{"id": id}, "ai.prompt.create_success", nil, "创建AI提示词成功")
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req promptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("ai.prompt.request.invalid", nil, "提示词参数错误"))
		return
	}
	if appErr := h.requireService().Update(c.Request.Context(), id, input(req)); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{}, "ai.prompt.update_success", nil, "更新AI提示词成功")
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("ai.prompt.status.invalid", nil, "提示词状态无效"))
		return
	}
	if appErr := h.requireService().ChangeStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{}, "ai.prompt.status_success", nil, "修改AI提示词状态成功")
}

func (h *Handler) DeleteOne(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.requireService().DeleteOne(c.Request.Context(), id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{}, "ai.prompt.delete_success", nil, "删除AI提示词成功")
}

func (h *Handler) DeleteBatch(c *gin.Context) {
	var req deleteBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("ai.prompt.ids.invalid", nil, "提示词ID列表无效"))
		return
	}
	if appErr := h.requireService().DeleteBatch(c.Request.Context(), req.IDs); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OKWithMessageKey(c, gin.H{}, "ai.prompt.delete_batch_success", nil, "批量删除AI提示词成功")
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return failingService{}
	}
	return h.service
}

type failingService struct{}

func (failingService) PageInit(ctx context.Context) (*promptmodule.PageInitResponse, *apperror.Error) {
	return nil, apperror.InternalKey("ai.prompt.service_missing", nil, "AI提示词服务未配置")
}
func (failingService) List(ctx context.Context, query promptmodule.ListQuery) (*promptmodule.ListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("ai.prompt.service_missing", nil, "AI提示词服务未配置")
}
func (failingService) Detail(ctx context.Context, id int64) (*promptmodule.Item, *apperror.Error) {
	return nil, apperror.InternalKey("ai.prompt.service_missing", nil, "AI提示词服务未配置")
}
func (failingService) Create(ctx context.Context, input promptmodule.Input) (int64, *apperror.Error) {
	return 0, apperror.InternalKey("ai.prompt.service_missing", nil, "AI提示词服务未配置")
}
func (failingService) Update(ctx context.Context, id int64, input promptmodule.Input) *apperror.Error {
	return apperror.InternalKey("ai.prompt.service_missing", nil, "AI提示词服务未配置")
}
func (failingService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return apperror.InternalKey("ai.prompt.service_missing", nil, "AI提示词服务未配置")
}
func (failingService) DeleteOne(ctx context.Context, id int64) *apperror.Error {
	return apperror.InternalKey("ai.prompt.service_missing", nil, "AI提示词服务未配置")
}
func (failingService) DeleteBatch(ctx context.Context, ids []int64) *apperror.Error {
	return apperror.InternalKey("ai.prompt.service_missing", nil, "AI提示词服务未配置")
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequestKey("ai.prompt.id.invalid", nil, "提示词ID无效"))
		return 0, false
	}
	return id, true
}

func input(req promptRequest) promptmodule.Input {
	return promptmodule.Input{Slug: req.Slug, Category: req.Category, Title: req.Title, CoverURL: req.CoverURL, Prompt: req.Prompt, Preview: req.Preview, TagsJSON: req.TagsJSON, SourceURL: req.SourceURL, Status: req.Status}
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}
