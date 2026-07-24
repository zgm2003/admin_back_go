package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWalletRedeemCodeDatabaseContract(t *testing.T) {
	root := backendRoot(t)

	t.Run("schema migration", func(t *testing.T) {
		body := readWalletRedeemCodeContractFile(t, root, "database", "migrations", "202607240101_wallet_redeem_codes.sql")
		normalized := normalizeWalletRedeemCodeSQL(body)
		for _, required := range []string{
			"drop temporary table if exists _wallet_redeem_code_schema_guard",
			"create temporary table _wallet_redeem_code_schema_guard",
			"check (violations = 0)",
			"information_schema.tables", "table_name in ('redeem_code_batches', 'redeem_codes')",
			"information_schema.columns", "table_name = 'users'", "column_name = 'id'", "column_type = 'int unsigned'",
			"create table redeem_code_batches", "create table redeem_codes",
			"id bigint not null auto_increment", "batch_no varchar(64) character set ascii collate ascii_bin not null",
			"request_id varchar(128) character set ascii collate ascii_bin not null",
			"request_fingerprint_version varchar(64) character set ascii collate ascii_bin not null",
			"request_fingerprint char(64) character set ascii collate ascii_bin not null",
			"amount_cents bigint not null", "quantity int unsigned not null", "expires_at datetime(6) null",
			"note varchar(255) not null default ''", "created_by int unsigned not null",
			"created_at datetime(6) not null default current_timestamp(6)",
			"updated_at datetime(6) not null default current_timestamp(6) on update current_timestamp(6)",
			"unique key uk_redeem_code_batches_batch_no (batch_no)",
			"unique key uk_redeem_code_batches_creator_request (created_by, request_id)",
			"key idx_redeem_code_batches_created_at_id (created_at, id)",
			"key idx_redeem_code_batches_expires_at_id (expires_at, id)",
			"constraint chk_redeem_code_batches_amount_cents check (amount_cents between 1 and 100000000)",
			"constraint chk_redeem_code_batches_quantity check (quantity between 1 and 1000)",
			"constraint chk_redeem_code_batches_expiry check (expires_at is null or expires_at > created_at)",
			"constraint fk_redeem_code_batches_created_by foreign key (created_by) references users (id) on update restrict on delete restrict",
			"batch_id bigint not null", "code char(28) character set ascii collate ascii_bin not null",
			"state varchar(16) not null", "used_by int unsigned null", "used_at datetime(6) null",
			"unique key uk_redeem_codes_code (code)",
			"key idx_redeem_codes_batch_state_id (batch_id, state, id)",
			"key idx_redeem_codes_state_id (state, id)",
			"key idx_redeem_codes_used_by_used_at_id (used_by, used_at, id)",
			"constraint chk_redeem_codes_state check (state in ('unused', 'used', 'voided'))",
			"constraint fk_redeem_codes_batch foreign key (batch_id) references redeem_code_batches (id) on update restrict on delete restrict",
			"constraint fk_redeem_codes_used_by foreign key (used_by) references users (id) on update restrict on delete restrict",
			"drop temporary table _wallet_redeem_code_schema_guard",
		} {
			if !strings.Contains(normalized, required) {
				t.Errorf("wallet redeem-code schema migration missing %q", required)
			}
		}
		for _, requiredPattern := range []string{
			`constraint chk_redeem_codes_usage check \(\(state = 'used' and used_by is not null and used_at is not null\) or \(state in \('unused', 'voided'\) and used_by is null and used_at is null\)\s*\)`,
		} {
			if !regexp.MustCompile(requiredPattern).MatchString(normalized) {
				t.Errorf("wallet redeem-code schema migration missing pattern %q", requiredPattern)
			}
		}
		if strings.Contains(normalized, "id bigint unsigned") || strings.Contains(normalized, "batch_id bigint unsigned") {
			t.Fatal("redeem-code primary and batch identities must remain signed BIGINT")
		}
		if regexp.MustCompile(`(?i)create table (redeem_code_batches|redeem_codes)[\s\S]*?\bis_del\b`).MatchString(normalized) {
			t.Fatal("redeem-code tables must not add soft-delete columns")
		}
		if strings.Contains(normalized, "start transaction") || strings.Contains(normalized, "permissions") || strings.Contains(normalized, "role_permissions") || strings.Contains(normalized, "authz_principal_versions") {
			t.Fatal("schema revision must contain schema facts only")
		}
	})

	t.Run("permission migration", func(t *testing.T) {
		body := readWalletRedeemCodeContractFile(t, root, "database", "migrations", "202607240102_wallet_redeem_code_permissions.sql")
		normalized := normalizeWalletRedeemCodeSQL(body)
		for _, required := range []string{
			"drop temporary table if exists _wallet_redeem_code_permission_guard",
			"create temporary table _wallet_redeem_code_permission_guard", "check (violations = 0)",
			"information_schema.tables", "redeem_code_batches", "redeem_codes",
			"role_row.id = 1", "role_row.is_del = 2", "permission.id = 437", "permission.code = 'payment'",
			"permission.platform = 'admin'", "permission.type = 1", "permission.is_del = 2",
			"permission.id in (657, 658, 659)",
			"permission.code in ('payment_redeem_code_list', 'payment_redeem_code_generate', 'payment_redeem_code_void')",
			"18446744073709551615", "start transaction", "for update",
			"(657, '兑换码管理', '/payment/redeem-codes', 'ticket', 437, 'payment/redeem-codes', 'admin', 2, 35, 'payment_redeem_code_list', 'menu.payment_redeem_codes', 1, 1, 2)",
			"(658, '批量生成兑换码', '', '', 657, null, 'admin', 3, 1, 'payment_redeem_code_generate', '', 2, 1, 2)",
			"(659, '作废兑换码', '', '', 657, null, 'admin', 3, 2, 'payment_redeem_code_void', '', 2, 1, 2)",
			"insert into role_permissions (role_id, permission_id, is_del)",
			"select 1, permission.id, 2", "permission.id in (437, 657, 658, 659)",
			"insert into authz_principal_versions (user_id, platform, version, updated_at)",
			"user_row.role_id = 1", "user_row.status = 1", "user_row.is_del = 2",
			"principal_version.version = principal_version.version + 1", "principal_version.platform = 'admin'",
			"commit", "drop temporary table _wallet_redeem_code_permission_guard",
		} {
			if !strings.Contains(normalized, required) {
				t.Errorf("wallet redeem-code permission migration missing %q", required)
			}
		}
		if strings.Contains(normalized, "role_row.status") {
			t.Fatal("administrator role preflight must not require a roles.status column")
		}
		for _, forbidden := range []string{"create table redeem_code_batches", "create table redeem_codes", "alter table redeem_code_batches", "alter table redeem_codes"} {
			if strings.Contains(normalized, forbidden) {
				t.Fatalf("permission revision contains schema DDL %q", forbidden)
			}
		}
	})

	t.Run("canonical schema", func(t *testing.T) {
		schema := readWalletRedeemCodeContractFile(t, root, "database", "schema", "admin.hcl")
		batchTable := hclTableBlock(t, schema, "redeem_code_batches")
		codeTable := hclTableBlock(t, schema, "redeem_codes")
		for _, required := range []string{
			`column "id"`, `type           = bigint`, `auto_increment = true`,
			`column "batch_no"`, `type    = varchar(64)`, `charset = "ascii"`, `collate = "ascii_bin"`,
			`column "request_id"`, `type    = varchar(128)`, `column "request_fingerprint_version"`,
			`column "request_fingerprint"`, `type    = char(64)`, `column "amount_cents"`,
			`column "quantity"`, `type     = int`, `unsigned = true`, `column "expires_at"`, `type = datetime(6)`,
			`column "note"`, `default = ""`, `column "created_by"`, `column "created_at"`, `column "updated_at"`,
			`index "uk_redeem_code_batches_batch_no"`, `columns = [column.batch_no]`,
			`index "uk_redeem_code_batches_creator_request"`, `columns = [column.created_by, column.request_id]`,
			`index "idx_redeem_code_batches_created_at_id"`, `columns = [column.created_at, column.id]`,
			`index "idx_redeem_code_batches_expires_at_id"`, `columns = [column.expires_at, column.id]`,
			`check "chk_redeem_code_batches_amount_cents"`, `check "chk_redeem_code_batches_quantity"`,
			`check "chk_redeem_code_batches_expiry"`, `foreign_key "fk_redeem_code_batches_created_by"`,
			`ref_columns = [table.users.column.id]`, `on_update   = RESTRICT`, `on_delete   = RESTRICT`,
		} {
			if !strings.Contains(batchTable, required) {
				t.Errorf("canonical redeem_code_batches table missing %q", required)
			}
		}
		for _, required := range []string{
			`column "id"`, `type           = bigint`, `auto_increment = true`, `column "batch_id"`,
			`column "code"`, `type    = char(28)`, `charset = "ascii"`, `collate = "ascii_bin"`,
			`column "state"`, `type = varchar(16)`, `column "used_by"`, `column "used_at"`,
			`column "created_at"`, `column "updated_at"`, `index "uk_redeem_codes_code"`,
			`index "idx_redeem_codes_batch_state_id"`, `columns = [column.batch_id, column.state, column.id]`,
			`index "idx_redeem_codes_state_id"`, `columns = [column.state, column.id]`,
			`index "idx_redeem_codes_used_by_used_at_id"`, `columns = [column.used_by, column.used_at, column.id]`,
			`check "chk_redeem_codes_state"`, `check "chk_redeem_codes_usage"`,
			`foreign_key "fk_redeem_codes_batch"`, `ref_columns = [table.redeem_code_batches.column.id]`,
			`foreign_key "fk_redeem_codes_used_by"`, `ref_columns = [table.users.column.id]`,
			`on_update   = RESTRICT`, `on_delete   = RESTRICT`,
		} {
			if !strings.Contains(codeTable, required) {
				t.Errorf("canonical redeem_codes table missing %q", required)
			}
		}
		for name, table := range map[string]string{"redeem_code_batches": batchTable, "redeem_codes": codeTable} {
			if strings.Contains(table, `unsigned       = true`) && strings.Contains(hclColumnBlock(t, table, "id"), `unsigned`) {
				t.Errorf("%s.id must be signed", name)
			}
			if strings.Contains(table, `column "is_del"`) {
				t.Errorf("%s must not contain is_del", name)
			}
		}
	})

	t.Run("permissions and reconciliation", func(t *testing.T) {
		seed := readWalletRedeemCodeContractFile(t, root, "database", "seeds", "admin_permissions.sql")
		for _, tuple := range []string{
			"(657, '兑换码管理', '/payment/redeem-codes', 'Ticket', 437, 'payment/redeem-codes', 'admin', 2, 35, 'payment_redeem_code_list', 'menu.payment_redeem_codes', 1, 1, 2)",
			"(658, '批量生成兑换码', '', '', 657, NULL, 'admin', 3, 1, 'payment_redeem_code_generate', '', 2, 1, 2)",
			"(659, '作废兑换码', '', '', 657, NULL, 'admin', 3, 2, 'payment_redeem_code_void', '', 2, 1, 2)",
		} {
			if strings.Count(seed, tuple) != 1 {
				t.Errorf("permission seed must contain exactly one %q", tuple)
			}
		}

		for _, rel := range []string{"020_backfill_core.sql", "032_verify_money.sql", "050_contract_preconditions.sql"} {
			body := normalizeWalletRedeemCodeSQL(readWalletRedeemCodeContractFile(t, root, "database", "reconciliation", rel))
			fundingPattern := regexp.MustCompile(`(?:[a-z_]+\.)?direction\s*=\s*'in'\s+and\s+(?:[a-z_]+\.)?source_type\s+in\s*\(\s*'recharge'\s*,\s*'redeem_code'\s*\)`)
			if !fundingPattern.MatchString(body) {
				t.Errorf("%s does not include redeem_code in cumulative recharge funding", rel)
			}
		}

		verifySchema := strings.ToLower(readWalletRedeemCodeContractFile(t, root, "database", "reconciliation", "030_verify_schema.sql"))
		for _, required := range []string{
			"redeem_code_required_tables", "redeem_code_required_columns", "redeem_code_column_shapes",
			"redeem_code_indexes", "redeem_code_checks",
		} {
			if !strings.Contains(verifySchema, required) {
				t.Errorf("schema reconciliation missing %q", required)
			}
		}

		verifyRelations := strings.ToLower(readWalletRedeemCodeContractFile(t, root, "database", "reconciliation", "031_verify_relations.sql"))
		for _, required := range []string{
			"redeem_code_relationship_orphans", "redeem_code_batch_quantity_mismatch",
			"redeem_code_batches", "redeem_codes", "created_by", "used_by", "quantity",
		} {
			if !strings.Contains(verifyRelations, required) {
				t.Errorf("relation reconciliation missing %q", required)
			}
		}

		for _, rel := range []string{"032_verify_money.sql", "050_contract_preconditions.sql"} {
			body := strings.ToLower(readWalletRedeemCodeContractFile(t, root, "database", "reconciliation", rel))
			for _, required := range []string{
				"redeem_code_used_without_transaction", "redeem_code_transaction_without_used_code",
				"redeem_code_non_used_with_transaction", "source_type", "redeem_code", "source_id",
				"balance_before_cents", "balance_after_cents", "amount_cents", "direction", "used_by", "wallet_id",
			} {
				if !strings.Contains(body, required) {
					t.Errorf("%s missing redeem-code money invariant %q", rel, required)
				}
			}
		}
	})

	t.Run("overflow safe money arithmetic", func(t *testing.T) {
		invariants := []struct {
			name     string
			operator string
		}{
			{name: "redeem_code_used_without_transaction", operator: `=`},
			{name: "redeem_code_transaction_without_used_code", operator: `<>`},
		}
		for _, rel := range []string{"032_verify_money.sql", "050_contract_preconditions.sql"} {
			body := readWalletRedeemCodeContractFile(t, root, "database", "reconciliation", rel)
			for _, invariant := range invariants {
				block := normalizeWalletRedeemCodeSQL(walletRedeemCodeInvariantBlock(t, body, invariant.name))
				safeComparison := regexp.MustCompile(
					`cast\(transaction_row\.balance_before_cents as decimal\(65,0\)\)\s*\+\s*` +
						`cast\(transaction_row\.amount_cents as decimal\(65,0\)\)\s*` + regexp.QuoteMeta(invariant.operator) + `\s*` +
						`cast\(transaction_row\.balance_after_cents as decimal\(65,0\)\)`,
				)
				if count := len(safeComparison.FindAllStringIndex(block, -1)); count != 1 {
					t.Errorf("%s invariant %s has %d overflow-safe balance comparisons, want 1", rel, invariant.name, count)
				}
				unsafeAddition := regexp.MustCompile(`transaction_row\.balance_before_cents\s*\+\s*transaction_row\.amount_cents`)
				if unsafeAddition.MatchString(block) {
					t.Errorf("%s invariant %s performs signed BIGINT addition before widening", rel, invariant.name)
				}
			}
		}
	})

	t.Run("schema reconciliation rejects weakened facts", func(t *testing.T) {
		verifySchema := readWalletRedeemCodeContractFile(t, root, "database", "reconciliation", "030_verify_schema.sql")

		columnShapes := walletRedeemCodeInvariantStatement(t, verifySchema, "redeem_code_column_shapes")
		seenDefaults := map[string]int{"note": 0, "created_at": 0, "updated_at": 0}
		for _, rawLine := range strings.Split(strings.ToLower(strings.ReplaceAll(columnShapes, "`", "")), "\n") {
			line := strings.TrimSpace(rawLine)
			switch {
			case strings.Contains(line, "column_name='note'"):
				seenDefaults["note"]++
				if !strings.Contains(line, "not (column_default <=> '')") {
					t.Errorf("redeem_code_column_shapes note default mismatch is not null-safe: %s", line)
				}
			case strings.Contains(line, "column_name='created_at'"):
				seenDefaults["created_at"]++
				if !strings.Contains(line, "not (upper(column_default) <=> 'current_timestamp(6)')") {
					t.Errorf("redeem_code_column_shapes created_at default mismatch is not null-safe: %s", line)
				}
			case strings.Contains(line, "column_name='updated_at'"):
				seenDefaults["updated_at"]++
				if !strings.Contains(line, "not (upper(column_default) <=> 'current_timestamp(6)')") {
					t.Errorf("redeem_code_column_shapes updated_at default mismatch is not null-safe: %s", line)
				}
			}
		}
		for column, want := range map[string]int{"note": 1, "created_at": 2, "updated_at": 2} {
			if got := seenDefaults[column]; got != want {
				t.Errorf("redeem_code_column_shapes %s default checks=%d want %d", column, got, want)
			}
		}

		checks := normalizeWalletRedeemCodeSQL(walletRedeemCodeInvariantStatement(t, verifySchema, "redeem_code_checks"))
		for _, forbidden := range []string{"locate(", "normalized_fragment"} {
			if strings.Contains(checks, forbidden) {
				t.Errorf("redeem_code_checks still accepts partial clauses via %q", forbidden)
			}
		}
		if !regexp.MustCompile(`if\(\s*count\(\*\)\s*=\s*5\s+and\s+sum\(`).MatchString(checks) ||
			!regexp.MustCompile(`\)\s*=\s*5\s*,\s*0\s*,\s*1\s*\)`).MatchString(checks) {
			t.Error("redeem_code_checks must require exactly five fully matching CHECK constraints")
		}
		canonicalChecks := []string{
			"actual.table_name='redeem_code_batches' and actual.constraint_name='chk_redeem_code_batches_amount_cents' and actual.normalized_clause='(amount_centsbetween1and100000000)'",
			"actual.table_name='redeem_code_batches' and actual.constraint_name='chk_redeem_code_batches_quantity' and actual.normalized_clause='(quantitybetween1and1000)'",
			"actual.table_name='redeem_code_batches' and actual.constraint_name='chk_redeem_code_batches_expiry' and actual.normalized_clause='((expires_atisnull)or(expires_at>created_at))'",
			"actual.table_name='redeem_codes' and actual.constraint_name='chk_redeem_codes_state' and actual.normalized_clause='(statein(''unused'',''used'',''voided''))'",
			"actual.table_name='redeem_codes' and actual.constraint_name='chk_redeem_codes_usage' and actual.normalized_clause='(((state=''used'')and(used_byisnotnull)and(used_atisnotnull))or((statein(''unused'',''voided''))and(used_byisnull)and(used_atisnull)))'",
		}
		for _, canonical := range canonicalChecks {
			if count := strings.Count(checks, canonical); count != 1 {
				t.Errorf("redeem_code_checks canonical mapping %q count=%d want 1", canonical, count)
			}
		}
	})
}

func readWalletRedeemCodeContractFile(t *testing.T, root string, path ...string) string {
	t.Helper()
	fullPath := filepath.Join(append([]string{root}, path...)...)
	body, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(path...), err)
	}
	return string(body)
}

func normalizeWalletRedeemCodeSQL(body string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.NewReplacer("`", "", "\r", " ", "\n", " ", "\t", " ").Replace(body)), " "))
}

func hclColumnBlock(t *testing.T, table string, name string) string {
	t.Helper()
	marker := `column "` + name + `" {`
	start := strings.Index(table, marker)
	if start < 0 {
		t.Fatalf("canonical table missing %s", marker)
	}
	depth := 0
	for index := start; index < len(table); index++ {
		switch table[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return table[start : index+1]
			}
		}
	}
	t.Fatalf("canonical column %q has unbalanced braces", name)
	return ""
}

func walletRedeemCodeInvariantBlock(t *testing.T, body string, name string) string {
	t.Helper()
	lowerBody := strings.ToLower(body)
	marker := "select '" + strings.ToLower(name) + "' as invariant"
	start := strings.Index(lowerBody, marker)
	if start < 0 {
		t.Fatalf("reconciliation SQL missing invariant %q", name)
	}
	tail := lowerBody[start+len(marker):]
	if next := strings.Index(tail, "select '"); next >= 0 {
		return body[start : start+len(marker)+next]
	}
	return body[start:]
}

func walletRedeemCodeInvariantStatement(t *testing.T, body string, name string) string {
	t.Helper()
	lowerBody := strings.ToLower(body)
	marker := "select '" + strings.ToLower(name) + "' as invariant"
	start := strings.Index(lowerBody, marker)
	if start < 0 {
		t.Fatalf("reconciliation SQL missing invariant %q", name)
	}
	end := strings.Index(lowerBody[start:], ";")
	if end < 0 {
		t.Fatalf("reconciliation invariant %q has no statement terminator", name)
	}
	return body[start : start+end+1]
}
