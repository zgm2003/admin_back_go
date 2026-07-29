package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

const updateBodyLimit int64 = 64 << 10

type Handler struct{ service officialmodel.HTTPService }

func NewHandler(service officialmodel.HTTPService) *Handler { return &Handler{service: service} }

func (handler *Handler) PageInit(context *gin.Context) {
	if !validQuery(context) {
		response.Error(context, requestError())
		return
	}
	result, appErr := handler.requireService().PageInit(context.Request.Context())
	writeResult(context, result, appErr)
}

func (handler *Handler) List(context *gin.Context) {
	if !validQuery(context, "vendor", "family", "lifecycle", "input_modality", "model_id") {
		response.Error(context, requestError())
		return
	}
	var request listRequest
	if err := context.ShouldBindQuery(&request); err != nil {
		response.Error(context, requestError())
		return
	}
	result, appErr := handler.requireService().List(context.Request.Context(), officialmodel.ListQuery{
		Vendor: request.Vendor, Family: request.Family, Lifecycle: officialmodel.LifecycleStatus(request.Lifecycle),
		InputModality: request.InputModality, ModelID: request.ModelID,
	})
	writeResult(context, result, appErr)
}

func (handler *Handler) Detail(context *gin.Context) {
	if !validQuery(context) {
		response.Error(context, requestError())
		return
	}
	result, appErr := handler.requireService().Detail(context.Request.Context(), context.Param("model_id"))
	writeResult(context, result, appErr)
}

func (handler *Handler) SyncPrice(context *gin.Context) {
	if !validQuery(context) {
		response.Error(context, requestError())
		return
	}
	administratorID, ok := currentAdministratorID(context)
	if !ok {
		return
	}
	var request updateRequest
	if !bindStrictJSON(context, updateBodyLimit, &request) {
		response.Error(context, requestError())
		return
	}
	prices := make([]officialmodel.RatePriceInput, len(request.Rates))
	for index, rate := range request.Rates {
		prices[index] = officialmodel.RatePriceInput{
			Category: pricing.Category(rate.Category), Unit: rate.Unit, TierKey: *rate.TierKey,
			UnitScale: rate.UnitScale, Price: rate.Price,
		}
	}
	summary, appErr := handler.requireService().SetPriceOverride(context.Request.Context(), context.Param("model_id"), officialmodel.SetPriceOverrideInput{
		ExpectedVersion: *request.ExpectedVersion, Prices: prices, SourceURL: request.SourceURL,
		VerifiedAt: request.VerifiedAt, AdministratorID: administratorID,
	})
	writeMutation(context, summary, appErr)
}

func (handler *Handler) RestoreOfficialPrice(context *gin.Context) {
	administratorID, ok := currentAdministratorID(context)
	if !ok {
		return
	}
	if !validQuery(context, "expected_version") {
		response.Error(context, requestError())
		return
	}
	var request restoreRequest
	if err := context.ShouldBindQuery(&request); err != nil {
		response.Error(context, requestError())
		return
	}
	summary, appErr := handler.requireService().RestoreOfficialPrice(context.Request.Context(), context.Param("model_id"), request.ExpectedVersion, administratorID)
	writeMutation(context, summary, appErr)
}

func writeMutation(context *gin.Context, summary *officialmodel.MutationSummary, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(context, appErr)
		return
	}
	result, err := officialmodel.MutationResponseFromSummary(summary)
	if err != nil {
		response.Error(context, apperror.Internal("官方模型价格变更结果无效"))
		return
	}
	response.OK(context, result)
}

func (handler *Handler) requireService() officialmodel.HTTPService {
	if handler == nil || handler.service == nil {
		return nilHTTPService{}
	}
	return handler.service
}

func currentAdministratorID(context *gin.Context) (uint64, bool) {
	identity := middleware.GetAuthIdentity(context)
	if identity == nil || identity.UserID <= 0 {
		response.Error(context, apperror.Unauthorized("Token无效或已过期"))
		return 0, false
	}
	return uint64(identity.UserID), true
}

func bindStrictJSON(context *gin.Context, limit int64, target any) bool {
	if context == nil || context.Request == nil || context.Request.Body == nil {
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(context.Writer, context.Request.Body, limit))
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

func validQuery(context *gin.Context, allowed ...string) bool {
	if context == nil || context.Request == nil || context.Request.URL == nil {
		return false
	}
	set := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		set[key] = struct{}{}
	}
	for key, values := range context.Request.URL.Query() {
		if _, ok := set[key]; !ok || len(values) != 1 {
			return false
		}
	}
	return true
}

func requestError() *apperror.Error {
	return apperror.Wrap(officialmodel.ErrorCodeInvalidPriceSync, apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent,
		officialmodel.ErrorCodeInvalidPriceSync, nil, "官方模型请求参数无效", nil)
}

func writeResult(context *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(context, appErr)
		return
	}
	response.OK(context, result)
}

type nilHTTPService struct{}

func (nilHTTPService) PageInit(context.Context) (*officialmodel.PageInitResponse, *apperror.Error) {
	return nil, apperror.Internal("官方模型服务未配置")
}

func (nilHTTPService) List(context.Context, officialmodel.ListQuery) (*officialmodel.ListResponse, *apperror.Error) {
	return nil, apperror.Internal("官方模型服务未配置")
}

func (nilHTTPService) Detail(context.Context, string) (*officialmodel.OfficialModelDTO, *apperror.Error) {
	return nil, apperror.Internal("官方模型服务未配置")
}

func (nilHTTPService) SetPriceOverride(context.Context, string, officialmodel.SetPriceOverrideInput) (*officialmodel.MutationSummary, *apperror.Error) {
	return nil, apperror.Internal("官方模型服务未配置")
}

func (nilHTTPService) RestoreOfficialPrice(context.Context, string, int64, uint64) (*officialmodel.MutationSummary, *apperror.Error) {
	return nil, apperror.Internal("官方模型服务未配置")
}
