package uploadtoken

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/secretbox"
	storagecos "admin_back_go/internal/platform/storage/cos"
)

const (
	ProviderCOS   = "cos"
	FileKindImage = "image"
	FileKindFile  = "file"
)

type Service struct {
	repo        Repository
	box         secretbox.Box
	signer      storagecos.CredentialSigner
	ttl         time.Duration
	randomBytes int
	now         func() time.Time
	random      func([]byte) (int, error)
}

type Options struct {
	TTL         time.Duration
	RandomBytes int
	Now         func() time.Time
	Random      func([]byte) (int, error)
}

func NewService(repo Repository, box secretbox.Box, signer storagecos.CredentialSigner, opts Options) *Service {
	if signer == nil {
		signer = storagecos.DisabledSigner{}
	}
	if opts.TTL <= 0 {
		opts.TTL = 15 * time.Minute
	}
	if opts.RandomBytes <= 0 {
		opts.RandomBytes = 4
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Random == nil {
		opts.Random = rand.Read
	}
	return &Service{repo: repo, box: box, signer: signer, ttl: opts.TTL, randomBytes: opts.RandomBytes, now: opts.Now, random: opts.Random}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*CreateResponse, *apperror.Error) {
	input = normalizeInput(input)
	if !enum.IsUploadFolder(input.Folder) {
		return nil, apperror.BadRequest("上传目录不支持")
	}
	if input.FileName == "" {
		return nil, apperror.BadRequest("文件名不能为空")
	}
	if input.FileSize <= 0 {
		return nil, apperror.BadRequest("文件大小不正确")
	}
	if input.FileKind != FileKindImage && input.FileKind != FileKindFile {
		return nil, apperror.BadRequest("上传类型不支持")
	}
	if s == nil || s.repo == nil {
		return nil, apperror.Internal(ErrRepositoryNotConfiguredMessage)
	}

	cfg, err := s.repo.GetEnabledConfig(ctx)
	if err != nil {
		return nil, apperror.Internal("读取上传配置失败")
	}
	if cfg == nil {
		return nil, apperror.BadRequest("未配置有效上传设置")
	}
	if cfg.Driver != enum.UploadDriverCOS {
		return nil, apperror.BadRequest("当前上传驱动未启用 COS runtime")
	}

	imageExts, fileExts, appErr := parseRuleExts(cfg)
	if appErr != nil {
		return nil, appErr
	}
	if appErr := validateFile(input, cfg.MaxSizeMB, imageExts, fileExts); appErr != nil {
		return nil, appErr
	}

	secretID, err := s.box.Decrypt(cfg.SecretIDEnc)
	if err != nil || secretID == "" {
		return nil, apperror.Internal("上传密钥不可用")
	}
	secretKey, err := s.box.Decrypt(cfg.SecretKeyEnc)
	if err != nil || secretKey == "" {
		return nil, apperror.Internal("上传密钥不可用")
	}

	key, err := s.buildKey(input.Folder, input.FileName)
	if err != nil {
		return nil, apperror.Internal("生成上传路径失败")
	}
	creds, err := s.signer.Sign(ctx, storagecos.SignInput{
		SecretID: secretID, SecretKey: secretKey, Bucket: cfg.Bucket, Region: cfg.Region, AppID: cfg.AppID, Key: key, TTL: s.ttl,
	})
	if errors.Is(err, storagecos.ErrDisabled) {
		return nil, apperror.Internal("COS 临时凭证未启用")
	}
	if err != nil {
		return nil, apperror.Internal("COS 临时凭证签发失败")
	}
	if creds == nil {
		return nil, apperror.Internal("COS 临时凭证签发失败")
	}

	return &CreateResponse{
		Provider:     ProviderCOS,
		Bucket:       cfg.Bucket,
		Region:       cfg.Region,
		Key:          key,
		UploadPath:   uploadPath(input.Folder, s.now()),
		BucketDomain: optionalString(cfg.BucketDomain),
		Credentials:  CredentialsDTO{TmpSecretID: creds.TmpSecretID, TmpSecretKey: creds.TmpSecretKey, SessionToken: creds.SessionToken},
		StartTime:    creds.StartTime,
		ExpiredTime:  creds.ExpiredTime,
		Rule:         UploadRuleDTO{MaxSizeMB: cfg.MaxSizeMB, ImageExts: imageExts, FileExts: fileExts},
	}, nil
}

func normalizeInput(input CreateInput) CreateInput {
	input.Folder = strings.Trim(strings.TrimSpace(input.Folder), "/")
	input.FileName = filepath.Base(strings.TrimSpace(input.FileName))
	input.FileKind = strings.TrimSpace(input.FileKind)
	return input
}

func parseRuleExts(cfg *EnabledConfig) ([]string, []string, *apperror.Error) {
	var imageExts []string
	var fileExts []string
	if err := json.Unmarshal([]byte(cfg.ImageExts), &imageExts); err != nil {
		return nil, nil, apperror.BadRequest("上传配置不完整")
	}
	if err := json.Unmarshal([]byte(cfg.FileExts), &fileExts); err != nil {
		return nil, nil, apperror.BadRequest("上传配置不完整")
	}
	return imageExts, fileExts, nil
}

func validateFile(input CreateInput, maxSizeMB int, imageExts []string, fileExts []string) *apperror.Error {
	if maxSizeMB > 0 && input.FileSize > int64(maxSizeMB)*1024*1024 {
		return apperror.BadRequest("文件大小超过限制")
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(input.FileName)), ".")
	if ext == "" {
		return apperror.BadRequest("文件类型不支持")
	}
	allowed := fileExts
	if input.FileKind == FileKindImage {
		allowed = imageExts
	}
	for _, item := range allowed {
		if strings.EqualFold(item, ext) {
			return nil
		}
	}
	return apperror.BadRequest("文件类型不支持")
}

func (s *Service) buildKey(folder string, fileName string) (string, error) {
	now := s.now()
	randomPart, err := s.randomHex()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d-%s-%s", uploadPath(folder, now), now.UnixMilli(), randomPart, safeFileName(fileName)), nil
}

func (s *Service) randomHex() (string, error) {
	buf := make([]byte, s.randomBytes)
	if _, err := s.random(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func uploadPath(folder string, now time.Time) string {
	return fmt.Sprintf("%s/%04d/%02d/%02d/", folder, now.Year(), now.Month(), now.Day())
}

func safeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "file"
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := b.String()
	if out == "" {
		out = "file"
	}
	if len(out) > 120 {
		out = out[len(out)-120:]
	}
	return out
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
