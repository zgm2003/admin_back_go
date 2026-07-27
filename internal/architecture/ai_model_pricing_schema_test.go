package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIModelPricingDatabaseContract(t *testing.T) {
	root := backendRoot(t)

	t.Run("guarded normalized migration", func(t *testing.T) {
		migration := normalizeAIModelPricingSQL(readAIModelPricingFile(t, root, "database", "migrations", "202607270103_ai_model_pricing.sql"))
		for _, required := range []string{
			"drop temporary table if exists _ai_model_pricing_guard",
			"create temporary table _ai_model_pricing_guard", "check (violations = 0)",
			"information_schema.tables", "ai_model_price_overrides", "ai_model_price_override_rates",
			"create table ai_model_price_overrides", "create table ai_model_price_override_rates",
			"catalog_vendor varchar(32)", "model_id varchar(191)", "version bigint unsigned not null",
			"source_url varchar(2048) not null", "verified_at date not null", "updated_by int unsigned not null",
			"created_at datetime(6) not null default current_timestamp(6)",
			"updated_at datetime(6) not null default current_timestamp(6) on update current_timestamp(6)",
			"unique key uk_ai_model_price_overrides_identity (catalog_vendor, model_id)",
			"constraint chk_ai_model_price_overrides_version check (version > 0)",
			"override_id bigint unsigned not null", "category varchar(32)", "unit varchar(32)", "tier_key varchar(64)",
			"price_units bigint not null", "unit_scale bigint unsigned not null",
			"unique key uk_ai_model_price_override_rates_key (override_id, category, unit, tier_key)",
			"constraint chk_ai_model_price_override_rates_price check (price_units >= 0)",
			"constraint chk_ai_model_price_override_rates_scale check (unit_scale > 0)",
			"constraint fk_ai_model_price_override_rates_override foreign key (override_id) references ai_model_price_overrides (id) on update restrict on delete cascade",
			"start transaction", "for update",
			"(921, '模型定价', '/ai/model-pricing', '', 5, 'ai/model-pricing', 'admin', 2, 7, 'ai_model_pricing_list', 'menu.ai_model_pricing', 1, 1, 2)",
			"(922, '编辑模型定价', '', '', 921, null, 'admin', 3, 1, 'ai_model_pricing_edit', '', 2, 1, 2)",
			"drop temporary table _ai_model_pricing_guard",
		} {
			if !strings.Contains(migration, required) {
				t.Errorf("AI model pricing migration missing %q", required)
			}
		}
		for _, forbidden := range []string{"role_permissions", "prices_json", "rates_json", "price_json"} {
			if strings.Contains(migration, forbidden) {
				t.Errorf("AI model pricing migration contains forbidden %q", forbidden)
			}
		}
		for _, table := range []string{"ai_model_price_overrides", "ai_model_price_override_rates"} {
			block := normalizedCreateTableBlock(t, migration, table)
			for _, forbidden := range []string{"is_del", "deleted_at", " json"} {
				if strings.Contains(block, forbidden) {
					t.Errorf("%s migration table contains forbidden %q", table, forbidden)
				}
			}
		}
	})

	t.Run("canonical hcl", func(t *testing.T) {
		schema := readAIModelPricingFile(t, root, "database", "schema", "admin.hcl")
		head := hclTableBlock(t, schema, "ai_model_price_overrides")
		rates := hclTableBlock(t, schema, "ai_model_price_override_rates")
		for _, required := range []string{
			`column "id"`, `type           = bigint`, `unsigned       = true`, `auto_increment = true`,
			`column "catalog_vendor"`, `type    = varchar(32)`, `charset = "ascii"`, `collate = "ascii_bin"`,
			`column "model_id"`, `type    = varchar(191)`, `column "version"`, `column "source_url"`, `type = varchar(2048)`,
			`column "verified_at"`, `type = date`, `column "updated_by"`, `type     = int`,
			`column "created_at"`, `column "updated_at"`, `index "uk_ai_model_price_overrides_identity"`,
			`columns = [column.catalog_vendor, column.model_id]`, `check "chk_ai_model_price_overrides_version"`,
		} {
			if !strings.Contains(head, required) {
				t.Errorf("ai_model_price_overrides HCL missing %q", required)
			}
		}
		for _, required := range []string{
			`column "id"`, `column "override_id"`, `column "category"`, `column "unit"`, `column "tier_key"`,
			`column "price_units"`, `column "unit_scale"`, `index "uk_ai_model_price_override_rates_key"`,
			`columns = [column.override_id, column.category, column.unit, column.tier_key]`,
			`foreign_key "fk_ai_model_price_override_rates_override"`, `ref_columns = [table.ai_model_price_overrides.column.id]`,
			`on_update   = RESTRICT`, `on_delete   = CASCADE`,
			`check "chk_ai_model_price_override_rates_price"`, `check "chk_ai_model_price_override_rates_scale"`,
		} {
			if !strings.Contains(rates, required) {
				t.Errorf("ai_model_price_override_rates HCL missing %q", required)
			}
		}
		for name, block := range map[string]string{"head": head, "rates": rates} {
			for _, forbidden := range []string{`column "is_del"`, `column "deleted_at"`, "json"} {
				if strings.Contains(strings.ToLower(block), forbidden) {
					t.Errorf("%s HCL contains forbidden %q", name, forbidden)
				}
			}
		}
	})

	t.Run("reconciliation and permission seed", func(t *testing.T) {
		verifySchema := strings.ToLower(readAIModelPricingFile(t, root, "database", "reconciliation", "030_verify_schema.sql"))
		for _, required := range []string{
			"ai_model_pricing_required_tables", "ai_model_pricing_required_columns", "ai_model_pricing_column_shapes",
			"ai_model_pricing_indexes", "ai_model_pricing_checks", "uk_ai_model_price_overrides_identity",
			"uk_ai_model_price_override_rates_key", "price_units", "unit_scale",
		} {
			if !strings.Contains(verifySchema, required) {
				t.Errorf("schema reconciliation missing %q", required)
			}
		}
		verifyRelations := strings.ToLower(readAIModelPricingFile(t, root, "database", "reconciliation", "031_verify_relations.sql"))
		for _, required := range []string{
			"ai_model_pricing_relationship_orphans", "ai_model_pricing_foreign_keys",
			"fk_ai_model_price_override_rates_override", "on delete cascade", "on update restrict",
		} {
			if !strings.Contains(verifyRelations, required) {
				t.Errorf("relation reconciliation missing %q", required)
			}
		}

		seed := readAIModelPricingFile(t, root, "database", "seeds", "admin_permissions.sql")
		for _, tuple := range []string{
			"(921, '模型定价', '/ai/model-pricing', '', 5, 'ai/model-pricing', 'admin', 2, 7, 'ai_model_pricing_list', 'menu.ai_model_pricing', 1, 1, 2)",
			"(922, '编辑模型定价', '', '', 921, NULL, 'admin', 3, 1, 'ai_model_pricing_edit', '', 2, 1, 2)",
		} {
			if strings.Count(seed, tuple) != 1 {
				t.Errorf("permission seed must contain exactly one %q", tuple)
			}
		}
		for _, required := range []string{"_ai_model_pricing_permission_seed_guard", "WHERE `id` IN (921, 922)", "ai_model_pricing_list", "ai_model_pricing_edit"} {
			if !strings.Contains(seed, required) {
				t.Errorf("permission seed missing collision guard %q", required)
			}
		}
	})
}

func readAIModelPricingFile(t *testing.T, root string, path ...string) string {
	t.Helper()
	fullPath := filepath.Join(append([]string{root}, path...)...)
	body, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(path...), err)
	}
	return string(body)
}

func normalizeAIModelPricingSQL(body string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.NewReplacer("`", "", "\r", " ", "\n", " ", "\t", " ").Replace(body)), " "))
}

func normalizedCreateTableBlock(t *testing.T, migration, table string) string {
	t.Helper()
	marker := "create table " + table + " "
	start := strings.Index(migration, marker)
	if start < 0 {
		t.Fatalf("migration missing %s", marker)
	}
	rest := migration[start:]
	end := strings.Index(rest, ") engine=innodb")
	if end < 0 {
		t.Fatalf("migration table %s has no ENGINE boundary", table)
	}
	return rest[:end]
}
