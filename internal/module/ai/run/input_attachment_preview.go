package airun

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"

	"admin_back_go/internal/infra/storage"
	"admin_back_go/internal/shared/apperror"
)

var (
	errInputAttachmentSelectionInvalid = errors.New("input attachment selection is invalid")
	errInputAttachmentEvidenceMissing  = errors.New("input attachment evidence is missing")
)

const inputAttachmentPreviewMaxTTLSeconds int64 = 300

func WithInputAttachmentPreviewer(previewer storage.ImagePreviewer) Option {
	return func(service *Service) {
		if previewer != nil {
			service.attachmentPreviewer = previewer
		}
	}
}

func (s *Service) InputAttachmentPreview(ctx context.Context, id int64, ordinal int64) (*InputAttachmentPreviewResponse, *apperror.Error) {
	if id <= 0 || ordinal <= 0 {
		return nil, invalidInputAttachmentSelection()
	}
	repository, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repository.InputSnapshot(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(
			"airun.input_attachment.query_failed", apperror.CategoryInternal, http.StatusInternalServerError,
			apperror.Permanent, "airun.input_attachment.query_failed", nil, "查询AI运行输入附件失败", err,
		)
	}
	if row == nil {
		return nil, apperror.New(
			"airun.input_attachment.run_not_found", apperror.CategoryNotFound, http.StatusNotFound,
			apperror.Permanent, "airun.input_attachment.run_not_found", nil, "AI运行记录不存在",
		)
	}
	if row.RunID != id {
		return nil, apperror.New(
			"airun.input_attachment.evidence_invalid", apperror.CategoryInternal, http.StatusInternalServerError,
			apperror.Permanent, "airun.input_attachment.evidence_invalid", nil, "AI运行输入附件证据无效",
		)
	}
	input, err := persistedImagePreviewInput(row.InputSnapshot, ordinal)
	if errors.Is(err, errInputAttachmentSelectionInvalid) {
		return nil, invalidInputAttachmentSelection()
	}
	if err != nil {
		return nil, unavailableInputAttachmentPreview(err)
	}
	if s.attachmentPreviewer == nil {
		return nil, apperror.New(
			"airun.input_attachment.previewer_missing", apperror.CategoryInternal, http.StatusInternalServerError,
			apperror.Permanent, "airun.input_attachment.previewer_missing", nil, "AI运行输入附件预览服务未配置",
		)
	}
	preview, err := s.attachmentPreviewer.Preview(ctx, input)
	if err != nil {
		if errors.Is(err, storage.ErrInvalidImagePreviewInput) ||
			errors.Is(err, storage.ErrConditionalObjectUnavailable) ||
			errors.Is(err, storage.ErrConditionalObjectVersionChanged) {
			return nil, unavailableInputAttachmentPreview(err)
		}
		return nil, apperror.Wrap(
			"airun.input_attachment.preview_failed", apperror.CategoryDependency, http.StatusServiceUnavailable,
			apperror.Retryable, "airun.input_attachment.preview_failed", nil, "生成AI运行输入附件预览失败", err,
		)
	}
	preview.URL = strings.TrimSpace(preview.URL)
	if preview.URL == "" || preview.ExpiresIn <= 0 || preview.ExpiresIn > inputAttachmentPreviewMaxTTLSeconds {
		return nil, apperror.New(
			"airun.input_attachment.preview_invalid", apperror.CategoryInternal, http.StatusInternalServerError,
			apperror.Permanent, "airun.input_attachment.preview_invalid", nil, "AI运行输入附件预览结果无效",
		)
	}
	return &InputAttachmentPreviewResponse{URL: preview.URL, ExpiresIn: preview.ExpiresIn}, nil
}

type persistedInputAttachment struct {
	Type      string `json:"type"`
	ObjectKey string `json:"object_key"`
	MIMEType  string `json:"mime_type"`
	Size      int64  `json:"size"`
	ETag      string `json:"etag"`
}

func persistedImagePreviewInput(raw string, ordinal int64) (storage.ImagePreviewInput, error) {
	var snapshot struct {
		Attachments json.RawMessage `json:"attachments"`
		MetaJSON    *string         `json:"meta_json"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &snapshot) != nil {
		return storage.ImagePreviewInput{}, errInputAttachmentEvidenceMissing
	}
	attachmentsJSON := snapshot.Attachments
	if len(attachmentsJSON) > 0 && snapshot.MetaJSON != nil {
		return storage.ImagePreviewInput{}, errInputAttachmentEvidenceMissing
	}
	if snapshot.MetaJSON != nil {
		var metadata struct {
			Attachments json.RawMessage `json:"attachments"`
		}
		if json.Unmarshal([]byte(*snapshot.MetaJSON), &metadata) != nil {
			return storage.ImagePreviewInput{}, errInputAttachmentEvidenceMissing
		}
		attachmentsJSON = metadata.Attachments
	}
	var attachments []persistedInputAttachment
	if len(attachmentsJSON) == 0 || json.Unmarshal(attachmentsJSON, &attachments) != nil || ordinal > int64(len(attachments)) {
		return storage.ImagePreviewInput{}, errInputAttachmentSelectionInvalid
	}
	attachment := attachments[ordinal-1]
	if strings.TrimSpace(attachment.Type) != "image" {
		return storage.ImagePreviewInput{}, errInputAttachmentSelectionInvalid
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(attachment.MIMEType))
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
		return storage.ImagePreviewInput{}, errInputAttachmentEvidenceMissing
	}
	input := storage.ImagePreviewInput{
		StorageProvider: "cos",
		ObjectKey:       strings.TrimSpace(attachment.ObjectKey),
		ETag:            strings.TrimSpace(attachment.ETag),
		Size:            attachment.Size,
		MIMEType:        strings.ToLower(mediaType),
	}
	if input.Validate() != nil {
		return storage.ImagePreviewInput{}, errInputAttachmentEvidenceMissing
	}
	return input, nil
}

func invalidInputAttachmentSelection() *apperror.Error {
	return apperror.New(
		"airun.input_attachment.selection_invalid", apperror.CategoryValidation, http.StatusBadRequest,
		apperror.Permanent, "airun.input_attachment.selection_invalid", nil, "无效的AI运行输入附件",
	)
}

func unavailableInputAttachmentPreview(cause error) *apperror.Error {
	return apperror.Wrap(
		"airun.input_attachment.preview_unavailable", apperror.CategoryConflict, http.StatusConflict,
		apperror.Permanent, "airun.input_attachment.preview_unavailable", nil, "AI运行输入附件预览不可用", cause,
	)
}
