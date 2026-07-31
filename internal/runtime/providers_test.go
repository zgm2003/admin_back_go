package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/infra/secretkey"
	aichat "admin_back_go/internal/module/ai/chat"
	aitool "admin_back_go/internal/module/ai/tool"
)

func TestBuildProvidersExposesChatTransportCapabilities(t *testing.T) {
	ring, err := secretkey.NewKeyRing(strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	providers, err := BuildProviders(config.Config{}, ring)
	if err != nil {
		t.Fatalf("BuildProviders: %v", err)
	}
	if providers.AITransportCapabilities == nil {
		t.Fatal("chat transport capability resolver is nil")
	}
	capabilities, ok := providers.AITransportCapabilities.ResolveCapabilities(infraai.EngineTypeOpenAI)
	if !ok || len(capabilities.InputModalities) == 0 || !capabilities.SupportsStreaming {
		t.Fatalf("chat transport capabilities=%#v ok=%v", capabilities, ok)
	}
}

func TestBuildProvidersBuildsDedicatedMailDiagnosticBoxCurrentOnly(t *testing.T) {
	ring, err := secretkey.NewKeyRing(strings.Repeat("c", 64))
	if err != nil {
		t.Fatalf("current key ring: %v", err)
	}
	providers, err := BuildProviders(config.Config{}, ring)
	if err != nil {
		t.Fatalf("BuildProviders: %v", err)
	}
	box := requireMailDiagnosticBox(t, providers)
	if box.CurrentKeyID() != ring.MailDiagnosticKeyID() {
		t.Fatalf("diagnostic current key ID = %q, want %q", box.CurrentKeyID(), ring.MailDiagnosticKeyID())
	}
	keyID, ciphertext, err := box.Encrypt("123456")
	if err != nil || keyID != ring.MailDiagnosticKeyID() {
		t.Fatalf("diagnostic encrypt key=%q err=%v", keyID, err)
	}
	plain, err := box.Decrypt(keyID, ciphertext)
	if err != nil || plain != "123456" {
		t.Fatalf("diagnostic current-key decrypt plain=%q err=%v", plain, err)
	}
	otherRing, err := secretkey.NewKeyRing(strings.Repeat("x", 64))
	if err != nil {
		t.Fatalf("other key ring: %v", err)
	}
	otherCiphertext, err := secretbox.New(otherRing.MailDiagnosticKey()).Encrypt("654321")
	if err != nil {
		t.Fatalf("encrypt other diagnostic code: %v", err)
	}
	if _, err := box.Decrypt(otherRing.MailDiagnosticKeyID(), otherCiphertext); err == nil {
		t.Fatal("current-only diagnostic box retained an unconfigured previous key")
	}
}

func TestBuildProvidersBuildsDedicatedMailDiagnosticBoxCurrentPrevious(t *testing.T) {
	oldRing, err := secretkey.NewKeyRing(strings.Repeat("o", 64))
	if err != nil {
		t.Fatalf("old key ring: %v", err)
	}
	dualRing, err := secretkey.NewKeyRingWithPrevious(strings.Repeat("n", 64), []string{strings.Repeat("o", 64)})
	if err != nil {
		t.Fatalf("dual key ring: %v", err)
	}
	providers, err := BuildProviders(config.Config{}, dualRing)
	if err != nil {
		t.Fatalf("BuildProviders: %v", err)
	}
	box := requireMailDiagnosticBox(t, providers)
	oldCiphertext, err := secretbox.New(oldRing.MailDiagnosticKey()).Encrypt("654321")
	if err != nil {
		t.Fatalf("encrypt old diagnostic code: %v", err)
	}
	plain, err := box.Decrypt(oldRing.MailDiagnosticKeyID(), oldCiphertext)
	if err != nil || plain != "654321" {
		t.Fatalf("previous diagnostic decrypt plain=%q err=%v", plain, err)
	}
	if box.CurrentKeyID() != dualRing.MailDiagnosticKeyID() || box.CurrentKeyID() == oldRing.MailDiagnosticKeyID() {
		t.Fatalf("dual diagnostic box did not retain the new current key")
	}
}

func TestBuildProvidersBuildsDedicatedMailDiagnosticBoxWithPurposeSeparation(t *testing.T) {
	ring, err := secretkey.NewKeyRing(strings.Repeat("p", 64))
	if err != nil {
		t.Fatalf("key ring: %v", err)
	}
	providers, err := BuildProviders(config.Config{}, ring)
	if err != nil {
		t.Fatalf("BuildProviders: %v", err)
	}
	diagnosticBox := requireMailDiagnosticBox(t, providers)
	credentialCiphertext, err := providers.Secretbox.Encrypt("credential-secret")
	if err != nil {
		t.Fatalf("credential encrypt: %v", err)
	}
	if _, err := diagnosticBox.Decrypt(ring.MailDiagnosticKeyID(), credentialCiphertext); err == nil {
		t.Fatal("mail diagnostic box decrypted credential-purpose ciphertext")
	}
	_, diagnosticCiphertext, err := diagnosticBox.Encrypt("123456")
	if err != nil {
		t.Fatalf("diagnostic encrypt: %v", err)
	}
	if _, err := providers.Secretbox.Decrypt(diagnosticCiphertext); err == nil {
		t.Fatal("credential box decrypted mail-diagnostic-purpose ciphertext")
	}
}

func TestRuntimeMailDiagnosticProviderUsesNoSeparateEnvironmentSecret(t *testing.T) {
	for _, path := range []string{"providers.go", "api.go", "worker.go"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(body), "MAIL_DIAGNOSTIC_SECRET") {
			t.Fatalf("%s introduced a separate mail diagnostic root", path)
		}
	}
}

func requireMailDiagnosticBox(t *testing.T, providers Providers) secretbox.VersionedBox {
	t.Helper()
	field := reflect.ValueOf(providers).FieldByName("MailDiagnosticBox")
	if !field.IsValid() {
		t.Fatal("runtime Providers is missing MailDiagnosticBox")
	}
	box, ok := field.Interface().(secretbox.VersionedBox)
	if !ok {
		t.Fatalf("MailDiagnosticBox has type %T", field.Interface())
	}
	return box
}

func TestAIChatEngineFactorySupportsOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	engine, err := (aiChatEngineFactory{}).NewEngine(context.Background(), aichat.EngineConfig{
		EngineType: infraai.EngineTypeOpenAI,
		BaseURL:    server.URL,
		APIKey:     "sk-test",
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	result, err := engine.StreamChat(context.Background(), infraai.ChatInput{
		Content: "hi",
		Inputs:  map[string]any{"model_id": "gpt-test"},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if result.Answer != "ok" {
		t.Fatalf("answer = %q, want ok", result.Answer)
	}
}

func TestAIToolEngineFactoryUsesConfiguredResponsesProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s, want /v1/responses", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n"))
	}))
	defer server.Close()

	engine, err := (aiToolEngineFactory{}).NewEngine(context.Background(), aitool.EngineConfig{
		EngineType:  infraai.EngineTypeOpenAI,
		BaseURL:     server.URL,
		APIKey:      "sk-test",
		APIProtocol: infraai.APIProtocolResponses,
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}
	result, err := engine.StreamChat(context.Background(), infraai.ChatInput{
		Content: "hi",
		Inputs:  map[string]any{"model_id": "gpt-test"},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if result.Answer != "ok" {
		t.Fatalf("answer = %q, want ok", result.Answer)
	}
}

func TestAIProviderTesterSupportsOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-test"}]}`))
	}))
	defer server.Close()

	result, err := (aiConnectionTester{}).TestConnection(context.Background(), infraai.TestConnectionInput{
		EngineType: infraai.EngineTypeOpenAI,
		BaseURL:    server.URL,
		APIKey:     "sk-test",
		TimeoutMs:  int(time.Second / time.Millisecond),
	})
	if err != nil {
		t.Fatalf("TestConnection returned error: %v", err)
	}
	if result == nil || !result.OK {
		t.Fatalf("unexpected result: %#v", result)
	}
}
