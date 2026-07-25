package requestidentity

import (
	"errors"
	"testing"
)

func TestFingerprintIsStableForIdenticalTypedInput(t *testing.T) {
	input := Input{
		UserID:         42,
		Operation:      "generate",
		Modality:       "image",
		AgentID:        7,
		ModelID:        "image-v1",
		NormalizedText: "draw a cat",
		Attachments:    []AttachmentIdentity{{StorageProvider: "cos", StorageKey: "users/42/cat.png", SHA256: "abc"}},
		Options:        GenerationOptions{Size: "1024x1024", Count: 1, Extra: map[string]string{"quality": "high"}},
	}
	first, err := Fingerprint(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Fingerprint(input)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("identical input changed fingerprint: %x != %x", first, second)
	}
	if len(first) != 32 {
		t.Fatalf("fingerprint length=%d", len(first))
	}
}

func TestFingerprintChangesWithContentOrOptions(t *testing.T) {
	base := Input{UserID: 42, Operation: "generate", Modality: "text", AgentID: 7, ModelID: "m", NormalizedText: "hello", Options: GenerationOptions{MaxOutputTokens: 100}}
	first, err := Fingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	changedText := base
	changedText.NormalizedText = "hello!"
	second, _ := Fingerprint(changedText)
	if first == second {
		t.Fatal("content change must change fingerprint")
	}
	changedOptions := base
	changedOptions.Options.MaxOutputTokens = 101
	third, _ := Fingerprint(changedOptions)
	if first == third {
		t.Fatal("option change must change fingerprint")
	}
}

func TestCompareReplayRejectsDifferentFingerprint(t *testing.T) {
	want := [32]byte{1}
	if err := CompareForReplay(IdentityStatusReplayable, want, want); err != nil {
		t.Fatalf("equal fingerprint rejected: %v", err)
	}
	if err := CompareForReplay(IdentityStatusReplayable, want, [32]byte{2}); !errors.Is(err, ErrRequestIdentityConflict) {
		t.Fatalf("different fingerprint error=%v", err)
	}
}

func TestCompareReplayRejectsLegacyNonReplayableIdentity(t *testing.T) {
	legacyMarkerHash := [32]byte{1}
	if err := CompareForReplay(IdentityStatusLegacyNonReplayable, legacyMarkerHash, legacyMarkerHash); !errors.Is(err, ErrRequestIdentityNotReplayable) {
		t.Fatalf("legacy marker replay error=%v", err)
	}
}
