package enum

import (
	"reflect"
	"testing"
)

func TestUploadDriverMembership(t *testing.T) {
	if !IsUploadDriver(UploadDriverCOS) || !IsUploadDriver(UploadDriverOSS) {
		t.Fatalf("expected cos and oss to be valid upload drivers")
	}
	if IsUploadDriver("s3") {
		t.Fatalf("expected s3 to be invalid in current upload driver enum")
	}
}

func TestNormalizeUploadExtsTrimsLowercasesDedupesAndKeepsEnumOrder(t *testing.T) {
	got, err := NormalizeUploadExts(
		[]string{" PNG ", "jpg", "png", "JPEG"},
		IsUploadImageExt,
		UploadImageExts,
	)
	if err != nil {
		t.Fatalf("NormalizeUploadExts returned error: %v", err)
	}
	want := []string{"jpeg", "jpg", "png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestNormalizeUploadExtsRejectsUnknownValue(t *testing.T) {
	_, err := NormalizeUploadExts([]string{"png", "exe"}, IsUploadImageExt, UploadImageExts)
	if err == nil {
		t.Fatalf("expected unknown extension to be rejected")
	}
}
