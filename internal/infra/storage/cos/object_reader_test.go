package cos

import (
	"context"
	"errors"
	"testing"
)

func TestObjectReaderRejectsDisabledAndInvalidConfig(t *testing.T) {
	_, err := NewObjectReader(ObjectReaderConfig{}).Get(context.Background(), GetInput{})
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled reader error = %v, want ErrDisabled", err)
	}

	_, err = NewObjectReader(ObjectReaderConfig{Enabled: true}).Get(context.Background(), GetInput{
		SecretID:  "sid",
		SecretKey: "skey",
		Bucket:    "bucket-123",
		Region:    "ap-guangzhou",
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid reader config error = %v, want ErrInvalidConfig", err)
	}
}

func TestNormalizeGetInputTrimsKey(t *testing.T) {
	got := normalizeGetInput(GetInput{
		SecretID:  " sid ",
		SecretKey: " skey ",
		Bucket:    " bucket-123 ",
		Region:    " ap-guangzhou ",
		Key:       " /folder/a.png ",
		Endpoint:  " https://cos.example.test ",
	})
	if got.SecretID != "sid" || got.SecretKey != "skey" || got.Bucket != "bucket-123" || got.Region != "ap-guangzhou" || got.Key != "folder/a.png" || got.Endpoint != "https://cos.example.test" {
		t.Fatalf("unexpected normalized input: %#v", got)
	}
}
