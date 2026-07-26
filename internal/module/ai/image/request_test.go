package aiimage

import (
	"testing"

	"admin_back_go/internal/module/ai/requestidentity"
)

func TestRequestIdentityInputFromSnapshotMatchesAcceptedFingerprint(t *testing.T) {
	snapshot := ProviderInputSnapshot{
		Version: imageInputSnapshotVersion, Operation: "image.generate", Modality: "image", Model: RequiredModelID,
		Prompt: "draw", Size: "1024x1024", Quality: "auto", OutputFormat: "png", Moderation: "auto", N: 1,
		MaxOutputTokens: 32768,
		Attachments: []AttachmentSnapshot{{
			Role: FileRoleInput, SortOrder: 1, StorageProvider: StorageProviderCOS, StorageKey: "immutable/input.png",
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", MimeType: "image/png", SizeBytes: 7,
		}},
	}
	want, err := imageRequestFingerprint(7, 5, snapshot)
	if err != nil {
		t.Fatalf("accepted fingerprint: %v", err)
	}

	identity, err := RequestIdentityInput(7, 5, snapshot)
	if err != nil {
		t.Fatalf("RequestIdentityInput: %v", err)
	}
	got, err := requestidentity.Fingerprint(identity)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if got != want {
		t.Fatalf("runtime identity fingerprint=%x accepted=%x", got, want)
	}
}
