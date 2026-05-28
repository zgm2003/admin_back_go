package i18n

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
)

const legacyMessagePrefix = "legacy."

// FallbackMessageID returns the deterministic catalog key used for legacy
// app errors that still carry only a fallback message.
func FallbackMessageID(fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return ""
	}
	sum := sha1.Sum([]byte(fallback))
	return legacyMessagePrefix + hex.EncodeToString(sum[:])[:12]
}
