package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconciliationCoreInvariantFilesCoverRelationsAndMoney(t *testing.T) {
	root := backendRoot(t)
	relations, err := os.ReadFile(filepath.Join(root, "database", "reconciliation", "031_verify_relations.sql"))
	if err != nil {
		t.Fatal(err)
	}
	money, err := os.ReadFile(filepath.Join(root, "database", "reconciliation", "032_verify_money.sql"))
	if err != nil {
		t.Fatal(err)
	}
	relationBody := strings.ToLower(string(relations))
	for _, required := range []string{
		"rbac_relationship_orphans", "payment_relationship_orphans", "wallet_relationship_orphans",
		"ai_relationship_orphans", "notification_relationship_orphans", "export_relationship_orphans",
		"authz_principal_versions", "payment_callback_events", "ai_image_files", "source_task_id",
		"redeem_code_foreign_keys", "redeem_code_relationship_orphans", "redeem_code_batch_quantity_mismatch",
	} {
		if !strings.Contains(relationBody, required) {
			t.Errorf("relationship invariants missing %q", required)
		}
	}
	moneyBody := strings.ToLower(string(money))
	for _, required := range []string{
		"wallet_balance_matches_ledger", "wallet_source_identity_unique", "payment_callback_identity_unique",
		"total_recharge_cents", "total_consume_cents", "direction='out'", "source_type in ('recharge','redeem_code')",
		"redeem_code_used_without_transaction", "redeem_code_transaction_without_used_code",
		"redeem_code_non_used_with_transaction",
	} {
		if !strings.Contains(moneyBody, required) {
			t.Errorf("money invariants missing %q", required)
		}
	}
}
