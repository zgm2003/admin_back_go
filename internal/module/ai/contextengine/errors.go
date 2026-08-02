package contextengine

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"admin_back_go/internal/shared/apperror"
)

var (
	ErrInvalidContextValue         = errors.New("invalid context value")
	ErrInvalidContextPlan          = errors.New("invalid context plan")
	ErrInvalidBudget               = errors.New("invalid context budget")
	ErrInvalidFixedScore           = errors.New("invalid fixed score")
	ErrInvalidProfileIndex         = errors.New("invalid context profile index")
	ErrInvalidSHA256               = errors.New("invalid sha256")
	ErrNilPlanCommitGuard          = errors.New("context plan commit guard is nil")
	ErrInvalidPlanCommitToken      = errors.New("invalid context plan commit token")
	ErrPlanCommitAborted           = errors.New("context plan commit aborted by run state or lease")
	ErrPlanRepositoryNotConfigured = errors.New("context plan repository not configured")
)

type ErrorCode string

const (
	ErrCodeProfileUnavailable       ErrorCode = "ai.context.profile_unavailable"
	ErrCodeDocumentParseFailed      ErrorCode = "ai.context.document_parse_failed"
	ErrCodeEmbeddingFailed          ErrorCode = "ai.context.embedding_failed"
	ErrCodeIndexFailed              ErrorCode = "ai.context.index_failed"
	ErrCodeIndexInconsistent        ErrorCode = "ai.context.index_inconsistent"
	ErrCodeSnapshotConflict         ErrorCode = "ai.context.snapshot_conflict"
	ErrCodePermissionDenied         ErrorCode = "ai.context.permission_denied"
	ErrCodeRetrievalFailed          ErrorCode = "ai.context.retrieval_failed"
	ErrCodeRerankFailed             ErrorCode = "ai.context.rerank_failed"
	ErrCodeRequiredOverflow         ErrorCode = "ai.context.required_overflow"
	ErrCodeToolContinuationOverflow ErrorCode = "ai.context.tool_continuation_overflow"
	ErrCodeAttachmentUnavailable    ErrorCode = "ai.context.attachment_unavailable"
	ErrCodeMemoryUnavailable        ErrorCode = "ai.context.memory_unavailable"
	ErrCodePlanConflict             ErrorCode = "ai.context.plan_conflict"
)

func (code ErrorCode) Validate() error {
	switch code {
	case ErrCodeProfileUnavailable,
		ErrCodeDocumentParseFailed,
		ErrCodeEmbeddingFailed,
		ErrCodeIndexFailed,
		ErrCodeIndexInconsistent,
		ErrCodeSnapshotConflict,
		ErrCodePermissionDenied,
		ErrCodeRetrievalFailed,
		ErrCodeRerankFailed,
		ErrCodeRequiredOverflow,
		ErrCodeToolContinuationOverflow,
		ErrCodeAttachmentUnavailable,
		ErrCodeMemoryUnavailable,
		ErrCodePlanConflict:
		return nil
	}
	return fmt.Errorf("%w: error code %q", ErrInvalidContextValue, code)
}

type contextErrorDefinition struct {
	category   apperror.Category
	httpStatus int
	retry      apperror.RetryClass
	message    string
}

func NewPlanError(stage string, code ErrorCode) (PlanError, error) {
	definition, err := contextErrorDefinitionFor(code)
	if err != nil {
		return PlanError{}, err
	}
	message := definition.message
	planError := PlanError{Stage: strings.TrimSpace(stage), Code: code, Message: &message}
	if err := planError.Validate(); err != nil {
		return PlanError{}, err
	}
	return planError, nil
}

func NewContextAppError(code ErrorCode, cause error) (*apperror.Error, error) {
	definition, err := contextErrorDefinitionFor(code)
	if err != nil {
		return nil, err
	}
	if cause == nil {
		return apperror.New(
			string(code), definition.category, definition.httpStatus, definition.retry,
			string(code), nil, definition.message,
		), nil
	}
	return apperror.Wrap(
		string(code), definition.category, definition.httpStatus, definition.retry,
		string(code), nil, definition.message, cause,
	), nil
}

func contextErrorDefinitionFor(code ErrorCode) (contextErrorDefinition, error) {
	switch code {
	case ErrCodePermissionDenied:
		return contextErrorDefinition{apperror.CategoryAuthorization, http.StatusForbidden, apperror.Permanent, "上下文来源权限已变化"}, nil
	case ErrCodeSnapshotConflict, ErrCodePlanConflict:
		return contextErrorDefinition{apperror.CategoryConflict, http.StatusConflict, apperror.Permanent, "上下文快照已变化，请重新发起请求"}, nil
	case ErrCodeRequiredOverflow, ErrCodeToolContinuationOverflow:
		return contextErrorDefinition{apperror.CategoryValidation, http.StatusBadRequest, apperror.Permanent, "上下文超过模型可用窗口"}, nil
	case ErrCodeAttachmentUnavailable:
		return contextErrorDefinition{apperror.CategoryValidation, http.StatusUnprocessableEntity, apperror.Permanent, "历史附件当前不可用"}, nil
	case ErrCodeDocumentParseFailed:
		return contextErrorDefinition{apperror.CategoryValidation, http.StatusUnprocessableEntity, apperror.Permanent, "文档内容无法解析"}, nil
	case ErrCodeProfileUnavailable:
		return contextErrorDefinition{apperror.CategoryDependency, http.StatusServiceUnavailable, apperror.Permanent, "上下文配置当前不可用"}, nil
	case ErrCodeEmbeddingFailed, ErrCodeIndexFailed, ErrCodeRetrievalFailed, ErrCodeRerankFailed, ErrCodeMemoryUnavailable:
		return contextErrorDefinition{apperror.CategoryDependency, http.StatusServiceUnavailable, apperror.Retryable, "上下文服务暂时不可用"}, nil
	case ErrCodeIndexInconsistent:
		return contextErrorDefinition{apperror.CategoryDependency, http.StatusServiceUnavailable, apperror.Permanent, "上下文索引状态不一致"}, nil
	}
	return contextErrorDefinition{}, fmt.Errorf("%w: error code %q", ErrInvalidContextValue, code)
}
