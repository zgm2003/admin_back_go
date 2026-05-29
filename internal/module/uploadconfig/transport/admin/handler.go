package admin

import (
	"context"
	"strconv"

	uploadconfigmodule "admin_back_go/internal/module/uploadconfig"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	DriverInit(ctx context.Context) (*uploadconfigmodule.DriverInitResponse, *apperror.Error)
	DriverList(ctx context.Context, query uploadconfigmodule.DriverListQuery) (*uploadconfigmodule.DriverListResponse, *apperror.Error)
	CreateDriver(ctx context.Context, input uploadconfigmodule.DriverCreateInput) (int64, *apperror.Error)
	UpdateDriver(ctx context.Context, id int64, input uploadconfigmodule.DriverUpdateInput) *apperror.Error
	DeleteDrivers(ctx context.Context, ids []int64) *apperror.Error
	RuleInit(ctx context.Context) (*uploadconfigmodule.RuleInitResponse, *apperror.Error)
	RuleList(ctx context.Context, query uploadconfigmodule.RuleListQuery) (*uploadconfigmodule.RuleListResponse, *apperror.Error)
	CreateRule(ctx context.Context, input uploadconfigmodule.RuleMutationInput) (int64, *apperror.Error)
	UpdateRule(ctx context.Context, id int64, input uploadconfigmodule.RuleMutationInput) *apperror.Error
	DeleteRules(ctx context.Context, ids []int64) *apperror.Error
	SettingInit(ctx context.Context) (*uploadconfigmodule.SettingInitResponse, *apperror.Error)
	SettingList(ctx context.Context, query uploadconfigmodule.SettingListQuery) (*uploadconfigmodule.SettingListResponse, *apperror.Error)
	CreateSetting(ctx context.Context, input uploadconfigmodule.SettingMutationInput) (int64, *apperror.Error)
	UpdateSetting(ctx context.Context, id int64, input uploadconfigmodule.SettingMutationInput) *apperror.Error
	ChangeSettingStatus(ctx context.Context, id int64, status int) *apperror.Error
	DeleteSettings(ctx context.Context, ids []int64) *apperror.Error
}

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) DriverInit(c *gin.Context) {
	result, appErr := h.requireService().DriverInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) DriverList(c *gin.Context) {
	var req driverListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("上传驱动列表参数错误"))
		return
	}
	result, appErr := h.requireService().DriverList(c.Request.Context(), uploadconfigmodule.DriverListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		Driver:      req.Driver,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) DriverCreate(c *gin.Context) {
	var req driverCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("上传驱动参数错误"))
		return
	}
	id, appErr := h.requireService().CreateDriver(c.Request.Context(), uploadconfigmodule.DriverCreateInput{
		Driver: req.Driver, SecretID: req.SecretID, SecretKey: req.SecretKey, Bucket: req.Bucket, Region: req.Region,
		RoleARN: req.RoleARN, AppID: req.AppID, Endpoint: req.Endpoint, BucketDomain: req.BucketDomain,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}

func (h *Handler) DriverUpdate(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req driverUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("上传驱动参数错误"))
		return
	}
	appErr := h.requireService().UpdateDriver(c.Request.Context(), id, uploadconfigmodule.DriverUpdateInput{
		Driver: req.Driver, SecretID: req.SecretID, SecretKey: req.SecretKey, Bucket: req.Bucket, Region: req.Region,
		RoleARN: req.RoleARN, AppID: req.AppID, Endpoint: req.Endpoint, BucketDomain: req.BucketDomain,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DriverDeleteOne(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.requireService().DeleteDrivers(c.Request.Context(), []int64{id}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DriverDeleteBatch(c *gin.Context) {
	var req deleteBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("请选择要删除的上传驱动"))
		return
	}
	if appErr := h.requireService().DeleteDrivers(c.Request.Context(), req.IDs); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) RuleInit(c *gin.Context) {
	result, appErr := h.requireService().RuleInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) RuleList(c *gin.Context) {
	var req ruleListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("上传规则列表参数错误"))
		return
	}
	result, appErr := h.requireService().RuleList(c.Request.Context(), uploadconfigmodule.RuleListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		Title:       req.Title,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) RuleCreate(c *gin.Context) {
	var req ruleMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("上传规则参数错误"))
		return
	}
	id, appErr := h.requireService().CreateRule(c.Request.Context(), uploadconfigmodule.RuleMutationInput{
		Title: req.Title, MaxSizeMB: req.MaxSizeMB, ImageExts: req.ImageExts, FileExts: req.FileExts,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}

func (h *Handler) RuleUpdate(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req ruleMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("上传规则参数错误"))
		return
	}
	if appErr := h.requireService().UpdateRule(c.Request.Context(), id, uploadconfigmodule.RuleMutationInput{
		Title: req.Title, MaxSizeMB: req.MaxSizeMB, ImageExts: req.ImageExts, FileExts: req.FileExts,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) RuleDeleteOne(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.requireService().DeleteRules(c.Request.Context(), []int64{id}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) RuleDeleteBatch(c *gin.Context) {
	var req deleteBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("请选择要删除的上传规则"))
		return
	}
	if appErr := h.requireService().DeleteRules(c.Request.Context(), req.IDs); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) SettingInit(c *gin.Context) {
	result, appErr := h.requireService().SettingInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) SettingList(c *gin.Context) {
	var req settingListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("上传设置列表参数错误"))
		return
	}
	result, appErr := h.requireService().SettingList(c.Request.Context(), uploadconfigmodule.SettingListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		Remark:      req.Remark,
		Status:      req.Status,
		DriverID:    req.DriverID,
		RuleID:      req.RuleID,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) SettingCreate(c *gin.Context) {
	var req settingMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("上传设置参数错误"))
		return
	}
	id, appErr := h.requireService().CreateSetting(c.Request.Context(), uploadconfigmodule.SettingMutationInput{
		DriverID: req.DriverID,
		RuleID:   req.RuleID,
		Status:   req.Status,
		Remark:   req.Remark,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}

func (h *Handler) SettingUpdate(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req settingMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("上传设置参数错误"))
		return
	}
	if appErr := h.requireService().UpdateSetting(c.Request.Context(), id, uploadconfigmodule.SettingMutationInput{
		DriverID: req.DriverID,
		RuleID:   req.RuleID,
		Status:   req.Status,
		Remark:   req.Remark,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) SettingChangeStatus(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("无效的状态"))
		return
	}
	if appErr := h.requireService().ChangeSettingStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) SettingDeleteOne(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.requireService().DeleteSettings(c.Request.Context(), []int64{id}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) SettingDeleteBatch(c *gin.Context) {
	var req deleteBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("请选择要删除的上传设置"))
		return
	}
	if appErr := h.requireService().DeleteSettings(c.Request.Context(), req.IDs); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return failingService{}
	}
	return h.service
}

type failingService struct{}

func (failingService) DriverInit(ctx context.Context) (*uploadconfigmodule.DriverInitResponse, *apperror.Error) {
	return nil, apperror.Internal("上传配置服务未配置")
}

func (failingService) DriverList(ctx context.Context, query uploadconfigmodule.DriverListQuery) (*uploadconfigmodule.DriverListResponse, *apperror.Error) {
	return nil, apperror.Internal("上传配置服务未配置")
}

func (failingService) CreateDriver(ctx context.Context, input uploadconfigmodule.DriverCreateInput) (int64, *apperror.Error) {
	return 0, apperror.Internal("上传配置服务未配置")
}

func (failingService) UpdateDriver(ctx context.Context, id int64, input uploadconfigmodule.DriverUpdateInput) *apperror.Error {
	return apperror.Internal("上传配置服务未配置")
}

func (failingService) DeleteDrivers(ctx context.Context, ids []int64) *apperror.Error {
	return apperror.Internal("上传配置服务未配置")
}

func (failingService) RuleInit(ctx context.Context) (*uploadconfigmodule.RuleInitResponse, *apperror.Error) {
	return nil, apperror.Internal("上传配置服务未配置")
}

func (failingService) RuleList(ctx context.Context, query uploadconfigmodule.RuleListQuery) (*uploadconfigmodule.RuleListResponse, *apperror.Error) {
	return nil, apperror.Internal("上传配置服务未配置")
}

func (failingService) CreateRule(ctx context.Context, input uploadconfigmodule.RuleMutationInput) (int64, *apperror.Error) {
	return 0, apperror.Internal("上传配置服务未配置")
}

func (failingService) UpdateRule(ctx context.Context, id int64, input uploadconfigmodule.RuleMutationInput) *apperror.Error {
	return apperror.Internal("上传配置服务未配置")
}

func (failingService) DeleteRules(ctx context.Context, ids []int64) *apperror.Error {
	return apperror.Internal("上传配置服务未配置")
}

func (failingService) SettingInit(ctx context.Context) (*uploadconfigmodule.SettingInitResponse, *apperror.Error) {
	return nil, apperror.Internal("上传配置服务未配置")
}

func (failingService) SettingList(ctx context.Context, query uploadconfigmodule.SettingListQuery) (*uploadconfigmodule.SettingListResponse, *apperror.Error) {
	return nil, apperror.Internal("上传配置服务未配置")
}

func (failingService) CreateSetting(ctx context.Context, input uploadconfigmodule.SettingMutationInput) (int64, *apperror.Error) {
	return 0, apperror.Internal("上传配置服务未配置")
}

func (failingService) UpdateSetting(ctx context.Context, id int64, input uploadconfigmodule.SettingMutationInput) *apperror.Error {
	return apperror.Internal("上传配置服务未配置")
}

func (failingService) ChangeSettingStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return apperror.Internal("上传配置服务未配置")
}

func (failingService) DeleteSettings(ctx context.Context, ids []int64) *apperror.Error {
	return apperror.Internal("上传配置服务未配置")
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest("无效的上传配置ID"))
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

var _ HTTPService = (*uploadconfigmodule.Service)(nil)
