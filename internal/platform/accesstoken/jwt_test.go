package accesstoken

import (
	"strings"
	"testing"
	"time"
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
	if claims.SessionID != 42 || claims.UserID != 7 || claims.Platform != "admin" || claims.DeviceID != "device-a" {
		t.Fatalf("unexpected claims: %#v", claims)
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
