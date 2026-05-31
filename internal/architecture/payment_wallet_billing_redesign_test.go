package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPaymentWalletBillingRedesignContract(t *testing.T) {
	root := backendRoot(t)

	migration := readArchitectureFile(t, root, "database/migrations/20260530_payment_wallet_billing_redesign.sql")
	paymentRoutes := readArchitectureFile(t, root, "internal/module/payment/transport/admin/route.go")
	walletRoutes := readArchitectureFile(t, root, "internal/module/payment/wallet/transport/admin/route.go")
	walletDTO := readArchitectureFile(t, root, "internal/module/payment/wallet/dto.go")
	walletService := readArchitectureFile(t, root, "internal/module/payment/wallet/service.go")
	walletRepository := readArchitectureFile(t, root, "internal/module/payment/wallet/repository.go")
	walletZh := readArchitectureFile(t, root, "internal/shared/i18n/locales/zh-CN/wallet.yaml")
	walletEn := readArchitectureFile(t, root, "internal/shared/i18n/locales/en-US/wallet.yaml")
	routes := paymentRoutes + "\n" + walletRoutes

	for _, want := range []string{
		"ai_billing_rules",
		"ai_billing_records",
		"billing_record_id",
		"/payment/ledger",
		"/payment/wallets",
		"/profile/wallet",
		"payment_ledger_list",
		"payment_wallet_list",
		"ai_billing_rule_edit",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("migration must contain %q", want)
		}
	}

	for _, forbidden := range []string{
		"credit_logs",
		"canvas_credit_logs",
		"wallet_ledger",
		"payment_bill",
	} {
		if strings.Contains(migration, forbidden) {
			t.Fatalf("migration must not contain %q", forbidden)
		}
	}
	if containsSQLIdentifier(migration, "points") {
		t.Fatalf("migration must not contain SQL identifier %q", "points")
	}

	for _, forbidden := range []string{
		"/api/admin/v1/payment/orders",
		"/:id/sync",
		"/:id/close",
		"/consumptions",
		"/wallet/ledger",
		"/wallet/users",
	} {
		if strings.Contains(routes, forbidden) {
			t.Fatalf("admin payment/wallet routes must not expose %q", forbidden)
		}
	}

	for _, forbidden := range []string{
		"SourceConsume",
		"type ConsumeInput",
		"type ConsumeResponse",
		"func (s *Service) Consume",
		"func (r *GormRepository) Consume",
		"ErrConsumeSourceOwnerMismatch",
		"wallet.consume.",
	} {
		source := strings.Join([]string{walletDTO, walletService, walletRepository, walletZh, walletEn}, "\n")
		if strings.Contains(source, forbidden) {
			t.Fatalf("legacy wallet consume surface must not remain: %q", forbidden)
		}
	}
}

func readArchitectureFile(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(body)
}

func containsSQLIdentifier(source, identifier string) bool {
	pattern := `(?i)(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(identifier) + `([^A-Za-z0-9_]|$)`
	return regexp.MustCompile(pattern).FindStringIndex(source) != nil
}
