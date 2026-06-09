package ai

import "testing"

func TestNormalizeOpenAIBaseURLAppendsV1ForOriginOnlyURL(t *testing.T) {
	got, err := NormalizeOpenAIBaseURL(" http://host.docker.internal:8317/ ", "")
	if err != nil {
		t.Fatalf("NormalizeOpenAIBaseURL error = %v", err)
	}
	if got != "http://host.docker.internal:8317/v1" {
		t.Fatalf("base url = %q, want origin plus /v1", got)
	}
}

func TestNormalizeOpenAIBaseURLKeepsVersionedAndNestedPaths(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com/v1/":        "https://api.openai.com/v1",
		"http://proxy.example.test/openai/": "http://proxy.example.test/openai",
	}
	for input, want := range cases {
		got, err := NormalizeOpenAIBaseURL(input, "")
		if err != nil {
			t.Fatalf("NormalizeOpenAIBaseURL(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeOpenAIBaseURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeOpenAIBaseURLUsesFallback(t *testing.T) {
	got, err := NormalizeOpenAIBaseURL("", "https://api.openai.com/v1")
	if err != nil {
		t.Fatalf("NormalizeOpenAIBaseURL error = %v", err)
	}
	if got != "https://api.openai.com/v1" {
		t.Fatalf("base url = %q, want fallback", got)
	}
}
