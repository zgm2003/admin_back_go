package uploadtoken

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/uploadpolicy"
)

const mebibyte = int64(1 << 20)

var (
	errActiveRuleRepositoryMissing = errors.New("active upload rule repository is not configured")
	errActiveRuleMissing           = errors.New("active upload rule is not configured")
	errActiveRuleInvalid           = errors.New("active upload rule is invalid")
)

type activeRuleResolver struct {
	repository Repository
}

func NewActiveRuleResolver(repository Repository) uploadpolicy.Resolver {
	return activeRuleResolver{repository: repository}
}

func (resolver activeRuleResolver) ResolveActive(ctx context.Context) (uploadpolicy.Rule, error) {
	if resolver.repository == nil {
		return uploadpolicy.Rule{}, errActiveRuleRepositoryMissing
	}

	config, err := resolver.repository.GetEnabledConfig(ctx)
	if err != nil {
		return uploadpolicy.Rule{}, fmt.Errorf("resolve active upload rule: %w", err)
	}
	if config == nil {
		return uploadpolicy.Rule{}, errActiveRuleMissing
	}
	if config.MaxSizeMB <= 0 || int64(config.MaxSizeMB) > math.MaxInt64/mebibyte {
		return uploadpolicy.Rule{}, errActiveRuleInvalid
	}

	imageExtensions, err := normalizeActiveRuleExtensions(config.ImageExts, enum.IsUploadImageExt, enum.UploadImageExts)
	if err != nil {
		return uploadpolicy.Rule{}, fmt.Errorf("%w: image extensions", errActiveRuleInvalid)
	}
	fileExtensions, err := normalizeActiveRuleExtensions(config.FileExts, enum.IsUploadFileExt, enum.UploadFileExts)
	if err != nil {
		return uploadpolicy.Rule{}, fmt.Errorf("%w: file extensions", errActiveRuleInvalid)
	}

	return uploadpolicy.Rule{
		MaxFileBytes:    int64(config.MaxSizeMB) * mebibyte,
		ImageExtensions: imageExtensions,
		FileExtensions:  fileExtensions,
	}, nil
}

func normalizeActiveRuleExtensions(raw string, allowed func(string) bool, ordered []string) ([]string, error) {
	var extensions []string
	if err := json.Unmarshal([]byte(raw), &extensions); err != nil {
		return nil, err
	}
	return enum.NormalizeUploadExts(extensions, allowed, ordered)
}
