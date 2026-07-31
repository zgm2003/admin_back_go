package cos

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	tencentcos "github.com/tencentyun/cos-go-sdk-v5"
)

const (
	trustedAIChatImagePrefix      = "ai_chat_images/"
	trustedAIChatAttachmentPrefix = "ai_chat_attachments/"
)

var (
	ErrObjectInspectorNotConfigured = errors.New("cos object inspector is not configured")
	ErrUntrustedObjectKey           = errors.New("cos object key is outside the trusted AI chat attachment namespace")
	ErrInvalidObjectMetadata        = errors.New("cos object metadata is invalid")
	ErrInvalidGIF                   = errors.New("cos GIF object is invalid")
	ErrAnimatedGIF                  = errors.New("animated GIF is not supported")
)

type ObjectConfig struct {
	SecretID     string
	SecretKey    string
	SessionToken string
	Bucket       string
	Region       string
	Endpoint     string
	BucketDomain string
}

type ObjectConfigProvider interface {
	ActiveObjectConfig(context.Context) (ObjectConfig, error)
}

type ObjectMetadata struct {
	Key               string
	MIMEType          string
	Size              int64
	ETag              string
	TrustedURL        string
	GIFStaticVerified bool
}

type ObjectInspector interface {
	Head(context.Context, string) (ObjectMetadata, error)
}

type ObjectInspectorConfig struct {
	Enabled    bool
	Timeout    time.Duration
	HTTPClient *http.Client
}

type COSObjectInspector struct {
	enabled    bool
	timeout    time.Duration
	httpClient *http.Client
	provider   ObjectConfigProvider
}

func NewObjectInspector(provider ObjectConfigProvider, config ObjectInspectorConfig) *COSObjectInspector {
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{}
	}
	return &COSObjectInspector{
		enabled: config.Enabled, timeout: config.Timeout, httpClient: config.HTTPClient, provider: provider,
	}
}

func (inspector *COSObjectInspector) Head(ctx context.Context, key string) (ObjectMetadata, error) {
	if inspector == nil || !inspector.enabled {
		return ObjectMetadata{}, ErrDisabled
	}
	key, err := normalizeTrustedAIChatObjectKey(key)
	if err != nil {
		return ObjectMetadata{}, err
	}
	if inspector.provider == nil {
		return ObjectMetadata{}, ErrObjectInspectorNotConfigured
	}
	config, err := inspector.provider.ActiveObjectConfig(ctx)
	if err != nil {
		return ObjectMetadata{}, fmt.Errorf("resolve active COS object config: %w", err)
	}
	config = normalizeObjectConfig(config)
	if config.SecretID == "" || config.SecretKey == "" || config.Bucket == "" || config.Region == "" {
		return ObjectMetadata{}, ErrInvalidConfig
	}
	bucket, err := bucketURL(PutInput{Bucket: config.Bucket, Region: config.Region, Endpoint: config.Endpoint})
	if err != nil {
		return ObjectMetadata{}, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, inspector.timeout)
	defer cancel()
	client := tencentcos.NewClient(&tencentcos.BaseURL{BucketURL: bucket}, signedHTTPClient(inspector.httpClient, PutInput{
		SecretID: config.SecretID, SecretKey: config.SecretKey, SessionToken: config.SessionToken,
	}))
	response, err := client.Object.Head(reqCtx, key, nil)
	if err != nil {
		return ObjectMetadata{}, fmt.Errorf("cos object head: %w", err)
	}
	if response == nil || response.Response == nil {
		return ObjectMetadata{}, ErrInvalidObjectMetadata
	}
	mimeType, _, err := mime.ParseMediaType(strings.TrimSpace(response.Header.Get("Content-Type")))
	if err != nil || strings.TrimSpace(mimeType) == "" {
		return ObjectMetadata{}, ErrInvalidObjectMetadata
	}
	size := response.ContentLength
	if header := strings.TrimSpace(response.Header.Get("Content-Length")); header != "" {
		parsed, parseErr := strconv.ParseInt(header, 10, 64)
		if parseErr != nil {
			return ObjectMetadata{}, ErrInvalidObjectMetadata
		}
		size = parsed
	}
	if size <= 0 {
		return ObjectMetadata{}, ErrInvalidObjectMetadata
	}
	etag := strings.TrimSpace(response.Header.Get("ETag"))
	if etag == "" {
		return ObjectMetadata{}, ErrInvalidObjectMetadata
	}
	gifStaticVerified := false
	if strings.EqualFold(mimeType, "image/gif") {
		if err := verifyStaticGIFObject(reqCtx, client, key, etag, size); err != nil {
			return ObjectMetadata{}, err
		}
		gifStaticVerified = true
	}
	trustedURL, err := trustedObjectURL(config, key)
	if err != nil {
		return ObjectMetadata{}, err
	}
	return ObjectMetadata{
		Key: key, MIMEType: strings.ToLower(mimeType), Size: size, ETag: etag,
		TrustedURL: trustedURL, GIFStaticVerified: gifStaticVerified,
	}, nil
}

func TrustedAIChatObjectKey(key, attachmentType string) (string, error) {
	clean, err := normalizeTrustedAIChatObjectKey(key)
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(attachmentType) {
	case "image":
		return clean, nil
	case "file":
		if strings.HasPrefix(clean, trustedAIChatAttachmentPrefix) {
			return clean, nil
		}
	}
	return "", ErrUntrustedObjectKey
}

func normalizeTrustedAIChatObjectKey(key string) (string, error) {
	trimmed := strings.TrimSpace(key)
	if key != trimmed || trimmed == "" || strings.Contains(trimmed, "\\") || path.Clean(trimmed) != trimmed ||
		(!strings.HasPrefix(trimmed, trustedAIChatImagePrefix) && !strings.HasPrefix(trimmed, trustedAIChatAttachmentPrefix)) ||
		trimmed == trustedAIChatImagePrefix || trimmed == trustedAIChatAttachmentPrefix {
		return "", ErrUntrustedObjectKey
	}
	return trimmed, nil
}

func normalizeObjectConfig(config ObjectConfig) ObjectConfig {
	config.SecretID = strings.TrimSpace(config.SecretID)
	config.SecretKey = strings.TrimSpace(config.SecretKey)
	config.SessionToken = strings.TrimSpace(config.SessionToken)
	config.Bucket = strings.TrimSpace(config.Bucket)
	config.Region = strings.TrimSpace(config.Region)
	config.Endpoint = strings.TrimSpace(config.Endpoint)
	config.BucketDomain = strings.TrimSpace(config.BucketDomain)
	return config
}

func trustedObjectURL(config ObjectConfig, key string) (string, error) {
	base := config.Endpoint
	if config.BucketDomain != "" {
		base = "https://" + config.BucketDomain
	}
	if base == "" {
		bucket, err := bucketURL(PutInput{Bucket: config.Bucket, Region: config.Region})
		if err != nil {
			return "", err
		}
		base = bucket.String()
	}
	joined, err := url.JoinPath(base, key)
	if err != nil {
		return "", ErrInvalidConfig
	}
	return joined, nil
}
