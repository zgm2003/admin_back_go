package cos

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	tencentcos "github.com/tencentyun/cos-go-sdk-v5"
)

type ObjectWriterConfig struct {
	Enabled    bool
	Timeout    time.Duration
	HTTPClient *http.Client
}

type PutInput struct {
	SecretID     string
	SecretKey    string
	SessionToken string
	Bucket       string
	Region       string
	Key          string
	Body         []byte
	ContentType  string
	Endpoint     string
}

type ObjectWriter interface {
	Put(ctx context.Context, input PutInput) error
}

type COSObjectWriter struct {
	enabled    bool
	timeout    time.Duration
	httpClient *http.Client
}

func NewObjectWriter(cfg ObjectWriterConfig) *COSObjectWriter {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	return &COSObjectWriter{
		enabled:    cfg.Enabled,
		timeout:    cfg.Timeout,
		httpClient: cfg.HTTPClient,
	}
}

func (w *COSObjectWriter) Put(ctx context.Context, input PutInput) error {
	if w == nil || !w.enabled {
		return ErrDisabled
	}
	input = normalizePutInput(input)
	if input.SecretID == "" || input.SecretKey == "" || input.Bucket == "" || input.Region == "" || input.Key == "" || len(input.Body) == 0 {
		return ErrInvalidConfig
	}

	bucketURL, err := bucketURL(input)
	if err != nil {
		return fmt.Errorf("cos object put: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	client := tencentcos.NewClient(&tencentcos.BaseURL{BucketURL: bucketURL}, signedHTTPClient(w.httpClient, input))
	_, err = client.Object.Put(reqCtx, input.Key, bytes.NewReader(input.Body), &tencentcos.ObjectPutOptions{
		ObjectPutHeaderOptions: &tencentcos.ObjectPutHeaderOptions{
			ContentType:   input.ContentType,
			ContentLength: int64(len(input.Body)),
		},
	})
	if err != nil {
		return fmt.Errorf("cos object put: %w", err)
	}
	return nil
}

func normalizePutInput(input PutInput) PutInput {
	input.SecretID = strings.TrimSpace(input.SecretID)
	input.SecretKey = strings.TrimSpace(input.SecretKey)
	input.SessionToken = strings.TrimSpace(input.SessionToken)
	input.Bucket = strings.TrimSpace(input.Bucket)
	input.Region = strings.TrimSpace(input.Region)
	input.Key = strings.TrimLeft(strings.TrimSpace(input.Key), "/")
	input.ContentType = strings.TrimSpace(input.ContentType)
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	if input.ContentType == "" {
		input.ContentType = "application/octet-stream"
	}
	return input
}

func bucketURL(input PutInput) (*url.URL, error) {
	rawURL := input.Endpoint
	if rawURL == "" {
		rawURL = fmt.Sprintf("https://%s.cos.%s.myqcloud.com", input.Bucket, input.Region)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, ErrInvalidConfig
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, ErrInvalidConfig
	}
	return parsed, nil
}

func signedHTTPClient(base *http.Client, input PutInput) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	copied := *base
	copied.Transport = &tencentcos.AuthorizationTransport{
		SecretID:     input.SecretID,
		SecretKey:    input.SecretKey,
		SessionToken: input.SessionToken,
		Transport:    base.Transport,
	}
	return &copied
}
