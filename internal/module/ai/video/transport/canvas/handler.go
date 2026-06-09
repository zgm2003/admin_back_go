package canvas

import (
	"context"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/ai/internal/canvasrequest"
	aivideomodule "admin_back_go/internal/module/ai/video"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

const maxReferenceMediaUploadBytes = 100 << 20

type Handler struct{ service aivideomodule.HTTPService }

func NewHandler(service aivideomodule.HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) VideoGenerations(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req videoGenerationRequest
	if !canvasrequest.BindAgentOwnedJSONOrForm(c, &req, "canvas.ai.video.request.invalid", "视频生成参数错误") {
		return
	}
	result, appErr := h.requireService().Create(c.Request.Context(), aivideomodule.CreateInput{
		UserID: userID, AgentID: req.AgentID, Prompt: req.Prompt,
		DurationSeconds: req.DurationSeconds, Size: req.Size, ResolutionName: req.ResolutionName,
		GenerateAudio: req.GenerateAudio, Watermark: req.Watermark,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) ReferenceMediaUpload(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req referenceMediaUploadRequest
	if !canvasrequest.BindAgentOwnedJSONOrForm(c, &req, "canvas.ai.video.reference_media.request.invalid", "参考媒体上传参数错误") {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.ai.video.reference_media.file.required", nil, "参考媒体文件不能为空"))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.ai.video.reference_media.file.invalid", nil, "参考媒体文件无效"))
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxReferenceMediaUploadBytes+1))
	if err != nil {
		response.Error(c, apperror.BadRequestKey("canvas.ai.video.reference_media.file.invalid", nil, "参考媒体文件无效"))
		return
	}
	if len(body) > maxReferenceMediaUploadBytes {
		response.Error(c, apperror.BadRequestKey("canvas.ai.video.reference_media.too_large", nil, "参考媒体文件过大"))
		return
	}
	mimeType := referenceUploadMimeType(fileHeader.Header.Get("Content-Type"), fileHeader.Filename)
	result, appErr := h.requireService().UploadReferenceMedia(c.Request.Context(), aivideomodule.ReferenceMediaUploadInput{
		UserID: userID, MediaKind: req.MediaKind, FileName: fileHeader.Filename, MimeType: mimeType, Body: body,
	})
	writeResult(c, result, appErr)
}

func (h *Handler) VideoStatus(c *gin.Context) {
	userID, id, ok := currentUserIDAndRouteID(c)
	if !ok {
		return
	}
	result, appErr := h.requireService().Status(c.Request.Context(), userID, id)
	writeResult(c, result, appErr)
}

func (h *Handler) VideoContent(c *gin.Context) {
	userID, id, ok := currentUserIDAndRouteID(c)
	if !ok {
		return
	}
	body, contentType, appErr := h.requireService().Content(c.Request.Context(), userID, id)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	c.Data(http.StatusOK, contentType, body)
}

func (h *Handler) requireService() aivideomodule.HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

func currentUserID(c *gin.Context) (int64, bool) {
	identity := middleware.GetAuthIdentity(c)
	if identity == nil || identity.UserID <= 0 {
		response.Error(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
		return 0, false
	}
	if identity.Platform != enum.PlatformCanvas {
		response.Error(c, apperror.UnauthorizedKey("auth.platform.invalid", map[string]any{"platform": identity.Platform}, "Token平台不匹配"))
		return 0, false
	}
	return identity.UserID, true
}

func currentUserIDAndRouteID(c *gin.Context) (int64, int64, bool) {
	userID, ok := currentUserID(c)
	if !ok {
		return 0, 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequestKey("canvas.ai.video.id.invalid", nil, "视频任务ID无效"))
		return 0, 0, false
	}
	return userID, id, true
}

func writeResult(c *gin.Context, result any, appErr *apperror.Error) {
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func referenceUploadMimeType(raw string, fileName string) string {
	mimeType := strings.TrimSpace(raw)
	if mimeType == "" || strings.EqualFold(mimeType, "application/octet-stream") {
		if ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName))); ext != "" {
			if byExt := mime.TypeByExtension(ext); strings.TrimSpace(byExt) != "" {
				mimeType = strings.TrimSpace(byExt)
			}
		}
	}
	return strings.TrimSpace(mimeType)
}

type nilHTTPService struct{}

func (nilHTTPService) Create(ctx context.Context, input aivideomodule.CreateInput) (*aivideomodule.CreateResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.ai.video.service_missing", nil, "Canvas视频生成服务未配置")
}

func (nilHTTPService) Status(ctx context.Context, userID int64, id int64) (*aivideomodule.StatusResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.ai.video.service_missing", nil, "Canvas视频生成服务未配置")
}

func (nilHTTPService) Content(ctx context.Context, userID int64, id int64) ([]byte, string, *apperror.Error) {
	return nil, "", apperror.InternalKey("canvas.ai.video.service_missing", nil, "Canvas视频生成服务未配置")
}

func (nilHTTPService) UploadReferenceMedia(ctx context.Context, input aivideomodule.ReferenceMediaUploadInput) (*aivideomodule.ReferenceMediaUploadResponse, *apperror.Error) {
	return nil, apperror.InternalKey("canvas.ai.video.service_missing", nil, "Canvas视频生成服务未配置")
}
