package databaseevolution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStageFilesOrdersAndVerifiesChecksums(t *testing.T) {
	dir := t.TempDir()
	second := []byte("SELECT 2;\n")
	first := []byte("SELECT 1;\n")
	if err := os.WriteFile(filepath.Join(dir, "020_b.sql"), second, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "010_a.sql"), first, 0o600); err != nil {
		t.Fatal(err)
	}

	files, err := LoadStageFiles(dir, map[string]string{
		"010_a.sql": SHA256(first),
		"020_b.sql": SHA256(second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Name != "010_a.sql" || files[1].Name != "020_b.sql" {
		t.Fatalf("files=%v", files)
	}
	if files[0].SHA256 != SHA256(first) || string(files[0].SQL) != string(first) {
		t.Fatalf("first=%+v", files[0])
	}
}

func TestLoadStageFilesRejectsChecksumAndManifestDrift(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "010_a.sql"), []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		expected map[string]string
		want     string
	}{
		{name: "checksum", expected: map[string]string{"010_a.sql": strings.Repeat("0", 64)}, want: "checksum mismatch"},
		{name: "missing", expected: map[string]string{"010_a.sql": SHA256([]byte("SELECT 1;\n")), "020_b.sql": SHA256([]byte("SELECT 2;\n"))}, want: "missing stage file"},
		{name: "unexpected", expected: map[string]string{}, want: "unexpected stage file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadStageFiles(dir, tt.expected)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v want %q", err, tt.want)
			}
		})
	}
}
