package enum

import (
	"reflect"
	"testing"
)

func TestUploadDriverMembership(t *testing.T) {
	if !IsUploadDriver(UploadDriverCOS) {
		t.Fatalf("cos must be the only supported upload driver")
	}
	if IsUploadDriver("oss") || IsUploadDriver("s3") || IsUploadDriver("") {
		t.Fatalf("only cos should be accepted as an upload driver")
	}
	if len(UploadDrivers) != 1 || UploadDrivers[0] != UploadDriverCOS {
		t.Fatalf("upload drivers must be COS-only, got %#v", UploadDrivers)
	}
	if len(UploadDriverLabels) != 1 || UploadDriverLabels[UploadDriverCOS] != "腾讯云 COS" {
		t.Fatalf("upload driver labels must expose COS only, got %#v", UploadDriverLabels)
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
