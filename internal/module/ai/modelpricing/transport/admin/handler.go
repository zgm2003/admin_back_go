package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/ai/modelpricing"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

const updateBodyLimit int64 = 64 << 10

type Handler struct{ service modelpricing.HTTPService }

func NewHandler(service modelpricing.HTTPService) *Handler { return &Handler{service: service} }

func (handler *Handler) PageInit(c *gin.Context) {
	if !validQuery(c) {
		response.Error(c, requestError())
		return
	}
	result, appErr := handler.requireService().PageInit(c.Request.Context())
	writeResult(c, result, appErr)
}

func (handler *Handler) List(c *gin.Context) {
	if !validQuery(c, "family", "model_id") {
		response.Error(c, requestError())
		return
	}
	var request listRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		response.Error(c, requestError())
		return
	}
	result, appErr := handler.requireService().List(c.Request.Context(), modelpricing.ListQuery{Family: request.Family, ModelID: request.ModelID})
	writeResult(c, result, appErr)
}

func (handler *Handler) Detail(c *gin.Context) {
	if !validQuery(c) {
		response.Error(c, requestError())
		return
	}
	result, appErr := handler.requireService().Detail(c.Request.Context(), c.Param("model_id"))
	writeResult(c, result, appErr)
}

func (handler *Handler) Update(c *gin.Context) {
	if !validQuery(c) {
		response.Error(c, requestError())
		return
	}
	administratorID, ok := currentAdministratorID(c)
	if !ok {
		return
	}
	var request updateRequest
	if !bindStrictJSON(c, updateBodyLimit, &request) {
		response.Error(c, requestError())
		return
	}
	prices := make([]modelpricing.RatePriceInput, len(request.Rates))
	for index, rate := range request.Rates {
		prices[index] = modelpricing.RatePriceInput{
			Category: pricing.Category(rate.Category), Unit: rate.Unit, TierKey: *rate.TierKey,
			UnitScale: rate.UnitScale, Price: rate.Price,
		}
	}
	summary, appErr := handler.requireService().SetOverride(c.Request.Context(), c.Param("model_id"), modelpricing.SetOverrideInput{
		ExpectedVersion: *request.ExpectedVersion, Prices: prices, SourceURL: request.SourceURL,
		VerifiedAt: request.VerifiedAt, AdministratorID: administratorID,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	result, err := modelpricing.MutationResponseFromSummary(summary)
	if err != nil {
		response.Error(c, apperror.Internal("模型定价变更结果无效"))
		return
	}
	response.OK(c, result)
}

func (handler *Handler) RestoreOfficial(c *gin.Context) {
	administratorID, ok := currentAdministratorID(c)
	if !ok {
		return
	}
	if !validQuery(c, "expected_version") {
		response.Error(c, requestError())
		return
	}
	var request restoreRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		response.Error(c, requestError())
		return
	}
	summary, appErr := handler.requireService().RestoreOfficial(c.Request.Context(), c.Param("model_id"), request.ExpectedVersion, administratorID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	result, err := modelpricing.MutationResponseFromSummary(summary)
	if err != nil {
		response.Error(c, apperror.Internal("模型定价恢复结果无效"))
		return
	}
	response.OK(c, result)
}

func (handler *Handler) requireService() modelpricing.HTTPService {
	if handler == nil || handler.service == nil {
		return nilHTTPService{}
	}
	return handler.service
}

func currentAdministratorID(c *gin.Context) (uint64, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return 0, false
	}
	return uint64(identity.UserID), true
}

func bindStrictJSON(c *gin.Context, limit int64, target any) bool {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return false
	}
	return binding.Validator.ValidateStruct(target) == nil
}

func validQuery(c *gin.Context, allowed ...string) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	set := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		set[key] = struct{}{}
	}
	for key, values := range c.Request.URL.Query() {
		if _, ok := set[key]; !ok || len(values) != 1 {
			return false
		}
	}
	return true
}

func requestError() *apperror.Error {
	return apperror.Wrap(modelpricing.ErrorCodeInvalidOverride, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent,
		modelpricing.ErrorCodeInvalidOverride, nil, "模型定价请求参数无效", nil)
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

type nilHTTPService struct{}

func (nilHTTPService) PageInit(context.Context) (*modelpricing.PageInitResponse, *apperror.Error) {
	return nil, apperror.Internal("模型定价服务未配置")
}
func (nilHTTPService) List(context.Context, modelpricing.ListQuery) (*modelpricing.ListResponse, *apperror.Error) {
	return nil, apperror.Internal("模型定价服务未配置")
}
func (nilHTTPService) Detail(context.Context, string) (*modelpricing.ModelPriceDTO, *apperror.Error) {
	return nil, apperror.Internal("模型定价服务未配置")
}
func (nilHTTPService) SetOverride(context.Context, string, modelpricing.SetOverrideInput) (*modelpricing.MutationSummary, *apperror.Error) {
	return nil, apperror.Internal("模型定价服务未配置")
}
func (nilHTTPService) RestoreOfficial(context.Context, string, int64, uint64) (*modelpricing.MutationSummary, *apperror.Error) {
	return nil, apperror.Internal("模型定价服务未配置")
}
