package payment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCertPathResolverResolvesAbsolutePath(t *testing.T) {
	certPath := writeTestCert(t)

	resolved, err := CertPathResolver{}.Resolve(certPath)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved != filepath.ToSlash(certPath) {
		t.Fatalf("expected %q, got %q", filepath.ToSlash(certPath), resolved)
	}
}

func TestCertPathResolverResolvesRelativePathFromGoCertBaseDir(t *testing.T) {
	goRoot := t.TempDir()
	certPath := filepath.Join(goRoot, "runtime", "cert", "alipay", "appPublicCert.crt")
	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		t.Fatalf("mkdir cert dir: %v", err)
	}
	if err := os.WriteFile(certPath, []byte("test-cert"), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	resolved, err := CertPathResolver{CertBaseDir: goRoot}.Resolve("runtime/cert/alipay/appPublicCert.crt")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved != filepath.ToSlash(certPath) {
		t.Fatalf("expected %q, got %q", filepath.ToSlash(certPath), resolved)
	}
}

func TestCertPathResolverFailsWhenGoCertMissing(t *testing.T) {
	_, err := CertPathResolver{CertBaseDir: t.TempDir()}.Resolve("runtime/cert/alipay/appPublicCert.crt")
	if err == nil {
		t.Fatalf("expected missing cert error")
	}
}

func TestCertPathResolverRejectsEmptyPath(t *testing.T) {
	_, err := CertPathResolver{}.Resolve("  ")
	if !errors.Is(err, ErrCertPathRequired) {
		t.Fatalf("expected ErrCertPathRequired, got %v", err)
	}
}

func TestCertPathResolverRejectsMissingFile(t *testing.T) {
	_, err := CertPathResolver{CertBaseDir: t.TempDir()}.Resolve("runtime/cert/alipay/missing.crt")
	if err == nil {
		t.Fatalf("expected missing cert error")
	}
}

func writeTestCert(t *testing.T) string {
	t.Helper()
	certPath := filepath.Join(t.TempDir(), "appPublicCert.crt")
	if err := os.WriteFile(certPath, []byte("test-cert"), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	return certPath
}
