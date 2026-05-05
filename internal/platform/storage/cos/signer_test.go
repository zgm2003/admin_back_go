package cos

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSignerRejectsInvalidInput(t *testing.T) {
	signer := NewSigner(Config{Enabled: true, Endpoint: "sts.tencentcloudapi.com", Region: "ap-guangzhou"})

	tests := []SignInput{
		{SecretKey: "sk", Bucket: "bucket", Region: "ap-guangzhou", Key: "images/a.png", TTL: time.Minute},
		{SecretID: "sid", Bucket: "bucket", Region: "ap-guangzhou", Key: "images/a.png", TTL: time.Minute},
		{SecretID: "sid", SecretKey: "sk", Region: "ap-guangzhou", Key: "images/a.png", TTL: time.Minute},
		{SecretID: "sid", SecretKey: "sk", Bucket: "bucket", Key: "images/a.png", TTL: time.Minute},
		{SecretID: "sid", SecretKey: "sk", Bucket: "bucket", Region: "ap-guangzhou", TTL: time.Minute},
	}

	for _, tt := range tests {
		_, err := signer.Sign(context.Background(), tt)
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("expected ErrInvalidConfig, got %v", err)
		}
	}
}

func TestSignerDisabledReturnsExplicitError(t *testing.T) {
	signer := NewSigner(Config{Enabled: false})

	_, err := signer.Sign(context.Background(), SignInput{
		SecretID: "sid", SecretKey: "skey", Bucket: "bucket", Region: "ap-nanjing", Key: "images/a.png", TTL: time.Minute,
	})

	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestSignerCallsSTSWithScopedPolicy(t *testing.T) {
	var got CredentialRequest
	requestCredential := func(ctx context.Context, input CredentialRequest) (*Credentials, error) {
		got = input
		return &Credentials{TmpSecretID: "tmp-id", TmpSecretKey: "tmp-key", SessionToken: "token", StartTime: 100, ExpiredTime: 200}, nil
	}
	signer := NewSigner(Config{Enabled: true, Endpoint: "sts.tencentcloudapi.com", Region: "ap-guangzhou", RequestCredential: requestCredential})

	creds, err := signer.Sign(context.Background(), SignInput{
		SecretID: "sid", SecretKey: "skey", Bucket: "bucket-1314", Region: "ap-nanjing", AppID: "1314", Key: "images/2026/05/05/demo.png", TTL: 10 * time.Minute,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.TmpSecretID != "tmp-id" || creds.SessionToken != "token" {
		t.Fatalf("unexpected credentials %#v", creds)
	}
	if got.SecretID != "sid" || got.SecretKey != "skey" || got.Endpoint != "sts.tencentcloudapi.com" || got.Region != "ap-guangzhou" || got.DurationSeconds != 600 {
		t.Fatalf("unexpected request: %#v", got)
	}
	if len(got.Policy.Statement) != 1 {
		t.Fatalf("expected one statement, got %#v", got.Policy)
	}
	statement := got.Policy.Statement[0]
	if statement.Effect != "allow" {
		t.Fatalf("unexpected effect %q", statement.Effect)
	}
	if !stringSliceEqual(statement.Action, []string{"cos:PutObject", "cos:PostObject"}) {
		t.Fatalf("unexpected actions %#v", statement.Action)
	}
	wantResource := "qcs::cos:ap-nanjing:uid/1314:bucket-1314/images/2026/05/05/demo.png"
	if !stringSliceEqual(statement.Resource, []string{wantResource}) {
		t.Fatalf("unexpected resource %#v", statement.Resource)
	}
}

func TestSignerPropagatesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	signer := NewSigner(Config{Enabled: true, Endpoint: "sts.tencentcloudapi.com", Region: "ap-guangzhou", RequestCredential: func(ctx context.Context, input CredentialRequest) (*Credentials, error) {
		return nil, ctx.Err()
	}})

	_, err := signer.Sign(ctx, SignInput{SecretID: "sid", SecretKey: "skey", Bucket: "bucket-1314", Region: "ap-nanjing", AppID: "1314", Key: "images/a.png", TTL: time.Minute})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestSignerUsesSDKBoundaryWithoutRealNetwork(t *testing.T) {
	var gotPolicy CredentialPolicy
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		policyJSON, err := url.QueryUnescape(r.FormValue("Policy"))
		if err != nil {
			t.Fatalf("unescape policy: %v", err)
		}
		if err := json.Unmarshal([]byte(policyJSON), &gotPolicy); err != nil {
			t.Fatalf("unmarshal policy: %v raw=%s", err, policyJSON)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Response":{"Credentials":{"TmpSecretId":"tmp-id","TmpSecretKey":"tmp-key","Token":"token"},"ExpiredTime":200,"StartTime":100,"RequestId":"req-1"}}`))
	}))
	defer server.Close()

	signer := NewSigner(Config{Enabled: true, Endpoint: server.URL, Region: "ap-guangzhou"})
	creds, err := signer.Sign(context.Background(), SignInput{
		SecretID: "sid", SecretKey: "skey", Bucket: "bucket-1314", Region: "ap-nanjing", Key: "images/demo.png", TTL: time.Minute,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.TmpSecretID != "tmp-id" || creds.TmpSecretKey != "tmp-key" || creds.SessionToken != "token" {
		t.Fatalf("unexpected credentials: %#v", creds)
	}
	if len(gotPolicy.Statement) != 1 || !strings.Contains(gotPolicy.Statement[0].Resource[0], "bucket-1314/images/demo.png") {
		t.Fatalf("unexpected policy %#v", gotPolicy)
	}
}

func stringSliceEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
