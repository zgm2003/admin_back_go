package admin

import (
	"context"
	"strconv"

	"admin_back_go/internal/middleware"
	contextengine "admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	CreateProfile(context.Context, uint32, contextengine.CreateProfileInput) (*contextengine.ProfileDTO, *apperror.Error)
	UpdateProfile(context.Context, uint64, contextengine.UpdateProfileInput) (*contextengine.ProfileDTO, *apperror.Error)
	CreateSpace(context.Context, string, uint32, contextengine.CreateSpaceInput) (*contextengine.SpaceDTO, *apperror.Error)
	UpdateSpace(context.Context, string, uint64, contextengine.UpdateSpaceInput) (*contextengine.SpaceDTO, *apperror.Error)
	DeleteSpace(context.Context, string, uint64) *apperror.Error
	CreateDocument(context.Context, string, uint32, contextengine.CreateDocumentInput) (*contextengine.DocumentAdminDTO, *apperror.Error)
	ReindexDocument(context.Context, string, uint64) (*contextengine.DocumentAdminDTO, *apperror.Error)
}

type Handler struct {
	platform string
	service  HTTPService
}

func NewHandler(platform string, service HTTPService) *Handler {
	return &Handler{platform: platform, service: service}
}

func (handler *Handler) CreateProfile(c *gin.Context) {
	var request profileCreateRequest
	if !bind(c, &request) {
		return
	}
	actor, ok := actorID(c)
	if !ok {
		return
	}
	result, appErr := handler.service.CreateProfile(c.Request.Context(), actor, profileCreateInput(request))
	write(c, result, appErr)
}
func (handler *Handler) UpdateProfile(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var request profileUpdateRequest
	if !bind(c, &request) {
		return
	}
	result, appErr := handler.service.UpdateProfile(c.Request.Context(), id, contextengine.UpdateProfileInput{Name: request.Name, Status: request.Status})
	write(c, result, appErr)
}
func (handler *Handler) CreateSpace(c *gin.Context) {
	var request spaceRequest
	if !bind(c, &request) {
		return
	}
	actor, ok := actorID(c)
	if !ok {
		return
	}
	result, appErr := handler.service.CreateSpace(c.Request.Context(), handler.platform, actor, spaceInput(request))
	write(c, result, appErr)
}
func (handler *Handler) UpdateSpace(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var request spaceRequest
	if !bind(c, &request) {
		return
	}
	input := spaceInput(request)
	result, appErr := handler.service.UpdateSpace(c.Request.Context(), handler.platform, id, contextengine.UpdateSpaceInput(input))
	write(c, result, appErr)
}
func (handler *Handler) DeleteSpace(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	write(c, gin.H{}, handler.service.DeleteSpace(c.Request.Context(), handler.platform, id))
}
func (handler *Handler) CreateDocument(c *gin.Context) {
	var request documentRequest
	if !bind(c, &request) {
		return
	}
	actor, ok := actorID(c)
	if !ok {
		return
	}
	result, appErr := handler.service.CreateDocument(c.Request.Context(), handler.platform, actor, documentInput(request))
	write(c, result, appErr)
}
func (handler *Handler) ReindexDocument(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	result, appErr := handler.service.ReindexDocument(c.Request.Context(), handler.platform, id)
	write(c, result, appErr)
}

func bind(c *gin.Context, request any) bool {
	if err := c.ShouldBindJSON(request); err != nil {
		response.Error(c, apperror.BadRequest("上下文管理参数错误"))
		return false
	}
	return true
}
func routeID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperror.BadRequest("无效的上下文资源ID"))
		return 0, false
	}
	return id, true
}
func actorID(c *gin.Context) (uint32, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil {
		response.Error(c, apperror.Unauthorized("缺少管理员身份"))
		return 0, false
	}
	if identity.UserID <= 0 || uint64(identity.UserID) > uint64(^uint32(0)) {
		response.Error(c, apperror.Unauthorized("无效的管理员身份"))
		return 0, false
	}
	return uint32(identity.UserID), true
}
func write(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}
