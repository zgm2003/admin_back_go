package cos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestObjectInspectorProvesGIFIsStaticFromConditionalObjectContent(t *testing.T) {
	for _, frames := range []int{1, 2} {
		t.Run(fmt.Sprintf("frames_%d", frames), func(t *testing.T) {
			body := encodedGIF(t, frames)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "image/gif")
				writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
				writer.Header().Set("ETag", `"gif-v1"`)
				switch request.Method {
				case http.MethodHead:
					writer.WriteHeader(http.StatusOK)
				case http.MethodGet:
					if request.Header.Get("If-Match") != `"gif-v1"` {
						t.Fatalf("If-Match=%q", request.Header.Get("If-Match"))
					}
					_, _ = writer.Write(body)
				default:
					t.Fatalf("method=%s", request.Method)
				}
			}))
			t.Cleanup(server.Close)
			inspector := NewObjectInspector(&staticObjectConfigProvider{config: ObjectConfig{
				SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-1", Region: "ap-test", Endpoint: server.URL,
			}}, ObjectInspectorConfig{Enabled: true, Timeout: time.Second, HTTPClient: server.Client()})

			metadata, err := inspector.Head(context.Background(), "ai_chat_attachments/2026/07/demo.gif")
			if frames == 1 {
				if err != nil || !metadata.GIFStaticVerified {
					t.Fatalf("metadata=%#v err=%v", metadata, err)
				}
			} else if !errors.Is(err, ErrAnimatedGIF) {
				t.Fatalf("animated GIF error=%v", err)
			}
		})
	}
}

func TestStaticGIFProofRejectsTrailingData(t *testing.T) {
	body := append(encodedGIF(t, 1), 0x00)

	if err := requireStaticGIF(bytes.NewReader(body)); !errors.Is(err, ErrInvalidGIF) {
		t.Fatalf("trailing GIF data error=%v", err)
	}
}

func encodedGIF(t *testing.T, frames int) []byte {
	t.Helper()
	images := make([]*image.Paletted, frames)
	delays := make([]int, frames)
	palette := color.Palette{color.Black, color.White}
	for index := range images {
		images[index] = image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
		images[index].Pix[0] = uint8(index % len(palette))
	}
	var body bytes.Buffer
	if err := gif.EncodeAll(&body, &gif.GIF{Image: images, Delay: delays}); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

type staticObjectConfigProvider struct {
	config ObjectConfig
	err    error
	calls  int
}

func (provider *staticObjectConfigProvider) ActiveObjectConfig(context.Context) (ObjectConfig, error) {
	provider.calls++
	return provider.config, provider.err
}

func TestObjectInspectorUsesHeadMetadataAndTrustedKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodHead || request.URL.Path != "/ai_chat_images/2026/07/28/demo.png" {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "image/png; charset=binary")
		writer.Header().Set("Content-Length", "321")
		writer.Header().Set("ETag", `"image-v1"`)
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	provider := &staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-1", Region: "ap-test", Endpoint: server.URL,
	}}
	inspector := NewObjectInspector(provider, ObjectInspectorConfig{
		Enabled: true, Timeout: time.Second, HTTPClient: server.Client(),
	})

	metadata, err := inspector.Head(context.Background(), "ai_chat_images/2026/07/28/demo.png")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if metadata.Key != "ai_chat_images/2026/07/28/demo.png" || metadata.MIMEType != "image/png" || metadata.Size != 321 || metadata.ETag != `"image-v1"` ||
		metadata.TrustedURL != server.URL+"/ai_chat_images/2026/07/28/demo.png" {
		t.Fatalf("metadata=%#v", metadata)
	}
	if provider.calls != 1 {
		t.Fatalf("config provider calls=%d", provider.calls)
	}
}

func TestTrustedAIChatObjectKeySeparatesLegacyImagesFromNewFiles(t *testing.T) {
	tests := []struct {
		key, typ string
		wantOK   bool
	}{
		{"ai_chat_images/2026/07/old.jpg", "image", true},
		{"ai_chat_images/2026/07/old.pdf", "file", false},
		{"ai_chat_attachments/2026/07/new.jpg", "image", true},
		{"ai_chat_attachments/2026/07/report.pdf", "file", true},
		{"ai_chat_attachments/../secret.pdf", "file", false},
		{"exports/report.pdf", "file", false},
	}
	for _, test := range tests {
		t.Run(test.typ+"/"+test.key, func(t *testing.T) {
			_, err := TrustedAIChatObjectKey(test.key, test.typ)
			if (err == nil) != test.wantOK {
				t.Fatalf("key=%q type=%q err=%v", test.key, test.typ, err)
			}
		})
	}
}

func TestObjectInspectorRejectsMissingETag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/pdf")
		writer.Header().Set("Content-Length", "321")
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	inspector := NewObjectInspector(&staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-1", Region: "ap-test", Endpoint: server.URL,
	}}, ObjectInspectorConfig{Enabled: true, Timeout: time.Second, HTTPClient: server.Client()})

	_, err := inspector.Head(context.Background(), "ai_chat_attachments/2026/07/report.pdf")
	if !errors.Is(err, ErrInvalidObjectMetadata) {
		t.Fatalf("missing ETag error=%v", err)
	}
}

func TestObjectInspectorRejectsUntrustedKeyBeforeConfigLookup(t *testing.T) {
	provider := &staticObjectConfigProvider{}
	inspector := NewObjectInspector(provider, ObjectInspectorConfig{Enabled: true})

	_, err := inspector.Head(context.Background(), "images/demo.png")
	if !errors.Is(err, ErrUntrustedObjectKey) {
		t.Fatalf("untrusted key error=%v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("untrusted key reached config provider: calls=%d", provider.calls)
	}
}
