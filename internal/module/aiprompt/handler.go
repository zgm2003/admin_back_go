package aiprompt

import (
	"context"
	"strconv"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI提示词列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), identity.UserID, ListQuery{
		CurrentPage: req.CurrentPage, PageSize: req.PageSize, Title: req.Title, Category: req.Category, IsFavorite: req.IsFavorite,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) Detail(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().Detail(c.Request.Context(), identity.UserID, id)
	writeResult(c, result, appErr)
}

func (h *Handler) Create(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI提示词参数错误"))
		return
	}
	id, appErr := h.requireService().Create(c.Request.Context(), identity.UserID, CreateInput{
		Title: req.Title, Content: req.Content, Category: req.Category, Tags: req.Tags, Variables: req.Variables,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}

func (h *Handler) Update(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("AI提示词参数错误"))
		return
	}
	if appErr := h.requireService().Update(c.Request.Context(), identity.UserID, id, UpdateInput{
		Title: req.Title, Content: req.Content, Category: req.Category, Tags: req.Tags, Variables: req.Variables,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) Delete(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.requireService().Delete(c.Request.Context(), identity.UserID, id); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ToggleFavorite(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().ToggleFavorite(c.Request.Context(), identity.UserID, id)
	writeResult(c, result, appErr)
}

func (h *Handler) Use(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().Use(c.Request.Context(), identity.UserID, id)
	writeResult(c, result, appErr)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest("无效的AI提示词ID"))
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

func (nilHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return nil, apperror.Internal("AI提示词服务未配置")
}
func (nilHTTPService) List(ctx context.Context, userID int64, query ListQuery) (*ListResponse, *apperror.Error) {
	return nil, apperror.Internal("AI提示词服务未配置")
}
func (nilHTTPService) Detail(ctx context.Context, userID int64, id int64) (*DetailResponse, *apperror.Error) {
	return nil, apperror.Internal("AI提示词服务未配置")
}
func (nilHTTPService) Create(ctx context.Context, userID int64, input CreateInput) (int64, *apperror.Error) {
	return 0, apperror.Internal("AI提示词服务未配置")
}
func (nilHTTPService) Update(ctx context.Context, userID int64, id int64, input UpdateInput) *apperror.Error {
	return apperror.Internal("AI提示词服务未配置")
}
func (nilHTTPService) Delete(ctx context.Context, userID int64, id int64) *apperror.Error {
	return apperror.Internal("AI提示词服务未配置")
}
func (nilHTTPService) ToggleFavorite(ctx context.Context, userID int64, id int64) (*ToggleFavoriteResponse, *apperror.Error) {
	return nil, apperror.Internal("AI提示词服务未配置")
}
func (nilHTTPService) Use(ctx context.Context, userID int64, id int64) (*UseResponse, *apperror.Error) {
	return nil, apperror.Internal("AI提示词服务未配置")
}
