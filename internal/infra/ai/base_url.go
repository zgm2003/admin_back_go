package ai

import (
	"fmt"
	"net/url"
	"strings"
)

func NormalizeOpenAIBaseURL(value string, fallback string) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(value), "/")
	if raw == "" {
		raw = strings.TrimRight(strings.TrimSpace(fallback), "/")
	}
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: invalid OpenAI base url", ErrInvalidConfig)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: invalid OpenAI base url scheme", ErrInvalidConfig)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: invalid OpenAI base url", ErrInvalidConfig)
	}
	if parsed.EscapedPath() == "" || parsed.EscapedPath() == "/" {
		return raw + "/v1", nil
	}
	return raw, nil
}
