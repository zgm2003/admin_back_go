package cos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	tencentcos "github.com/tencentyun/cos-go-sdk-v5"
)

var (
	ErrObjectStreamerNotConfigured = errors.New("cos object streamer is not configured")
	ErrObjectUnavailable           = errors.New("cos object is unavailable")
	ErrObjectVersionChanged        = errors.New("cos object version changed")
)

type ObjectStreamerConfig struct {
	Enabled    bool
	Timeout    time.Duration
	HTTPClient *http.Client
}

type COSObjectStreamer struct {
	enabled bool
	timeout time.Duration
	client  *http.Client
	config  ObjectConfigProvider
}

func NewObjectStreamer(provider ObjectConfigProvider, config ObjectStreamerConfig) *COSObjectStreamer {
	if config.Timeout <= 0 {
		config.Timeout = 60 * time.Second
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{}
	}
	return &COSObjectStreamer{
		enabled: config.Enabled, timeout: config.Timeout, client: config.HTTPClient, config: provider,
	}
}

func (streamer *COSObjectStreamer) Head(ctx context.Context, input infraai.PreparedFileOpenInput) (infraai.PreparedFileObjectMetadata, error) {
	if streamer == nil || !streamer.enabled {
		return infraai.PreparedFileObjectMetadata{}, ErrDisabled
	}
	input, err := normalizePreparedFileOpenInput(input)
	if err != nil {
		return infraai.PreparedFileObjectMetadata{}, err
	}
	return streamer.headObject(ctx, input.ObjectKey, input.ETag, input.Size)
}

func (streamer *COSObjectStreamer) headObject(ctx context.Context, objectKey, etag string, size int64) (infraai.PreparedFileObjectMetadata, error) {
	client, err := streamer.objectClient(ctx)
	if err != nil {
		return infraai.PreparedFileObjectMetadata{}, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, streamer.timeout)
	defer cancel()
	headers := make(http.Header)
	headers.Set("If-Match", etag)
	response, err := client.Object.Head(reqCtx, objectKey, &tencentcos.ObjectHeadOptions{XOptionHeader: &headers})
	if err != nil {
		return infraai.PreparedFileObjectMetadata{}, mapObjectStreamError("head", err)
	}
	metadata, err := preparedFileMetadata(response)
	if err != nil {
		return infraai.PreparedFileObjectMetadata{}, err
	}
	if metadata.ETag != etag || metadata.Size != size {
		return infraai.PreparedFileObjectMetadata{}, ErrObjectVersionChanged
	}
	return metadata, nil
}

func (streamer *COSObjectStreamer) Open(ctx context.Context, input infraai.PreparedFileOpenInput) (io.ReadCloser, infraai.PreparedFileObjectMetadata, error) {
	input, err := normalizePreparedFileOpenInput(input)
	if err != nil {
		return nil, infraai.PreparedFileObjectMetadata{}, err
	}
	return streamer.openObject(ctx, input.ObjectKey, input.ETag, input.Size)
}

func (streamer *COSObjectStreamer) openObject(ctx context.Context, objectKey, etag string, size int64) (io.ReadCloser, infraai.PreparedFileObjectMetadata, error) {
	client, err := streamer.objectClient(ctx)
	if err != nil {
		return nil, infraai.PreparedFileObjectMetadata{}, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, streamer.timeout)
	headers := make(http.Header)
	headers.Set("If-Match", etag)
	response, err := client.Object.Get(reqCtx, objectKey, &tencentcos.ObjectGetOptions{XOptionHeader: &headers})
	if err != nil {
		cancel()
		return nil, infraai.PreparedFileObjectMetadata{}, mapObjectStreamError("get", err)
	}
	metadata, err := preparedFileMetadata(response)
	if err != nil || metadata.ETag != etag || metadata.Size != size {
		_ = response.Body.Close()
		cancel()
		if err != nil {
			return nil, infraai.PreparedFileObjectMetadata{}, err
		}
		return nil, infraai.PreparedFileObjectMetadata{}, ErrObjectVersionChanged
	}
	return &contextObjectReadCloser{ReadCloser: response.Body, cancel: cancel}, metadata, nil
}

func (streamer *COSObjectStreamer) objectClient(ctx context.Context) (*tencentcos.Client, error) {
	if streamer == nil || !streamer.enabled {
		return nil, ErrDisabled
	}
	if streamer.config == nil {
		return nil, ErrObjectStreamerNotConfigured
	}
	config, err := streamer.config.ActiveObjectConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve active COS object config: %w", err)
	}
	config = normalizeObjectConfig(config)
	if config.SecretID == "" || config.SecretKey == "" || config.Bucket == "" || config.Region == "" {
		return nil, ErrInvalidConfig
	}
	bucket, err := bucketURL(PutInput{Bucket: config.Bucket, Region: config.Region, Endpoint: config.Endpoint})
	if err != nil {
		return nil, err
	}
	return tencentcos.NewClient(&tencentcos.BaseURL{BucketURL: bucket}, signedHTTPClient(streamer.client, PutInput{
		SecretID: config.SecretID, SecretKey: config.SecretKey, SessionToken: config.SessionToken,
	})), nil
}

func normalizePreparedFileOpenInput(input infraai.PreparedFileOpenInput) (infraai.PreparedFileOpenInput, error) {
	key, err := TrustedAIChatObjectKey(input.ObjectKey, "file")
	if err != nil {
		return infraai.PreparedFileOpenInput{}, err
	}
	input.ObjectKey = key
	input.ETag = strings.TrimSpace(input.ETag)
	if input.ETag == "" || input.Size <= 0 {
		return infraai.PreparedFileOpenInput{}, ErrInvalidObjectMetadata
	}
	return input, nil
}

func preparedFileMetadata(response *tencentcos.Response) (infraai.PreparedFileObjectMetadata, error) {
	if response == nil || response.Response == nil {
		return infraai.PreparedFileObjectMetadata{}, ErrInvalidObjectMetadata
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(response.Header.Get("Content-Type")))
	if err != nil || strings.TrimSpace(mediaType) == "" {
		return infraai.PreparedFileObjectMetadata{}, ErrInvalidObjectMetadata
	}
	size := response.ContentLength
	if header := strings.TrimSpace(response.Header.Get("Content-Length")); header != "" {
		size, err = strconv.ParseInt(header, 10, 64)
		if err != nil {
			return infraai.PreparedFileObjectMetadata{}, ErrInvalidObjectMetadata
		}
	}
	etag := strings.TrimSpace(response.Header.Get("ETag"))
	if etag == "" || size <= 0 {
		return infraai.PreparedFileObjectMetadata{}, ErrInvalidObjectMetadata
	}
	return infraai.PreparedFileObjectMetadata{ETag: etag, Size: size, MIMEType: strings.ToLower(mediaType)}, nil
}

func mapObjectStreamError(operation string, err error) error {
	if status, ok := HTTPStatus(err); ok {
		switch status {
		case http.StatusNotFound:
			return fmt.Errorf("cos object %s: %w", operation, ErrObjectUnavailable)
		case http.StatusPreconditionFailed:
			return fmt.Errorf("cos object %s: %w", operation, ErrObjectVersionChanged)
		}
	}
	return fmt.Errorf("cos object %s: %w", operation, err)
}

type contextObjectReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (body *contextObjectReadCloser) Close() error {
	if body == nil {
		return nil
	}
	body.once.Do(body.cancel)
	return body.ReadCloser.Close()
}

var _ infraai.PreparedFileOpener = (*COSObjectStreamer)(nil)
