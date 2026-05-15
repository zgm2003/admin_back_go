package cos

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tencentcos "github.com/tencentyun/cos-go-sdk-v5"
)

const defaultMaxObjectReadBytes int64 = 64 << 20

type ObjectReaderConfig struct {
	Enabled    bool
	Timeout    time.Duration
	MaxBytes   int64
	HTTPClient *http.Client
}

type GetInput struct {
	SecretID     string
	SecretKey    string
	SessionToken string
	Bucket       string
	Region       string
	Key          string
	Endpoint     string
}

type GetResult struct {
	Body        []byte
	ContentType string
}

type ObjectReader interface {
	Get(ctx context.Context, input GetInput) (*GetResult, error)
}

type COSObjectReader struct {
	enabled    bool
	timeout    time.Duration
	maxBytes   int64
	httpClient *http.Client
}

func NewObjectReader(cfg ObjectReaderConfig) *COSObjectReader {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = defaultMaxObjectReadBytes
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	return &COSObjectReader{
		enabled:    cfg.Enabled,
		timeout:    cfg.Timeout,
		maxBytes:   cfg.MaxBytes,
		httpClient: cfg.HTTPClient,
	}
}

func (r *COSObjectReader) Get(ctx context.Context, input GetInput) (*GetResult, error) {
	if r == nil || !r.enabled {
		return nil, ErrDisabled
	}
	input = normalizeGetInput(input)
	if input.SecretID == "" || input.SecretKey == "" || input.Bucket == "" || input.Region == "" || input.Key == "" {
		return nil, ErrInvalidConfig
	}
	bucketURL, err := bucketURL(PutInput{Bucket: input.Bucket, Region: input.Region, Endpoint: input.Endpoint})
	if err != nil {
		return nil, fmt.Errorf("cos object get: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	client := tencentcos.NewClient(&tencentcos.BaseURL{BucketURL: bucketURL}, signedHTTPClient(r.httpClient, PutInput{
		SecretID:     input.SecretID,
		SecretKey:    input.SecretKey,
		SessionToken: input.SessionToken,
	}))
	resp, err := client.Object.Get(reqCtx, input.Key, nil)
	if err != nil {
		return nil, fmt.Errorf("cos object get: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, r.maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cos object get: %w", err)
	}
	if int64(len(body)) > r.maxBytes {
		return nil, fmt.Errorf("cos object get: object too large")
	}
	contentType := ""
	if resp.Header != nil {
		contentType = strings.TrimSpace(resp.Header.Get("Content-Type"))
	}
	return &GetResult{Body: body, ContentType: contentType}, nil
}

func normalizeGetInput(input GetInput) GetInput {
	input.SecretID = strings.TrimSpace(input.SecretID)
	input.SecretKey = strings.TrimSpace(input.SecretKey)
	input.SessionToken = strings.TrimSpace(input.SessionToken)
	input.Bucket = strings.TrimSpace(input.Bucket)
	input.Region = strings.TrimSpace(input.Region)
	input.Key = strings.TrimLeft(strings.TrimSpace(input.Key), "/")
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	return input
}
