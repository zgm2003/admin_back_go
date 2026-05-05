package user

import (
	"context"
	"strconv"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type InitService interface {
	Init(ctx context.Context, input InitInput) (*InitResponse, *apperror.Error)
}

type HTTPService interface {
	InitService
	PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error)
	Profile(ctx context.Context, userID int64, currentUserID int64) (*ProfileResponse, *apperror.Error)
	UpdateProfile(ctx context.Context, input UpdateProfileInput) *apperror.Error
	UpdatePassword(ctx context.Context, input UpdatePasswordInput) *apperror.Error
	UpdateEmail(ctx context.Context, input UpdateEmailInput) *apperror.Error
	UpdatePhone(ctx context.Context, input UpdatePhoneInput) *apperror.Error
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error
	ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error
	Delete(ctx context.Context, ids []int64) *apperror.Error
	BatchUpdateProfile(ctx context.Context, input BatchProfileUpdate) *apperror.Error
}

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Init(c *gin.Context) {
	h.respondWithCurrentUser(c)
}

func (h *Handler) Me(c *gin.Context) {
	h.respondWithCurrentUser(c)
}

func (h *Handler) PageInit(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	result, appErr := h.service.PageInit(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) CurrentProfile(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	h.respondWithProfile(c, identity.UserID, identity.UserID)
}

func (h *Handler) UserProfile(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	h.respondWithProfile(c, id, identity.UserID)
}

func (h *Handler) UpdateCurrentProfile(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return
	}
	if appErr := h.service.UpdateProfile(c.Request.Context(), UpdateProfileInput{
		UserID:        identity.UserID,
		Username:      req.Username,
		Avatar:        req.Avatar,
		Sex:           req.Sex,
		Birthday:      req.Birthday,
		AddressID:     *req.AddressID,
		DetailAddress: req.DetailAddress,
		Bio:           req.Bio,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) UpdatePassword(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	var req updatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return
	}
	if appErr := h.service.UpdatePassword(c.Request.Context(), UpdatePasswordInput{
		UserID:          identity.UserID,
		VerifyType:      req.VerifyType,
		OldPassword:     req.OldPassword,
		Account:         req.Account,
		Code:            req.Code,
		NewPassword:     req.NewPassword,
		ConfirmPassword: req.ConfirmPassword,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) UpdateEmail(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	var req updateEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return
	}
	if appErr := h.service.UpdateEmail(c.Request.Context(), UpdateEmailInput{
		UserID: identity.UserID,
		Email:  req.Email,
		Code:   req.Code,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) UpdatePhone(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	var req updatePhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return
	}
	if appErr := h.service.UpdatePhone(c.Request.Context(), UpdatePhoneInput{
		UserID: identity.UserID,
		Phone:  req.Phone,
		Code:   req.Code,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) List(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequest("列表参数错误"))
		return
	}
	addressIDs, appErr := parseIDList(req.AddressID, "地址参数错误")
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	query := ListQuery{
		CurrentPage:   req.CurrentPage,
		PageSize:      req.PageSize,
		Keyword:       req.Keyword,
		Username:      req.Username,
		Email:         req.Email,
		DetailAddress: req.DetailAddress,
		AddressIDs:    addressIDs,
		Sex:           req.Sex,
		DateRange:     parseDateRange(req.Date, req.DateStart, req.DateEnd),
	}
	if req.RoleID != nil {
		query.RoleID = *req.RoleID
	}
	result, appErr := h.service.List(c.Request.Context(), query)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Update(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return
	}
	if appErr := h.service.Update(c.Request.Context(), id, UpdateInput{
		Username:      req.Username,
		Avatar:        req.Avatar,
		RoleID:        req.RoleID,
		Sex:           req.Sex,
		AddressID:     *req.AddressID,
		DetailAddress: req.DetailAddress,
		Bio:           req.Bio,
	}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("无效的状态"))
		return
	}
	if appErr := h.service.ChangeStatus(c.Request.Context(), id, req.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DeleteOne(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), []int64{id}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) DeleteBatch(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	var req deleteBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("请选择要删除的用户"))
		return
	}
	if appErr := h.service.Delete(c.Request.Context(), req.IDs); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) BatchUpdateProfile(c *gin.Context) {
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	var req batchProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequest("参数错误"))
		return
	}
	input, appErr := buildBatchProfileInput(req)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if appErr := h.service.BatchUpdateProfile(c.Request.Context(), input); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) respondWithCurrentUser(c *gin.Context) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.Unauthorized("Token无效或已过期"))
		return
	}
	if h.service == nil {
		response.Error(c, apperror.Internal("用户初始化服务未配置"))
		return
	}

	result, appErr := h.service.Init(c.Request.Context(), InitInput{
		UserID:   identity.UserID,
		Platform: identity.Platform,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) respondWithProfile(c *gin.Context, userID int64, currentUserID int64) {
	if h.service == nil {
		response.Error(c, apperror.Internal("用户管理服务未配置"))
		return
	}
	result, appErr := h.service.Profile(c.Request.Context(), userID, currentUserID)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequest("无效的用户ID"))
		return 0, false
	}
	return id, true
}

func parseIDList(value string, message string) ([]int64, *apperror.Error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			return nil, apperror.BadRequest(message)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func parseDateRange(date string, start string, end string) []string {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if start != "" || end != "" {
		return []string{start, end}
	}
	date = strings.TrimSpace(date)
	if date == "" {
		return nil
	}
	parts := strings.Split(date, ",")
	if len(parts) < 2 {
		return nil
	}
	return []string{strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])}
}

func buildBatchProfileInput(req batchProfileRequest) (BatchProfileUpdate, *apperror.Error) {
	input := BatchProfileUpdate{
		IDs:   req.IDs,
		Field: req.Field,
	}
	switch req.Field {
	case BatchProfileFieldSex:
		if req.Sex == nil {
			return input, apperror.BadRequest("性别不能为空")
		}
		input.Sex = *req.Sex
	case BatchProfileFieldAddressID:
		if req.AddressID == nil {
			return input, apperror.BadRequest("地址不能为空")
		}
		input.AddressID = *req.AddressID
	case BatchProfileFieldDetailAddress:
		input.DetailAddress = req.DetailAddress
	default:
		return input, apperror.BadRequest("无效的批量修改字段")
	}
	return input, nil
}
