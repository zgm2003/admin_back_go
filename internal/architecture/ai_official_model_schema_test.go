package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIOfficialModelFinalSchemaUsesOnlyCanonicalNames(t *testing.T) {
	root := backendRoot(t)
	migrationPath := filepath.Join(root, "database", "migrations", "202607280101_ai_official_models.sql")
	migration := normalizeOfficialModelSchema(t, migrationPath)
	for _, required := range []string{
		"create table ai_official_model_price_overrides",
		"create table ai_official_model_price_override_rates",
		"official_model_id", "official_catalog_version", "mapping_status", "mapped_at",
		"check (mapping_status in ('mapped','unmapped'))",
		"drop column max_output_tokens",
		"ai_official_model_list", "ai_official_model_price_sync",
	} {
		if !strings.Contains(migration, required) {
			t.Errorf("final official model migration missing %q", required)
		}
	}

	for _, relative := range []string{
		filepath.Join("database", "schema", "admin.hcl"),
		filepath.Join("database", "reconciliation", "030_verify_schema.sql"),
		filepath.Join("database", "reconciliation", "031_verify_relations.sql"),
		filepath.Join("database", "seeds", "admin_permissions.sql"),
	} {
		body := strings.ToLower(readOfficialModelSchemaFile(t, filepath.Join(root, relative)))
		for _, old := range []string{"ai_model_price_overrides", "ai_model_price_override_rates", "ai_model_pricing", "model-pricing"} {
			if strings.Contains(body, old) {
				t.Errorf("%s retains old name %q", relative, old)
			}
		}
	}

	hcl := strings.ToLower(readOfficialModelSchemaFile(t, filepath.Join(root, "database", "schema", "admin.hcl")))
	aiAgents := tableBlock(t, hcl, "ai_agents")
	if strings.Contains(aiAgents, `column "max_output_tokens"`) {
		t.Fatal("ai_agents still owns max_output_tokens")
	}
	providerModels := tableBlock(t, hcl, "ai_provider_models")
	for _, required := range []string{`column "official_model_id"`, `column "official_catalog_version"`, `column "mapping_status"`, `column "mapped_at"`, `check "chk_ai_provider_models_mapping"`} {
		if !strings.Contains(providerModels, required) {
			t.Errorf("ai_provider_models missing %q", required)
		}
	}
}

func TestAIOfficialModelMigrationUpgradesLegacyRBACBeforeDestructiveDDL(t *testing.T) {
	root := backendRoot(t)
	migrationPath := filepath.Join(root, "database", "migrations", "202607280101_ai_official_models.sql")
	migration := normalizeOfficialModelSchema(t, migrationPath)

	legacyGuard := strings.Index(migration, "ai_model_pricing_list")
	destructiveDDL := strings.Index(migration, "drop table if exists ai_model_price_override_rates")
	if legacyGuard < 0 || destructiveDDL < 0 || legacyGuard > destructiveDDL {
		t.Fatal("legacy RBAC must be validated before old pricing tables are dropped")
	}

	for _, required := range []string{
		"ai_model_pricing_edit",
		"update permissions",
		"insert into authz_principal_versions",
		"permission_id in (921, 922)",
	} {
		if !strings.Contains(migration, required) {
			t.Errorf("legacy RBAC upgrade missing %q", required)
		}
	}
}

func normalizeOfficialModelSchema(t *testing.T, path string) string {
	t.Helper()
	return strings.ToLower(strings.Join(strings.Fields(strings.NewReplacer("`", "", "\r", " ", "\n", " ", "\t", " ").Replace(readOfficialModelSchemaFile(t, path))), " "))
}

func readOfficialModelSchemaFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func tableBlock(t *testing.T, hcl string, name string) string {
	t.Helper()
	start := strings.Index(hcl, `table "`+name+`" {`)
	if start < 0 {
		t.Fatalf("missing HCL table %s", name)
	}
	rest := hcl[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("unterminated HCL table %s", name)
	}
	return rest[:end]
}
