package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalletOwnedByPaymentModule(t *testing.T) {
	root := backendRoot(t)

	mustNotExist(t, root, "internal/module/wallet")
	mustExist(t, root, "internal/module/payment/wallet/service.go")
	mustExist(t, root, "internal/module/payment/wallet/repository.go")
	mustExist(t, root, "internal/module/payment/wallet/transport/admin/route.go")
}

func TestNoImportsOfOldWalletModulePath(t *testing.T) {
	root := backendRoot(t)
	banned := `"admin_back_go/internal/module/wallet"`
	var offenders []string

	for _, base := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, base), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(root, "internal/architecture/payment_wallet_aggregation_test.go")) {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(body), banned) {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s go files: %v", base, err)
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("old wallet module imports remain:\n  %s", strings.Join(offenders, "\n  "))
	}
}
