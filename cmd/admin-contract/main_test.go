package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const testCommit = "0123456789abcdef0123456789abcdef01234567"

func TestRunRequiresExplicitCommit(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"generate", "-out", t.TempDir()}, &stdout, &stderr); code == 0 {
		t.Fatalf("generate without commit succeeded: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

func TestRunGeneratesAndChecksExactBundle(t *testing.T) {
	output := filepath.Join(t.TempDir(), "admin", "v1")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"generate", "-out", output, "-commit", testCommit}, &stdout, &stderr); code != 0 {
		t.Fatalf("generate code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"check", "-out", output, "-commit", testCommit}, &stdout, &stderr); code != 0 {
		t.Fatalf("check code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	path := filepath.Join(output, "views.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read views: %v", err)
	}
	data[0] ^= 1
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("tamper views: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"generate", "-check", "-out", output, "-commit", testCommit}, &stdout, &stderr); code == 0 {
		t.Fatalf("check mode accepted drift: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}
