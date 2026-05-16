package sms

import (
	"context"
	"strconv"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error)
	Config(ctx context.Context) (*ConfigResponse, *apperror.Error)
	SaveConfig(ctx context.Context, input SaveConfigInput) *apperror.Error
	DeleteConfig(ctx context.Context) *apperror.Error
	TestSend(ctx context.Context, input TestInput) *apperror.Error
	Templates(ctx context.Context) ([]TemplateDTO, *apperror.Error)
	CreateTemplate(ctx context.Context, input SaveTemplateInput) (uint64, *apperror.Error)
	UpdateTemplate(ctx context.Context, id uint64, input SaveTemplateInput) *apperror.Error
	ChangeTemplateStatus(ctx context.Context, id uint64, status int) *apperror.Error
	DeleteTemplate(ctx context.Context, id uint64) *apperror.Error
	Logs(ctx context.Context, query LogQuery) (*LogListResponse, *apperror.Error)
	Log(ctx context.Context, id uint64) (*LogDTO, *apperror.Error)
	DeleteLogs(ctx context.Context, ids []uint64) *apperror.Error
}

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) PageInit(c *gin.Context) {
	result, appErr := h.requireService().PageInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) Config(c *gin.Context) {
	result, appErr := h.requireService().Config(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) SaveConfig(c *gin.Context) {
	var req saveConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("sms.config.request.invalid", nil, "短信配置参数错误"))
		return
	}
	appErr := h.requireService().SaveConfig(c.Request.Context(), SaveConfigInput{
		SecretID: req.SecretID, SecretKey: req.SecretKey, SmsSdkAppID: req.SmsSdkAppID, SignName: req.SignName,
		Region: req.Region, Endpoint: req.Endpoint, Status: req.Status, VerifyCodeTTLMinutes: req.VerifyCodeTTLMinutes,
	})
	writeResult(c, gin.H{}, appErr)
}

func (h *Handler) DeleteConfig(c *gin.Context) {
	writeResult(c, gin.H{}, h.requireService().DeleteConfig(c.Request.Context()))
}

func (h *Handler) TestSend(c *gin.Context) {
	var req testRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("sms.test.request.invalid", nil, "测试短信参数错误"))
		return
	}
	appErr := h.requireService().TestSend(c.Request.Context(), TestInput{ToPhone: req.ToPhone, TemplateScene: req.TemplateScene})
	writeResult(c, gin.H{}, appErr)
}

func (h *Handler) Templates(c *gin.Context) {
	result, appErr := h.requireService().Templates(c.Request.Context())
	writeResult(c, gin.H{"list": result}, appErr)
}

func (h *Handler) CreateTemplate(c *gin.Context) {
	var req templateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("sms.template.request.invalid", nil, "短信模板参数错误"))
		return
	}
	id, appErr := h.requireService().CreateTemplate(c.Request.Context(), templateInput(req))
	writeResult(c, gin.H{"id": id}, appErr)
}

func (h *Handler) UpdateTemplate(c *gin.Context) {
	id, ok := routeID(c, "sms.template.id.invalid", "无效的短信模板ID")
	if !ok {
		return
	}
	var req templateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("sms.template.request.invalid", nil, "短信模板参数错误"))
		return
	}
	writeResult(c, gin.H{}, h.requireService().UpdateTemplate(c.Request.Context(), id, templateInput(req)))
}

func (h *Handler) ChangeTemplateStatus(c *gin.Context) {
	id, ok := routeID(c, "sms.template.id.invalid", "无效的短信模板ID")
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("sms.status.invalid", nil, "无效的状态"))
		return
	}
	writeResult(c, gin.H{}, h.requireService().ChangeTemplateStatus(c.Request.Context(), id, req.Status))
}

func (h *Handler) DeleteTemplate(c *gin.Context) {
	id, ok := routeID(c, "sms.template.id.invalid", "无效的短信模板ID")
	if !ok {
		return
	}
	writeResult(c, gin.H{}, h.requireService().DeleteTemplate(c.Request.Context(), id))
}

func (h *Handler) Logs(c *gin.Context) {
	var req logListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("sms.log.list.request.invalid", nil, "短信日志列表参数错误"))
		return
	}
	query, appErr := logQueryFromRequest(req)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	result, appErr := h.requireService().Logs(c.Request.Context(), query)
	writeResult(c, result, appErr)
}

func (h *Handler) Log(c *gin.Context) {
	id, ok := routeID(c, "sms.log.id.invalid", "无效的短信日志ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().Log(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) DeleteLog(c *gin.Context) {
	id, ok := routeID(c, "sms.log.id.invalid", "无效的短信日志ID")
	if !ok {
		return
	}
	writeResult(c, gin.H{}, h.requireService().DeleteLogs(c.Request.Context(), []uint64{id}))
}

func (h *Handler) DeleteLogs(c *gin.Context) {
	var req deleteLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("sms.log.delete.required", nil, "请选择要删除的短信日志"))
		return
	}
	writeResult(c, gin.H{}, h.requireService().DeleteLogs(c.Request.Context(), req.IDs))
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return failingService{}
	}
	return h.service
}

type failingService struct{}

func (failingService) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	return nil, serviceNotConfigured()
}
func (failingService) Config(ctx context.Context) (*ConfigResponse, *apperror.Error) {
	return nil, serviceNotConfigured()
}
func (failingService) SaveConfig(ctx context.Context, input SaveConfigInput) *apperror.Error {
	return serviceNotConfigured()
}
func (failingService) DeleteConfig(ctx context.Context) *apperror.Error {
	return serviceNotConfigured()
}
func (failingService) TestSend(ctx context.Context, input TestInput) *apperror.Error {
	return serviceNotConfigured()
}
func (failingService) Templates(ctx context.Context) ([]TemplateDTO, *apperror.Error) {
	return nil, serviceNotConfigured()
}
func (failingService) CreateTemplate(ctx context.Context, input SaveTemplateInput) (uint64, *apperror.Error) {
	return 0, serviceNotConfigured()
}
func (failingService) UpdateTemplate(ctx context.Context, id uint64, input SaveTemplateInput) *apperror.Error {
	return serviceNotConfigured()
}
func (failingService) ChangeTemplateStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	return serviceNotConfigured()
}
func (failingService) DeleteTemplate(ctx context.Context, id uint64) *apperror.Error {
	return serviceNotConfigured()
}
func (failingService) Logs(ctx context.Context, query LogQuery) (*LogListResponse, *apperror.Error) {
	return nil, serviceNotConfigured()
}
func (failingService) Log(ctx context.Context, id uint64) (*LogDTO, *apperror.Error) {
	return nil, serviceNotConfigured()
}
func (failingService) DeleteLogs(ctx context.Context, ids []uint64) *apperror.Error {
	return serviceNotConfigured()
}

func serviceNotConfigured() *apperror.Error {
	return apperror.InternalKey("sms.service_missing", nil, "短信服务未配置")
}

func routeID(c *gin.Context, key string, fallback string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperror.BadRequestKey(key, nil, fallback))
		return 0, false
	}
	return id, true
}

func templateInput(req templateRequest) SaveTemplateInput {
	return SaveTemplateInput{Scene: req.Scene, Name: req.Name, TencentTemplateID: req.TencentTemplateID, Variables: req.Variables, SampleVariables: req.SampleVariables, Status: req.Status}
}

func logQueryFromRequest(req logListRequest) (LogQuery, *apperror.Error) {
	start, appErr := parseTime(req.CreatedAtStart)
	if appErr != nil {
		return LogQuery{}, appErr
	}
	end, appErr := parseTime(req.CreatedAtEnd)
	if appErr != nil {
		return LogQuery{}, appErr
	}
	return LogQuery{CurrentPage: req.CurrentPage, PageSize: req.PageSize, Scene: req.Scene, Status: req.Status, ToPhone: req.ToPhone, CreatedAtStart: start, CreatedAtEnd: end}, nil
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}
