package payment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxCertificateBytes = 64 << 10

var certConfigCodePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// CertificateFile is a private Alipay certificate upload payload.
type CertificateFile struct {
	ConfigCode string
	CertType   string
	FileName   string
	Size       int64
	Reader     io.Reader
}

// CertificateSaveResult is the stored private certificate metadata returned to the API layer.
type CertificateSaveResult struct {
	Path     string
	FileName string
	SHA256   string
	Size     int64
}

// LocalCertStore writes Alipay certificates to a private local runtime directory.
type LocalCertStore struct {
	BaseDir string
}

func (s LocalCertStore) Save(ctx context.Context, file CertificateFile) (*CertificateSaveResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	code := strings.TrimSpace(file.ConfigCode)
	if code == "" || !certConfigCodePattern.MatchString(code) {
		return nil, fmt.Errorf("payment: invalid config code %q", file.ConfigCode)
	}
	if !validCertificateType(strings.TrimSpace(file.CertType)) {
		return nil, fmt.Errorf("payment: invalid certificate type %q", file.CertType)
	}
	if !validCertificateExt(file.FileName) {
		return nil, fmt.Errorf("payment: invalid certificate extension %q", file.FileName)
	}
	if file.Reader == nil {
		return nil, errors.New("payment: certificate reader is required")
	}
	if file.Size <= 0 || file.Size > maxCertificateBytes {
		return nil, fmt.Errorf("payment: certificate size must be between 1 and %d bytes", maxCertificateBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file.Reader, maxCertificateBytes+1))
	if err != nil {
		return nil, fmt.Errorf("payment: read certificate: %w", err)
	}
	if len(data) == 0 || len(data) > maxCertificateBytes {
		return nil, fmt.Errorf("payment: certificate size must be between 1 and %d bytes", maxCertificateBytes)
	}
	sum := sha256.Sum256(data)
	shaText := hex.EncodeToString(sum[:])
	relativePath := filepath.ToSlash(filepath.Join("runtime", "payment", "certs", "alipay", code, shaText+".crt"))
	baseDir := strings.TrimSpace(s.BaseDir)
	if baseDir == "" {
		baseDir = "."
	}
	fullPath := filepath.Join(filepath.FromSlash(baseDir), filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		return nil, fmt.Errorf("payment: create certificate directory: %w", err)
	}
	if err := os.WriteFile(fullPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("payment: write certificate: %w", err)
	}
	return &CertificateSaveResult{Path: relativePath, FileName: filepath.Base(file.FileName), SHA256: shaText, Size: int64(len(data))}, nil
}

func validCertificateType(value string) bool {
	switch value {
	case "app_cert", "alipay_cert", "alipay_root_cert":
		return true
	default:
		return false
	}
}

func validCertificateExt(name string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(name))) {
	case ".crt", ".pem":
		return true
	default:
		return false
	}
}
