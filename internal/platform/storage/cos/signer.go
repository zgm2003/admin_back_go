package cos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	sts "github.com/tencentyun/qcloud-cos-sts-sdk/go"
)

var (
	ErrDisabled      = errors.New("cos sts: disabled")
	ErrInvalidConfig = errors.New("cos sts: invalid config")
)

type Config struct {
	Enabled           bool
	Endpoint          string
	Region            string
	Timeout           time.Duration
	HTTPClient        *http.Client
	RequestCredential CredentialRequester
}

type SignInput struct {
	SecretID  string
	SecretKey string
	Bucket    string
	Region    string
	AppID     string
	Key       string
	TTL       time.Duration
}

type Credentials struct {
	TmpSecretID  string
	TmpSecretKey string
	SessionToken string
	StartTime    int64
	ExpiredTime  int64
}

type CredentialSigner interface {
	Sign(ctx context.Context, input SignInput) (*Credentials, error)
}

type DisabledSigner struct{}

func (DisabledSigner) Sign(ctx context.Context, input SignInput) (*Credentials, error) {
	return nil, ErrDisabled
}

type Signer struct {
	enabled           bool
	endpoint          string
	region            string
	timeout           time.Duration
	httpClient        *http.Client
	requestCredential CredentialRequester
}

type CredentialRequester func(ctx context.Context, input CredentialRequest) (*Credentials, error)

type CredentialRequest struct {
	SecretID        string
	SecretKey       string
	Endpoint        string
	Region          string
	DurationSeconds int64
	HTTPClient      *http.Client
	Policy          CredentialPolicy
}

type CredentialPolicy struct {
	Version   string                      `json:"version,omitempty"`
	Statement []CredentialPolicyStatement `json:"statement,omitempty"`
}

type CredentialPolicyStatement struct {
	Action   []string `json:"action,omitempty"`
	Effect   string   `json:"effect,omitempty"`
	Resource []string `json:"resource,omitempty"`
}

func NewSigner(cfg Config) *Signer {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "sts.tencentcloudapi.com"
	}
	if cfg.Region == "" {
		cfg.Region = "ap-guangzhou"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	if cfg.RequestCredential == nil {
		cfg.RequestCredential = requestCredentialWithSDK
	}
	return &Signer{
		enabled:           cfg.Enabled,
		endpoint:          cfg.Endpoint,
		region:            cfg.Region,
		timeout:           cfg.Timeout,
		httpClient:        cfg.HTTPClient,
		requestCredential: cfg.RequestCredential,
	}
}

func (s *Signer) Sign(ctx context.Context, input SignInput) (*Credentials, error) {
	if s == nil || !s.enabled {
		return nil, ErrDisabled
	}
	if input.SecretID == "" || input.SecretKey == "" || input.Bucket == "" || input.Region == "" || input.Key == "" {
		return nil, ErrInvalidConfig
	}
	if input.TTL <= 0 {
		return nil, ErrInvalidConfig
	}

	endpoint := s.endpoint
	if endpoint == "" {
		endpoint = "sts.tencentcloudapi.com"
	}
	region := s.region
	if region == "" {
		region = "ap-guangzhou"
	}
	reqCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	credentials, err := s.requestCredential(reqCtx, CredentialRequest{
		SecretID:        input.SecretID,
		SecretKey:       input.SecretKey,
		Endpoint:        endpoint,
		Region:          region,
		DurationSeconds: int64(input.TTL.Seconds()),
		HTTPClient:      s.httpClient,
		Policy: CredentialPolicy{
			Version: "2.0",
			Statement: []CredentialPolicyStatement{
				{
					Action: []string{
						"cos:PutObject",
						"cos:PostObject",
					},
					Effect: "allow",
					Resource: []string{
						cosResource(input),
					},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("cos sts sign: %w", err)
	}
	if credentials == nil || credentials.TmpSecretID == "" || credentials.TmpSecretKey == "" || credentials.SessionToken == "" {
		return nil, fmt.Errorf("cos sts sign: %w", ErrInvalidConfig)
	}
	return credentials, nil
}

func requestCredentialWithSDK(ctx context.Context, input CredentialRequest) (*Credentials, error) {
	httpClient := httpClientWithContext(ctx, input.HTTPClient)
	host := input.Endpoint
	scheme := "https"
	if parsed, err := url.Parse(input.Endpoint); err == nil && parsed.Host != "" {
		host = parsed.Host
		scheme = parsed.Scheme
	}
	client := sts.NewClient(input.SecretID, input.SecretKey, httpClient, sts.Host(host), sts.Scheme(scheme))
	resp, err := client.RequestCredential(&sts.CredentialOptions{
		Region:          input.Region,
		DurationSeconds: input.DurationSeconds,
		Policy: &sts.CredentialPolicy{
			Version: input.Policy.Version,
			Statement: []sts.CredentialPolicyStatement{
				{
					Action:   input.Policy.Statement[0].Action,
					Effect:   input.Policy.Statement[0].Effect,
					Resource: input.Policy.Statement[0].Resource,
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var complete sts.CredentialCompleteResult
	if err := json.NewDecoder(resp.Body).Decode(&complete); err != nil {
		return nil, err
	}
	if complete.Response != nil && complete.Response.Error != nil {
		complete.Response.Error.RequestId = complete.Response.RequestId
		return nil, complete.Response.Error
	}
	if complete.Response == nil || complete.Response.Credentials == nil {
		return nil, ErrInvalidConfig
	}
	return &Credentials{
		TmpSecretID:  complete.Response.Credentials.TmpSecretID,
		TmpSecretKey: complete.Response.Credentials.TmpSecretKey,
		SessionToken: complete.Response.Credentials.SessionToken,
		StartTime:    int64(complete.Response.StartTime),
		ExpiredTime:  int64(complete.Response.ExpiredTime),
	}, nil
}

type contextTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t contextTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if t.ctx == nil {
		return base.RoundTrip(req)
	}
	return base.RoundTrip(req.WithContext(t.ctx))
}

func httpClientWithContext(ctx context.Context, client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	copied := *client
	copied.Transport = contextTransport{ctx: ctx, base: client.Transport}
	return &copied
}

func cosResource(input SignInput) string {
	appID := strings.TrimSpace(input.AppID)
	if appID == "" {
		appID = appIDFromBucket(input.Bucket)
	}
	return fmt.Sprintf("qcs::cos:%s:uid/%s:%s/%s", input.Region, appID, input.Bucket, strings.TrimLeft(input.Key, "/"))
}

func appIDFromBucket(bucket string) string {
	index := strings.LastIndex(bucket, "-")
	if index < 0 || index == len(bucket)-1 {
		return ""
	}
	return bucket[index+1:]
}
