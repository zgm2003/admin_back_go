package payment

import (
	"context"
	"strconv"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) ConfigInit(c *gin.Context) {
	result, appErr := h.requireService().ConfigInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (h *Handler) ListConfigs(c *gin.Context) {
	var req listConfigsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付配置列表参数错误"))
		return
	}
	result, appErr := h.requireService().ListConfigs(c.Request.Context(), ConfigListQuery{
		CurrentPage: req.CurrentPage,
		PageSize:    req.PageSize,
		Name:        req.Name,
		Environment: req.Environment,
		Status:      req.Status,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) CreateConfig(c *gin.Context) {
	var req configMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付配置参数错误"))
		return
	}
	id, appErr := h.requireService().CreateConfig(c.Request.Context(), configInput(req))
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{"id": id})
}

func (h *Handler) UpdateConfig(c *gin.Context) {
	id, ok := routeInt64(c, "id", "无效的支付配置ID")
	if !ok {
		return
	}
	var req configMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付配置参数错误"))
		return
	}
	writeEmpty(c, h.requireService().UpdateConfig(c.Request.Context(), id, configInput(req)))
}

func (h *Handler) ChangeConfigStatus(c *gin.Context) {
	id, ok := routeInt64(c, "id", "无效的支付配置ID")
	if !ok {
		return
	}
	var req changeConfigStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("支付配置状态参数错误"))
		return
	}
	writeEmpty(c, h.requireService().ChangeConfigStatus(c.Request.Context(), id, req.Status))
}

func (h *Handler) DeleteConfig(c *gin.Context) {
	id, ok := routeInt64(c, "id", "无效的支付配置ID")
	if !ok {
		return
	}
	writeEmpty(c, h.requireService().DeleteConfig(c.Request.Context(), id))
}

func (h *Handler) UploadCertificate(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Error(c, apperror.BadRequest("请选择支付宝证书文件"))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		response.Error(c, apperror.BadRequest("读取支付宝证书文件失败"))
		return
	}
	defer file.Close()
	result, appErr := h.requireService().UploadCertificate(c.Request.Context(), CertificateUploadInput{
		ConfigCode: c.PostForm("config_code"),
		CertType:   c.PostForm("cert_type"),
		FileName:   fileHeader.Filename,
		Size:       fileHeader.Size,
		Reader:     file,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) TestConfig(c *gin.Context) {
	id, ok := routeInt64(c, "id", "无效的支付配置ID")
	if !ok {
		return
	}
	result, appErr := h.requireService().TestConfig(c.Request.Context(), id)
	writeResult(c, result, appErr)
}

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func routeInt64(c *gin.Context, name string, msg string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest(msg))
		return 0, false
	}
	return id, true
}

func configInput(req configMutationRequest) ConfigMutationInput {
	return ConfigMutationInput{
		Code:               req.Code,
		Name:               req.Name,
		AppID:              req.AppID,
		AppPrivateKey:      req.AppPrivateKey,
		AppCertPath:        req.AppCertPath,
		AlipayCertPath:     req.AlipayCertPath,
		AlipayRootCertPath: req.AlipayRootCertPath,
		NotifyURL:          req.NotifyURL,
		ReturnURL:          req.ReturnURL,
		Environment:        req.Environment,
		EnabledMethods:     req.EnabledMethods,
		Status:             req.Status,
		Remark:             req.Remark,
	}
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func writeEmpty(c *gin.Context, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

type nilHTTPService struct{}

func (nilHTTPService) ConfigInit(ctx context.Context) (*ConfigInitResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) ListConfigs(ctx context.Context, query ConfigListQuery) (*ConfigListResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) CreateConfig(ctx context.Context, input ConfigMutationInput) (int64, *apperror.Error) {
	return 0, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) UpdateConfig(ctx context.Context, id int64, input ConfigMutationInput) *apperror.Error {
	return apperror.Internal("支付服务未配置")
}
func (nilHTTPService) ChangeConfigStatus(ctx context.Context, id int64, status int) *apperror.Error {
	return apperror.Internal("支付服务未配置")
}
func (nilHTTPService) DeleteConfig(ctx context.Context, id int64) *apperror.Error {
	return apperror.Internal("支付服务未配置")
}
func (nilHTTPService) UploadCertificate(ctx context.Context, input CertificateUploadInput) (*CertificateUploadResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
func (nilHTTPService) TestConfig(ctx context.Context, id int64) (*ConfigTestResponse, *apperror.Error) {
	return nil, apperror.Internal("支付服务未配置")
}
