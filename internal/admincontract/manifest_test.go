package admincontract

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestManifestHashesEveryArtifact(t *testing.T) {
	bundle := mustBuildBundle(t)
	if bundle.Manifest.BundleVersion != BundleVersion || bundle.Manifest.BackendCommit != testBackendCommit {
		t.Fatalf("manifest identity=%#v", bundle.Manifest)
	}
	if len(bundle.Manifest.Artifacts) != len(bundle.Artifacts) {
		t.Fatalf("manifest artifacts=%d content artifacts=%d", len(bundle.Manifest.Artifacts), len(bundle.Artifacts))
	}
	for name, data := range bundle.Artifacts {
		artifact, exists := bundle.Manifest.Artifacts[name]
		if !exists {
			t.Fatalf("manifest missing %s", name)
		}
		hash := sha256.Sum256(data)
		if artifact.SHA256 != hex.EncodeToString(hash[:]) {
			t.Fatalf("bad hash for %s", name)
		}
		if artifact.SchemaVersion == "" {
			t.Fatalf("missing schema version for %s", name)
		}
	}
}

func TestBuildRequiresExplicitFullBackendCommit(t *testing.T) {
	for _, commit := range []string{"", "HEAD", "0123456", "ABCDEF0123456789ABCDEF0123456789ABCDEF01"} {
		if _, err := Build(BuildOptions{BackendCommit: commit}); err == nil {
			t.Fatalf("commit %q unexpectedly accepted", commit)
		}
	}
}
