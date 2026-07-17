package telemetry

import "testing"

func TestSanitizeAttributesDropsSensitiveAndHighCardinalityValues(t *testing.T) {
	got := SanitizeAttributes(Attributes{
		"authorization": "Bearer secret",
		"prompt":        "private prompt",
		"http.method":   "GET",
		"http.route":    "/api/admin/v1/users/:id?token=secret",
		"user_id":       int64(99),
		"request_id":    "req-unique",
		"db.binds":      []any{"secret"},
	})
	for _, forbidden := range []string{"authorization", "prompt", "user_id", "request_id", "db.binds"} {
		if _, exists := got[forbidden]; exists {
			t.Fatalf("%s retained: %#v", forbidden, got)
		}
	}
	if got["http.method"] != "GET" {
		t.Fatalf("safe method missing: %#v", got)
	}
	if got["http.route"] != "/api/admin/v1/users/:id" {
		t.Fatalf("route was not bounded: %#v", got)
	}
}

func TestSanitizeAttributesBoundsValuesAndHashesSlowDigest(t *testing.T) {
	got := SanitizeAttributes(Attributes{
		"db.operation":   " SELECT ",
		"db.table":       "users",
		"db.slow_digest": "SELECT * FROM users WHERE email='private@example.com'",
		"http.status":    503,
		"retryable":      true,
	})
	if got["db.operation"] != "SELECT" || got["db.table"] != "users" {
		t.Fatalf("safe database labels missing: %#v", got)
	}
	digest, ok := got["db.slow_digest"].(string)
	if !ok || len(digest) != 16 || digest == "SELECT * FROM users WHERE email='private@example.com'" {
		t.Fatalf("slow digest was not hashed: %#v", got)
	}
	if got["http.status"] != "503" || got["retryable"] != "true" {
		t.Fatalf("bounded scalar values missing: %#v", got)
	}
}
