package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIBillingWave0CanonicalSchemaDropsWalletCents(t *testing.T) {
	root := backendRoot(t)
	schema := readAIBillingWave0File(t, root, "database", "schema", "admin.hcl")

	for tableName, forbidden := range map[string][]string{
		"user_wallets":        {`column "balance_cents"`, `column "total_recharge_cents"`, `column "total_consume_cents"`},
		"wallet_transactions": {`column "amount_cents"`, `column "balance_before_cents"`, `column "balance_after_cents"`},
	} {
		table := hclTableBlock(t, schema, tableName)
		for _, column := range forbidden {
			if strings.Contains(table, column) {
				t.Errorf("canonical %s retained contracted column %s", tableName, column)
			}
		}
	}
}

func TestAIBillingWave0LegacyIdentityUsesDurableNonReplayableBoundary(t *testing.T) {
	root := backendRoot(t)
	expand := readAIBillingWave0File(t, root, "database", "migrations", "202607250101_ai_billing_expand.sql")
	backfill := readAIBillingWave0File(t, root, "database", "migrations", "202607250102_ai_billing_backfill.sql")
	contract := readAIBillingWave0File(t, root, "database", "migrations", "202607250103_ai_billing_contract.sql")

	for _, required := range []string{"ai_billing_migration_metadata", "legacy_cutover_at", "marker_version", "request_identity_status", "request_identity_marker"} {
		if !strings.Contains(expand, required) {
			t.Errorf("expand migration missing %q", required)
		}
	}
	for _, required := range []string{
		"legacy_cutover_v1", "legacy_non_replayable_v1", "legacy_non_replayable",
		"UNHEX(SHA2(CONCAT('legacy_non_replayable_v1:ai_runs:', `id`), 256))",
		"`created_at` >= @ai_billing_legacy_cutover_at",
		`{"version":"legacy_unavailable_v1","replayable":false}`,
	} {
		if !strings.Contains(backfill, required) {
			t.Errorf("backfill migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"|request:", "|text:", "|operation:", "@ai_billing_contract_started_at"} {
		if strings.Contains(backfill, forbidden) || strings.Contains(contract, forbidden) {
			t.Errorf("legacy identity still relies on non-canonical or transient boundary token %q", forbidden)
		}
	}
	for _, required := range []string{
		"JOIN `ai_billing_migration_metadata` AS metadata",
		"run_row.`created_at` >= metadata.`legacy_cutover_at`",
		"`legacy_cutover_at` <= CURRENT_TIMESTAMP(0)",
		"child_legacy_identity_after_cutover",
		"run_row.`request_identity_status` = 'legacy_non_replayable'",
		"attempt.`prepared_request_sha256` <> UNHEX(SHA2",
	} {
		if !strings.Contains(contract, required) {
			t.Errorf("contract migration missing %q", required)
		}
	}
}

func readAIBillingWave0File(t *testing.T, root string, parts ...string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
