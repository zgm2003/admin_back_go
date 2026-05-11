package payment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrCertPathRequired = errors.New("payment: cert path is required")

type CertPathResolver struct {
	CertBaseDir string
	WorkingDir  string
}

func (r CertPathResolver) Resolve(storedPath string) (string, error) {
	storedPath = strings.TrimSpace(strings.ReplaceAll(storedPath, "\\", "/"))
	if storedPath == "" {
		return "", ErrCertPathRequired
	}
	if filepath.IsAbs(storedPath) {
		return requireFile(storedPath)
	}
	for _, base := range []string{r.CertBaseDir, r.WorkingDir} {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		candidate := filepath.Join(filepath.FromSlash(base), filepath.FromSlash(storedPath))
		if resolved, err := requireFile(candidate); err == nil {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("payment: cert file not found: %s", storedPath)
}

func requireFile(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("payment: resolve cert abs path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("payment: cert file not readable: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("payment: cert path is directory: %s", abs)
	}
	return filepath.ToSlash(abs), nil
}
