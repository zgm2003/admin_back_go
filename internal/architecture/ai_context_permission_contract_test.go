package architecture

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestAIContextPermissionMigrationContract(t *testing.T) {
	migration := mustReadRepoFile(t, "database/migrations/202608020102_ai_context_permissions.sql")
	normalized := normalizeAIContextPermissionSQL(migration)
	for _, required := range []string{
		"start transaction",
		"create temporary table _ai_context_affected_roles",
		"create temporary table _ai_context_view_roles",
		"create temporary table _ai_context_manage_roles",
		"create temporary table _ai_context_document_roles",
		"create temporary table _ai_context_evaluate_roles",
		"create temporary table _ai_context_profile_roles",
		"create temporary table _ai_context_principal_versions_before",
		"group by role_permission.role_id having count(distinct permission.id) = 4",
		"group by role_permission.role_id having count(distinct permission.id) = 5",
		"create temporary table _ai_context_permission_guard",
		"check (violations = 0)",
		"permission_id in (122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 413, 415)",
		"insert into role_permissions",
		"on duplicate key update is_del = 2",
		"delete from role_permissions",
		"delete from permissions",
		"insert into authz_principal_versions",
		"set principal_version.version = principal_version.version + 1",
		"commit",
	} {
		if !strings.Contains(normalized, required) {
			t.Errorf("context permission migration missing %q", required)
		}
	}

	for _, exact := range []string{
		"having count(distinct permission.id) = 4",
		"having count(distinct permission.id) = 5",
		"permission.id in (128, 129, 130, 131)",
		"permission.id in (124, 125, 126, 127, 415)",
		"permission.id = 123",
		"permission.id = 413",
		"permission.id in (122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 415)",
	} {
		if strings.Count(normalized, exact) < 1 {
			t.Errorf("context permission migration missing mapping predicate %q", exact)
		}
	}
	if strings.Contains(normalized, "insert into _ai_context_view_roles (role_id) select role_id from _ai_context_affected_roles") {
		t.Fatal("binding-only role 413 must not receive the view permission")
	}

	guardAt := strings.Index(normalized, "create temporary table _ai_context_permission_guard")
	writeAt := firstAIContextWriteIndex(normalized)
	if guardAt < 0 || writeAt < 0 || guardAt > writeAt {
		t.Fatalf("partial-grant guard must be created before persistent writes: guard=%d write=%d", guardAt, writeAt)
	}
	if strings.Contains(normalized, "drop table permissions") || strings.Contains(normalized, "truncate table permissions") {
		t.Fatal("permission migration must not replace the permissions table")
	}
}

func TestAIContextPermissionSeedHasExactFinalIdentity(t *testing.T) {
	seed := mustReadAIContextPermissionFile(t, "database/seeds/admin_permissions.sql")

	expected := map[string]string{
		"923": "ai_context_view",
		"924": "ai_context_manage",
		"925": "ai_context_document_manage",
		"926": "ai_context_profile_manage",
		"927": "ai_context_evaluate",
	}
	for id, code := range expected {
		pattern := regexp.MustCompile(`(?m)^\(` + id + `,.*'` + regexp.QuoteMeta(code) + `'`)
		if got := len(pattern.FindAllString(seed, -1)); got != 1 {
			t.Errorf("seed permission %s/%s count=%d want 1", id, code, got)
		}
	}
	menu := "(122, '上下文工程', '/ai/context', 'Collection', 5, 'ai/context', 'admin', 2, 3, NULL, 'menu.ai_context', 1, 1, 2)"
	if strings.Count(seed, menu) != 1 {
		t.Fatalf("seed must contain final context menu exactly once")
	}
	for _, id := range []string{"123", "124", "125", "126", "127", "128", "129", "130", "131", "413", "415"} {
		if regexp.MustCompile(`(?m)^\(` + id + `,`).MatchString(seed) {
			t.Errorf("retired permission id %s remains in final seed", id)
		}
	}
}

func TestAIContextPermissionMigrationUsesTransactionalVersionSnapshot(t *testing.T) {
	normalized := normalizeAIContextPermissionSQL(mustReadRepoFile(t, "database/migrations/202608020102_ai_context_permissions.sql"))
	for _, required := range []string{
		"left join authz_principal_versions",
		"version_before",
		"updated_at_before",
		"where principal_version.platform = 'admin'",
		"affected = 1",
		"affected = 0",
		"coalesce(snapshot.version_before, 1) + 1",
		"not (current_version.version <=> snapshot.version_before)",
	} {
		if !strings.Contains(normalized, required) {
			t.Errorf("version snapshot contract missing %q", required)
		}
	}
}

func TestAIContextPermissionRoleFixtures(t *testing.T) {
	fixtures := []struct {
		name   string
		grants []int
		want   []int
		reject bool
	}{
		{name: "full legacy role", grants: []int{122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 413, 415}, want: []int{923, 924, 925, 926, 927}},
		{name: "menu only", grants: []int{122}, want: []int{923}},
		{name: "partial base manage", grants: []int{128}, reject: true},
		{name: "partial document manage", grants: []int{124}, reject: true},
		{name: "binding without base manage", grants: []int{413}, reject: true},
		{name: "unaffected role", grants: []int{}, want: []int{}},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			got, rejected := evaluateAIContextRoleFixture(fixture.grants)
			if rejected != fixture.reject {
				t.Fatalf("rejected=%v want %v", rejected, fixture.reject)
			}
			if fixture.reject {
				return
			}
			if !reflect.DeepEqual(got, fixture.want) {
				t.Fatalf("mapped permissions=%v want %v", got, fixture.want)
			}
		})
	}
}

func evaluateAIContextRoleFixture(grants []int) ([]int, bool) {
	has := make(map[int]bool, len(grants))
	for _, grant := range grants {
		has[grant] = true
	}
	baseCount := 0
	for _, grant := range []int{128, 129, 130, 131} {
		if has[grant] {
			baseCount++
		}
	}
	documentCount := 0
	for _, grant := range []int{124, 125, 126, 127, 415} {
		if has[grant] {
			documentCount++
		}
	}
	if (baseCount > 0 && baseCount < 4) || (documentCount > 0 && documentCount < 5) || (has[413] && baseCount != 4) {
		return nil, true
	}
	result := make([]int, 0, 5)
	for _, grant := range []int{122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 415} {
		if has[grant] {
			result = append(result, 923)
			break
		}
	}
	if baseCount == 4 {
		result = append(result, 924)
	}
	if documentCount == 5 {
		result = append(result, 925)
	}
	if baseCount == 4 && has[413] {
		result = append(result, 926)
	}
	if has[123] {
		result = append(result, 927)
	}
	return result, false
}

func firstAIContextWriteIndex(sql string) int {
	indices := []int{
		strings.Index(sql, "update permissions"),
		strings.Index(sql, "insert into permissions"),
		strings.Index(sql, "insert into role_permissions"),
		strings.Index(sql, "delete from role_permissions"),
	}
	first := -1
	for _, index := range indices {
		if index >= 0 && (first < 0 || index < first) {
			first = index
		}
	}
	return first
}

func normalizeAIContextPermissionSQL(body string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.NewReplacer("`", "", "\r", " ", "\n", " ", "\t", " ").Replace(body)), " "))
}

func mustReadAIContextPermissionFile(t *testing.T, relativePath string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(backendRoot(t), filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
