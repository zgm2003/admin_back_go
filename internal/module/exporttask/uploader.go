package exporttask

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	storagecos "admin_back_go/internal/platform/storage/cos"
)

const XLSXContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

type SecretDecrypter interface {
	Decrypt(ciphertext string) (string, error)
}

type UploadInput struct {
	TaskID   int64
	Prefix   string
	Body     []byte
	RowCount int64
}

type UploadResult struct {
	FileName string
	FileURL  string
	FileSize int64
	RowCount int64
}

type COSUploader struct {
	repository UploadConfigRepository
	box        SecretDecrypter
	writer     storagecos.ObjectWriter
	now        func() time.Time
}

type UploadOption func(*COSUploader)

func WithUploadNow(now func() time.Time) UploadOption {
	return func(u *COSUploader) {
		if now != nil {
			u.now = now
		}
	}
}

func NewCOSUploader(repository UploadConfigRepository, box SecretDecrypter, writer storagecos.ObjectWriter, opts ...UploadOption) *COSUploader {
	uploader := &COSUploader{repository: repository, box: box, writer: writer, now: time.Now}
	for _, opt := range opts {
		if opt != nil {
			opt(uploader)
		}
	}
	return uploader
}

func (u *COSUploader) Upload(ctx context.Context, input UploadInput) (*UploadResult, error) {
	if u == nil || u.repository == nil || u.box == nil || u.writer == nil {
		return nil, fmt.Errorf("export upload: uploader is not configured")
	}
	if input.TaskID <= 0 || len(input.Body) == 0 {
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
	now := u.now()
	prefix := strings.TrimSpace(input.Prefix)
	if prefix == "" {
		prefix = "export"
	}
	fileName := fmt.Sprintf("%s_%s_%d.xlsx", prefix, now.Format("20060102_150405"), input.TaskID)
	key := path.Join("exports", now.Format("20060102"), fileName)
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
	return &UploadResult{FileName: fileName, FileURL: objectURL(*cfg, key), FileSize: int64(len(input.Body)), RowCount: input.RowCount}, nil
}

func objectURL(cfg UploadConfig, key string) string {
	base := strings.TrimRight(strings.TrimSpace(cfg.BucketDomain), "/")
	if base == "" {
		base = fmt.Sprintf("https://%s.cos.%s.myqcloud.com", strings.TrimSpace(cfg.Bucket), strings.TrimSpace(cfg.Region))
	}
	return base + "/" + strings.TrimLeft(key, "/")
}
