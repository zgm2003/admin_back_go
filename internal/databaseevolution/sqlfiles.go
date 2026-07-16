package databaseevolution

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type StageFile struct {
	Name   string
	Path   string
	SHA256 string
	SQL    []byte
}

func SHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func LoadStageFiles(directory string, expected map[string]string) ([]StageFile, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read stage directory: %w", err)
	}

	actual := make(map[string]os.DirEntry)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".sql") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("stage file %q must not be a symlink", entry.Name())
		}
		actual[entry.Name()] = entry
	}
	for name := range actual {
		if _, ok := expected[name]; !ok {
			return nil, fmt.Errorf("unexpected stage file %q", name)
		}
	}
	for name := range expected {
		if filepath.Base(name) != name || !strings.HasSuffix(name, ".sql") {
			return nil, fmt.Errorf("invalid stage file name %q", name)
		}
		if _, ok := actual[name]; !ok {
			return nil, fmt.Errorf("missing stage file %q", name)
		}
	}

	names := make([]string, 0, len(expected))
	for name := range expected {
		names = append(names, name)
	}
	sort.Strings(names)
	files := make([]StageFile, 0, len(names))
	for _, name := range names {
		want := expected[name]
		if len(want) != 64 || strings.ToLower(want) != want {
			return nil, fmt.Errorf("invalid checksum for %q", name)
		}
		if _, err := hex.DecodeString(want); err != nil {
			return nil, fmt.Errorf("invalid checksum for %q", name)
		}
		path := filepath.Join(directory, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read stage file %q: %w", name, err)
		}
		got := SHA256(data)
		if got != want {
			return nil, fmt.Errorf("checksum mismatch for %q", name)
		}
		files = append(files, StageFile{Name: name, Path: path, SHA256: got, SQL: data})
	}
	return files, nil
}
