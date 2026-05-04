package logstore

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStoreListFilesScansOnlyAllowedLogFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "admin-api.log"), "INFO api\n")
	writeFile(t, filepath.Join(root, "notes.txt"), "no\n")
	writeFile(t, filepath.Join(root, "worker", "admin-worker.log"), "ERROR worker\n")
	writeFile(t, filepath.Join(root, "worker", "secret.txt"), "no\n")
	writeFile(t, filepath.Join(root, "deep", "child", "ignored.log"), "no\n")

	store := New(root, Options{AllowedExtensions: []string{".log"}, MaxTailLines: 500})
	got, err := store.ListFiles(context.Background())
	if err != nil {
		t.Fatalf("ListFiles returned error: %v", err)
	}

	names := make([]string, 0, len(got))
	for _, item := range got {
		names = append(names, item.Name)
		if item.Size <= 0 || item.SizeHuman == "" || item.MTime == "" {
			t.Fatalf("file metadata not populated: %#v", item)
		}
	}
	want := []string{"admin-api.log", "worker/admin-worker.log"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("expected allowed first-level log files %#v, got %#v", want, names)
	}
}

func TestStoreTailRejectsPathTraversalAndUnknownExtension(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "admin-api.log"), "INFO ok\n")
	writeFile(t, filepath.Join(root, "secret.txt"), "secret\n")

	store := New(root, Options{AllowedExtensions: []string{".log"}, MaxTailLines: 500})
	cases := []string{"../secret.log", "..\\secret.log", "/admin-api.log", "secret.txt", "missing.log"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := store.Tail(context.Background(), TailQuery{Name: name, Lines: 100})
			if err == nil {
				t.Fatalf("expected error for %q", name)
			}
		})
	}
}

func TestStoreTailLimitsFiltersAndParsesLevels(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "admin-api.log"), "DEBUG boot\nINFO ready\nWARNING slow client\nERROR db timeout\nINFO done\n")

	store := New(root, Options{AllowedExtensions: []string{".log"}, MaxTailLines: 3})
	got, err := store.Tail(context.Background(), TailQuery{Name: "admin-api.log", Lines: 100, Level: "ERROR", Keyword: "db"})
	if err != nil {
		t.Fatalf("Tail returned error: %v", err)
	}

	if got.Filename != "admin-api.log" || got.Total != 1 {
		t.Fatalf("unexpected tail response: %#v", got)
	}
	if len(got.Lines) != 1 || got.Lines[0].Content != "ERROR db timeout" || got.Lines[0].Level != "ERROR" || got.Lines[0].Number != 4 {
		t.Fatalf("unexpected filtered line: %#v", got.Lines)
	}

	latest, err := store.Tail(context.Background(), TailQuery{Name: "admin-api.log", Lines: 100})
	if err != nil {
		t.Fatalf("Tail latest returned error: %v", err)
	}
	if len(latest.Lines) != 3 || latest.Lines[0].Content != "WARNING slow client" || latest.Lines[2].Content != "INFO done" {
		t.Fatalf("expected max tail lines to cap to latest three lines, got %#v", latest.Lines)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
