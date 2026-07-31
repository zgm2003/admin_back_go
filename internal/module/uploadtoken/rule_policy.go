package uploadtoken

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/uploadpolicy"

	"gorm.io/gorm"
)

const mebibyte = int64(1 << 20)

var (
	errActiveRuleRepositoryMissing = errors.New("active upload rule repository is not configured")
	errActiveRuleMissing           = errors.New("active upload rule is not configured")
	errActiveRuleInvalid           = errors.New("active upload rule is invalid")
	errActiveRuleGuardUnavailable  = errors.New("active upload rule transaction guard is not configured")
)

type ActiveRuleResolver struct {
	repository Repository
}

type activeRuleGuardRepository interface {
	GetEnabledConfigForUpdate(context.Context, *gorm.DB) (*EnabledConfig, error)
}

func NewActiveRuleResolver(repository Repository) *ActiveRuleResolver {
	return &ActiveRuleResolver{repository: repository}
}

func (resolver *ActiveRuleResolver) ResolveActive(ctx context.Context) (uploadpolicy.Rule, error) {
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
	return activeRuleFromConfig(config)
}

func (resolver *ActiveRuleResolver) GuardActiveInTransaction(ctx context.Context, tx *gorm.DB, token uploadpolicy.ConsistencyToken) error {
	if token == (uploadpolicy.ConsistencyToken{}) {
		return uploadpolicy.ErrRuleSnapshotChanged
	}
	repository, ok := resolver.repository.(activeRuleGuardRepository)
	if !ok || tx == nil {
		return errActiveRuleGuardUnavailable
	}
	config, err := repository.GetEnabledConfigForUpdate(ctx, tx)
	if err != nil {
		return fmt.Errorf("guard active upload rule: %w", err)
	}
	if config == nil {
		return uploadpolicy.ErrRuleSnapshotChanged
	}
	rule, err := activeRuleFromConfig(config)
	if err != nil {
		return fmt.Errorf("%w: %v", uploadpolicy.ErrRuleSnapshotChanged, err)
	}
	if subtle.ConstantTimeCompare(rule.ConsistencyToken[:], token[:]) != 1 {
		return uploadpolicy.ErrRuleSnapshotChanged
	}
	return nil
}

func activeRuleFromConfig(config *EnabledConfig) (uploadpolicy.Rule, error) {
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

	rule := uploadpolicy.Rule{
		MaxFileBytes:    int64(config.MaxSizeMB) * mebibyte,
		ImageExtensions: imageExtensions,
		FileExtensions:  fileExtensions,
	}
	token, err := activeRuleConsistencyToken(config, rule)
	if err != nil {
		return uploadpolicy.Rule{}, fmt.Errorf("%w: consistency token", errActiveRuleInvalid)
	}
	rule.ConsistencyToken = token
	return rule, nil
}

func activeRuleConsistencyToken(config *EnabledConfig, rule uploadpolicy.Rule) (uploadpolicy.ConsistencyToken, error) {
	raw, err := json.Marshal(struct {
		SettingID       int64    `json:"setting_id"`
		DriverID        int64    `json:"driver_id"`
		RuleID          int64    `json:"rule_id"`
		Driver          string   `json:"driver"`
		Bucket          string   `json:"bucket"`
		Region          string   `json:"region"`
		AppID           string   `json:"appid"`
		Endpoint        string   `json:"endpoint"`
		BucketDomain    string   `json:"bucket_domain"`
		RoleARN         string   `json:"role_arn"`
		MaxFileBytes    int64    `json:"max_file_bytes"`
		ImageExtensions []string `json:"image_extensions"`
		FileExtensions  []string `json:"file_extensions"`
	}{
		SettingID: config.SettingID, DriverID: config.DriverID, RuleID: config.RuleID,
		Driver: strings.TrimSpace(config.Driver), Bucket: strings.TrimSpace(config.Bucket), Region: strings.TrimSpace(config.Region),
		AppID: strings.TrimSpace(config.AppID), Endpoint: strings.TrimSpace(config.Endpoint), BucketDomain: strings.TrimSpace(config.BucketDomain),
		RoleARN: strings.TrimSpace(config.RoleARN), MaxFileBytes: rule.MaxFileBytes,
		ImageExtensions: rule.ImageExtensions, FileExtensions: rule.FileExtensions,
	})
	if err != nil {
		return uploadpolicy.ConsistencyToken{}, err
	}
	return uploadpolicy.ConsistencyToken(sha256.Sum256(raw)), nil
}

func normalizeActiveRuleExtensions(raw string, allowed func(string) bool, ordered []string) ([]string, error) {
	var extensions []string
	if err := json.Unmarshal([]byte(raw), &extensions); err != nil {
		return nil, err
	}
	return enum.NormalizeUploadExts(extensions, allowed, ordered)
}
