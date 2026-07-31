package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadRuleExtensionNormalizationMigration(t *testing.T) {
	root := backendRoot(t)
	migrationPath := filepath.Join(root, "database", "migrations", "202607310002_upload_rule_extension_normalization.sql")
	body, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read upload rule extension normalization migration: %v", err)
	}
	migration := strings.ToLower(strings.ReplaceAll(string(body), "\r\n", "\n"))

	for _, required := range []string{
		"update `upload_rule`",
		"json_remove(`image_exts`, json_unquote(json_search(`image_exts`, 'one', 'doc')))",
		"when json_search(`file_exts`, 'one', 'doc') is null",
		"json_array_append(`file_exts`, '$', 'doc')",
		"where json_search(`image_exts`, 'one', 'doc') is not null",
	} {
		if !strings.Contains(migration, required) {
			t.Errorf("upload rule extension normalization migration missing %q", required)
		}
	}

	for _, forbidden := range []string{"'png'", "'avif'", "'pdf'", "'docx'", "'md'"} {
		if strings.Contains(migration, forbidden) {
			t.Errorf("migration must not broaden existing upload rule authorization with %s", forbidden)
		}
	}
}
