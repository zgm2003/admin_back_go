package payment

import (
	"context"
	"strings"
	"testing"
)

func TestLocalCertStoreSavesRelativeSHAPath(t *testing.T) {
	store := LocalCertStore{BaseDir: t.TempDir()}
	content := "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----"
	result, err := store.Save(context.Background(), CertificateFile{
		ConfigCode: "alipay_default",
		CertType:   "app_cert",
		FileName:   "appCertPublicKey.crt",
		Size:       int64(len(content)),
		Reader:     strings.NewReader(content),
	})
	if err != nil {
		t.Fatalf("Save error=%v", err)
	}
	if !strings.HasPrefix(result.Path, "runtime/payment/certs/alipay/alipay_default/") {
		t.Fatalf("unexpected path: %s", result.Path)
	}
	if !strings.HasSuffix(result.Path, ".crt") {
		t.Fatalf("unexpected extension: %s", result.Path)
	}
	if result.SHA256 == "" || result.Size == 0 {
		t.Fatalf("missing sha or size: %#v", result)
	}
}

func TestLocalCertStoreRejectsInvalidInput(t *testing.T) {
	store := LocalCertStore{BaseDir: t.TempDir()}
	cases := []CertificateFile{
		{ConfigCode: "../bad", CertType: "app_cert", FileName: "a.crt", Size: 1, Reader: strings.NewReader("x")},
		{ConfigCode: "ok", CertType: "bad", FileName: "a.crt", Size: 1, Reader: strings.NewReader("x")},
		{ConfigCode: "ok", CertType: "app_cert", FileName: "a.exe", Size: 1, Reader: strings.NewReader("x")},
		{ConfigCode: "ok", CertType: "app_cert", FileName: "a.crt", Size: 65537, Reader: strings.NewReader(strings.Repeat("x", 65537))},
	}
	for _, tc := range cases {
		if _, err := store.Save(context.Background(), tc); err == nil {
			t.Fatalf("expected reject for %#v", tc)
		}
	}
}
