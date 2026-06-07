package canvas

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	"admin_back_go/internal/middleware"
	aiimagemodule "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

const (
	maxImageEditFiles     = 10
	maxImageEditFileBytes = 20 << 20
	maxImageEditBodyBytes = maxImageEditFiles*maxImageEditFileBytes + 1<<20
)

type Handler struct{ service aiimagemodule.HTTPService }

func NewHandler(service aiimagemodule.HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req listRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.ai.image.list.request.invalid", nil, "图片任务列表参数错误"))
		return
	}
	result, appErr := h.requireService().List(c.Request.Context(), userID, aiimagemodule.ListQuery{
		CurrentPage: req.Page,
		PageSize:    req.PageSize,
		Platform:    enum.PlatformCanvas,
		Status:      req.Status,
	})
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Generations(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req imageGenerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.ai.image.request.invalid", nil, "图片生成参数错误"))
		return
	}
	result, appErr := h.requireService().Create(c.Request.Context(), createInput(userID, req))
	writeCreateResult(c, result, appErr)
}

func (h *Handler) Edits(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	req, uploads, ok := bindImageEditRequest(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().CreateWithUploadedFiles(c.Request.Context(), aiimagemodule.CreateWithUploadedFilesInput{
		CreateInput: createInput(userID, req),
		Files:       uploads,
	})
	writeCreateResult(c, result, appErr)
}

func (h *Handler) Status(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperror.BadRequestKey("canvas.ai.image.id.invalid", nil, "图片任务ID无效"))
		return
	}
	result, appErr := h.requireService().Detail(c.Request.Context(), userID, id, enum.PlatformCanvas)
	writeDetailResult(c, result, appErr)
}

func (h *Handler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperror.BadRequestKey("canvas.ai.image.id.invalid", nil, "图片任务ID无效"))
		return
	}
	if appErr := h.requireService().Delete(c.Request.Context(), userID, id, enum.PlatformCanvas); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, gin.H{})
}

func (h *Handler) requireService() aiimagemodule.HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func createInput(userID uint64, req imageGenerationRequest) aiimagemodule.CreateInput {
	return aiimagemodule.CreateInput{
		UserID:            userID,
		AgentID:           req.AgentID,
		Platform:          enum.PlatformCanvas,
		Prompt:            req.Prompt,
		Size:              req.Size,
		Quality:           req.Quality,
		OutputFormat:      req.OutputFormat,
		OutputCompression: req.OutputCompression,
		Moderation:        req.Moderation,
		N:                 req.N,
	}
}

func currentUserID(c *gin.Context) (uint64, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return 0, false
	}
	if identity.Platform != enum.PlatformCanvas {
		response.Error(c, apperror.UnauthorizedKey("auth.platform.invalid", map[string]any{"platform": identity.Platform}, "Token平台不匹配"))
		return 0, false
	}
	return uint64(identity.UserID), true
}

func writeCreateResult(c *gin.Context, result *aiimagemodule.CreateTaskResponse, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if result == nil || result.Task.ID == 0 {
		response.Error(c, apperror.InternalKey("canvas.ai.image.result_invalid", nil, "Canvas图片生成结果无效"))
		return
	}
	response.OK(c, imageGenerationResponse{TaskID: result.Task.ID, Status: result.Task.Status})
}

func writeDetailResult(c *gin.Context, result *aiimagemodule.DetailResponse, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if result == nil || result.Task.ID == 0 {
		response.Error(c, apperror.InternalKey("canvas.ai.image.result_invalid", nil, "Canvas图片生成结果无效"))
		return
	}
	response.OK(c, result)
}

func bindImageEditRequest(c *gin.Context) (imageGenerationRequest, []aiimagemodule.UploadedFileInput, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImageEditBodyBytes)
	if err := c.Request.ParseMultipartForm(maxImageEditFileBytes); err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.ai.image.request.invalid", nil, "图片生成参数错误"))
		return imageGenerationRequest{}, nil, false
	}
	var req imageGenerationRequest
	if err := c.ShouldBind(&req); err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.ai.image.request.invalid", nil, "图片生成参数错误"))
		return imageGenerationRequest{}, nil, false
	}
	form := c.Request.MultipartForm
	if form == nil || len(form.File["image"]) == 0 {
		response.Error(c, apperror.BadRequestKey("canvas.ai.image.request.invalid", nil, "图片生成参数错误"))
		return imageGenerationRequest{}, nil, false
	}
	files := form.File["image"]
	if len(files) > maxImageEditFiles {
		response.Error(c, apperror.BadRequestKey("canvas.ai.image.request.invalid", nil, "图片生成参数错误"))
		return imageGenerationRequest{}, nil, false
	}
	uploads := make([]aiimagemodule.UploadedFileInput, 0, len(files))
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			response.Error(c, apperror.BadRequestKey("canvas.ai.image.request.invalid", nil, "图片生成参数错误"))
			return imageGenerationRequest{}, nil, false
		}
		body, readErr := io.ReadAll(io.LimitReader(file, maxImageEditFileBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(body) == 0 || len(body) > maxImageEditFileBytes {
			response.Error(c, apperror.BadRequestKey("canvas.ai.image.request.invalid", nil, "图片生成参数错误"))
			return imageGenerationRequest{}, nil, false
		}
		mimeType := strings.TrimSpace(header.Header.Get("Content-Type"))
		if mimeType == "" {
			mimeType = http.DetectContentType(body)
		}
		uploads = append(uploads, aiimagemodule.UploadedFileInput{FileName: header.Filename, MimeType: mimeType, Body: body})
	}
	return req, uploads, true
}

type nilHTTPService struct{}

func (nilHTTPService) PageInit(ctx context.Context) (*aiimagemodule.PageInitResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) List(ctx context.Context, userID uint64, query aiimagemodule.ListQuery) (*aiimagemodule.ListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) Detail(ctx context.Context, userID uint64, taskID uint64, platform string) (*aiimagemodule.DetailResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) Create(ctx context.Context, input aiimagemodule.CreateInput) (*aiimagemodule.CreateTaskResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) CreateWithUploadedFiles(ctx context.Context, input aiimagemodule.CreateWithUploadedFilesInput) (*aiimagemodule.CreateTaskResponse, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) Favorite(ctx context.Context, input aiimagemodule.FavoriteInput) (*aiimagemodule.TaskDTO, *apperror.Error) {
	return nil, apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
func (nilHTTPService) Delete(ctx context.Context, userID uint64, taskID uint64, platform string) *apperror.Error {
	return apperror.InternalKey("aiimage.service_missing", nil, "AI图片服务未配置")
}
