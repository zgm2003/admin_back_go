package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

const maxAttributeValueLength = 160

var allowedAttributeKeys = map[string]struct{}{
	"http.method": {}, "http.route": {}, "http.status": {}, "http.status_code": {},
	"error.code": {}, "error.type": {}, "outcome": {}, "retryable": {},
	"db.system": {}, "db.operation": {}, "db.table": {}, "db.slow_digest": {},
	"redis.operation": {},
	"queue.type":      {}, "queue.lane": {}, "queue.outcome": {}, "queue.retry": {}, "queue.lease_expired": {}, "queue.exhausted": {},
	"provider.name": {}, "provider.modality": {}, "provider.status": {},
	"realtime.operation": {}, "realtime.transport": {}, "realtime.outcome": {},
	"scheduler.operation": {}, "scheduler.outcome": {}, "scheduler.lease_owned": {},
}

var sensitiveAttributeFragments = []string{
	"authorization", "cookie", "password", "secret", "token", "certificate",
	"prompt", "payload", "body", "query", "dsn", "bind", "session", "user_id",
	"request_id", "trace_id", "task_id", "run_id", "conversation_id",
}

func SanitizeAttributes(attributes Attributes) Attributes {
	sanitized := make(Attributes)
	for key, value := range attributes {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" || sensitiveAttributeKey(key) {
			continue
		}
		if _, allowed := allowedAttributeKeys[key]; !allowed {
			continue
		}
		text, ok := boundedAttributeValue(key, value)
		if !ok || text == "" {
			continue
		}
		sanitized[key] = text
	}
	return sanitized
}

// SlowDigest returns a short, deterministic digest suitable for telemetry.
// Callers use it before handing SQL to a Recorder so even custom Recorder
// implementations never receive the statement or its bound values.
func SlowDigest(statement string) string {
	statement = strings.TrimSpace(statement)
	if len(statement) == 16 {
		if _, err := hex.DecodeString(statement); err == nil {
			return strings.ToLower(statement)
		}
	}
	sum := sha256.Sum256([]byte(statement))
	return hex.EncodeToString(sum[:8])
}

func sensitiveAttributeKey(key string) bool {
	for _, fragment := range sensitiveAttributeFragments {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func boundedAttributeValue(key string, value any) (string, bool) {
	var text string
	switch typed := value.(type) {
	case string:
		text = strings.TrimSpace(typed)
	case fmt.Stringer:
		text = strings.TrimSpace(typed.String())
	case bool:
		text = strconv.FormatBool(typed)
	case int:
		text = strconv.Itoa(typed)
	case int8:
		text = strconv.FormatInt(int64(typed), 10)
	case int16:
		text = strconv.FormatInt(int64(typed), 10)
	case int32:
		text = strconv.FormatInt(int64(typed), 10)
	case int64:
		text = strconv.FormatInt(typed, 10)
	case uint:
		text = strconv.FormatUint(uint64(typed), 10)
	case uint8:
		text = strconv.FormatUint(uint64(typed), 10)
	case uint16:
		text = strconv.FormatUint(uint64(typed), 10)
	case uint32:
		text = strconv.FormatUint(uint64(typed), 10)
	case uint64:
		text = strconv.FormatUint(typed, 10)
	case float32:
		text = strconv.FormatFloat(float64(typed), 'g', -1, 32)
	case float64:
		text = strconv.FormatFloat(typed, 'g', -1, 64)
	default:
		return "", false
	}

	switch key {
	case "http.method", "db.operation":
		text = strings.ToUpper(text)
	case "http.route":
		text = sanitizeRoute(text)
	case "db.slow_digest":
		text = SlowDigest(text)
	default:
		text = strings.ToLower(text)
	}
	if len(text) > maxAttributeValueLength {
		text = text[:maxAttributeValueLength]
	}
	return text, text != ""
}

func sanitizeRoute(route string) string {
	if parsed, err := url.Parse(route); err == nil && parsed.Path != "" {
		route = parsed.Path
	} else if separator := strings.IndexAny(route, "?#"); separator >= 0 {
		route = route[:separator]
	}
	parts := strings.Split(route, "/")
	for index, part := range parts {
		if numericSegment(part) || uuidLikeSegment(part) {
			parts[index] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func numericSegment(segment string) bool {
	if segment == "" {
		return false
	}
	for _, character := range segment {
		if !unicode.IsDigit(character) {
			return false
		}
	}
	return true
}

func uuidLikeSegment(segment string) bool {
	if len(segment) != 36 {
		return false
	}
	for index, character := range segment {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if !unicode.IsDigit(character) && (unicode.ToLower(character) < 'a' || unicode.ToLower(character) > 'f') {
			return false
		}
	}
	return true
}
