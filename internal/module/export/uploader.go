package exporttask

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/shared/enum"
)

const XLSXContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

type SecretDecrypter interface {
	Decrypt(ciphertext string) (string, error)
}

type UploadInput struct {
	TaskID          int64
	ArtifactVersion string
	CreatedAt       time.Time
	Body            []byte
	RowCount        int64
}

type UploadResult struct {
	FileName  string
	FileURL   string
	ObjectKey string
	FileSize  int64
	RowCount  int64
}

type COSUploader struct {
	repository UploadConfigRepository
	box        SecretDecrypter
	writer     storagecos.ObjectWriter
}

func NewCOSUploader(repository UploadConfigRepository, box SecretDecrypter, writer storagecos.ObjectWriter) *COSUploader {
	return &COSUploader{repository: repository, box: box, writer: writer}
}

func (u *COSUploader) Upload(ctx context.Context, input UploadInput) (*UploadResult, error) {
	if u == nil || u.repository == nil || u.box == nil || u.writer == nil {
		return nil, fmt.Errorf("export upload: uploader is not configured")
	}
	version := strings.TrimSpace(input.ArtifactVersion)
	if input.TaskID <= 0 || len(input.Body) == 0 || input.CreatedAt.IsZero() || version == "" || safePathSegment(version) != version {
		return nil, fmt.Errorf("export upload: invalid upload input")
	}
	cfg, err := u.repository.GetEnabledConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("export upload: load upload config: %w", err)
	}
	if cfg == nil {
		return nil, ErrUploadConfigNotConfigured
	}
	if strings.TrimSpace(cfg.Driver) != enum.UploadDriverCOS {
		return nil, fmt.Errorf("export upload only supports cos driver: %s", cfg.Driver)
	}
	secretID, err := u.box.Decrypt(cfg.SecretIDEnc)
	if err != nil || strings.TrimSpace(secretID) == "" {
		return nil, fmt.Errorf("export upload: decrypt cos secret id: %w", err)
	}
	secretKey, err := u.box.Decrypt(cfg.SecretKeyEnc)
	if err != nil || strings.TrimSpace(secretKey) == "" {
		return nil, fmt.Errorf("export upload: decrypt cos secret key: %w", err)
	}
	fileName := fmt.Sprintf("%d-%s.xlsx", input.TaskID, version)
	key := path.Join("exports", input.CreatedAt.Format("20060102"), fileName)
	if err := u.writer.Put(ctx, storagecos.PutInput{
		SecretID:    secretID,
		SecretKey:   secretKey,
		Bucket:      cfg.Bucket,
		Region:      cfg.Region,
		Endpoint:    cfg.Endpoint,
		Key:         key,
		Body:        input.Body,
		ContentType: XLSXContentType,
	}); err != nil {
		return nil, fmt.Errorf("export upload: put cos object: %w", err)
	}
	return &UploadResult{FileName: fileName, FileURL: objectURL(*cfg, key), ObjectKey: key, FileSize: int64(len(input.Body)), RowCount: input.RowCount}, nil
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "export"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", "..", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return replacer.Replace(value)
}

func objectURL(cfg UploadConfig, key string) string {
	base := strings.TrimRight(strings.TrimSpace(cfg.BucketDomain), "/")
	if base == "" {
		base = fmt.Sprintf("https://%s.cos.%s.myqcloud.com", strings.TrimSpace(cfg.Bucket), strings.TrimSpace(cfg.Region))
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + strings.TrimLeft(base, "/")
	}
	return base + "/" + strings.TrimLeft(key, "/")
}
