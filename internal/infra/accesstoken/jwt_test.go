package accesstoken

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTCodecIssueParseRoundTrip(t *testing.T) {
	codec := NewJWTCodec([]byte("12345678901234567890123456789012"), Options{Issuer: "admin_go"})
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	token, err := codec.Issue(Claims{
		SessionID: 42,
		UserID:    7,
		Platform:  "admin",
		DeviceID:  "device-a",
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if strings.Count(token, ".") != 2 {
		t.Fatalf("expected JWT access token, got %q", token)
	}
	claims, err := codec.Parse(token, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if claims.SessionID != 42 || claims.UserID != 7 || claims.Issuer != "admin_go" || claims.Platform != "admin" || claims.DeviceID != "device-a" || !claims.NotBefore.Equal(now) {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

func TestJWTCodecRejectsFutureIssuedAt(t *testing.T) {
	codec := NewJWTCodec([]byte("12345678901234567890123456789012"), Options{Issuer: "admin_go"})
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	token, err := codec.Issue(Claims{SessionID: 42, UserID: 7, Platform: "admin", IssuedAt: now.Add(time.Minute), NotBefore: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if _, err = codec.Parse(token, now); err == nil {
		t.Fatalf("expected future issued-at to fail")
	}
}

func TestJWTCodecRejectsExpiredToken(t *testing.T) {
	codec := NewJWTCodec([]byte("12345678901234567890123456789012"), Options{Issuer: "admin_go"})
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	token, err := codec.Issue(Claims{SessionID: 42, UserID: 7, Platform: "admin", IssuedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if _, err = codec.Parse(token, now); err == nil {
		t.Fatalf("expected expired token error")
	}
}

func TestJWTCodecRejectsTamperedToken(t *testing.T) {
	codec := NewJWTCodec([]byte("12345678901234567890123456789012"), Options{Issuer: "admin_go"})
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	token, err := codec.Issue(Claims{SessionID: 42, UserID: 7, Platform: "admin", IssuedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	parts := strings.Split(token, ".")
	parts[2] = "tampered"
	if _, err = codec.Parse(strings.Join(parts, "."), now.Add(time.Minute)); err == nil {
		t.Fatalf("expected tampered token to fail")
	}
}

func TestJWTCodecRejectsWrongSigningKey(t *testing.T) {
	issuer := NewJWTCodec([]byte("12345678901234567890123456789012"), Options{Issuer: "admin_go"})
	parser := NewJWTCodec([]byte("abcdefghijklmnopqrstuvwxyzzzzzzz"), Options{Issuer: "admin_go"})
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	token, err := issuer.Issue(Claims{SessionID: 42, UserID: 7, Platform: "admin", IssuedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if _, err = parser.Parse(token, now.Add(time.Minute)); err == nil {
		t.Fatalf("expected wrong signing key to fail")
	}
}

func TestJWTCodecIssuesExplicitKeyID(t *testing.T) {
	codec := NewJWTCodec([]byte("12345678901234567890123456789012"), Options{Issuer: "admin_go"})
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	token, err := codec.Issue(Claims{SessionID: 42, UserID: 7, Platform: "admin", IssuedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	parts := strings.Split(token, ".")
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode JWT header: %v", err)
	}
	var header map[string]any
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatalf("decode JWT header JSON: %v", err)
	}
	kid, ok := header["kid"].(string)
	if !ok || strings.TrimSpace(kid) == "" {
		t.Fatalf("JWT header has no explicit kid: %#v", header)
	}
}

func TestRotatingJWTCodecVerifiesPreviousKeyAndSignsWithCurrent(t *testing.T) {
	oldKey := []byte("old-key-123456789012345678901234")
	newKey := []byte("new-key-123456789012345678901234")
	oldCodec, err := NewRotatingJWTCodec("old", map[string][]byte{"old": oldKey}, Options{Issuer: "admin_go"})
	if err != nil {
		t.Fatalf("create old codec: %v", err)
	}
	dualCodec, err := NewRotatingJWTCodec("new", map[string][]byte{"new": newKey, "old": oldKey}, Options{Issuer: "admin_go"})
	if err != nil {
		t.Fatalf("create dual codec: %v", err)
	}
	newOnlyCodec, err := NewRotatingJWTCodec("new", map[string][]byte{"new": newKey}, Options{Issuer: "admin_go"})
	if err != nil {
		t.Fatalf("create new-only codec: %v", err)
	}
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	claims := Claims{SessionID: 42, UserID: 7, Platform: "admin", IssuedAt: now, ExpiresAt: now.Add(time.Hour)}
	oldToken, err := oldCodec.Issue(claims)
	if err != nil {
		t.Fatalf("issue old token: %v", err)
	}
	if _, err := dualCodec.Parse(oldToken, now.Add(time.Minute)); err != nil {
		t.Fatalf("dual codec rejected previous token: %v", err)
	}
	if _, err := newOnlyCodec.Parse(oldToken, now.Add(time.Minute)); err == nil {
		t.Fatal("new-only codec accepted previous token")
	}

	newToken, err := dualCodec.Issue(claims)
	if err != nil {
		t.Fatalf("issue current token: %v", err)
	}
	if _, err := newOnlyCodec.Parse(newToken, now.Add(time.Minute)); err != nil {
		t.Fatalf("new-only codec rejected current token: %v", err)
	}
}

func issueRawJWTForKeyIDTest(t *testing.T, key []byte, keyID string, now time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": "admin_go", "sub": "7", "iat": now.Unix(), "nbf": now.Unix(),
		"exp": now.Add(time.Hour).Unix(), "sid": int64(42), "platform": "admin", "device_id": "",
	})
	if keyID != "" {
		token.Header["kid"] = keyID
	}
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign raw JWT: %v", err)
	}
	return signed
}

func TestRotatingJWTCodecRejectsMissingOrUnknownKeyID(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	codec, err := NewRotatingJWTCodec("current", map[string][]byte{"current": key}, Options{Issuer: "admin_go"})
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	legacy := issueRawJWTForKeyIDTest(t, key, "", now)
	unknown := issueRawJWTForKeyIDTest(t, key, "unknown", now)
	for name, token := range map[string]string{"missing": legacy, "unknown": unknown} {
		t.Run(name, func(t *testing.T) {
			if _, err := codec.Parse(token, now.Add(time.Minute)); err == nil {
				t.Fatal("expected key ID rejection")
			}
		})
	}
}
