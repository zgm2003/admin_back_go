package config

import (
	"reflect"
	"strings"
	"testing"
)

func mapLookup(values map[string]string) lookupEnv {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestEnvTextUsesFallbackForBlankValues(t *testing.T) {
	for name, value := range map[string]string{
		"empty":      "",
		"whitespace": " \t\r\n ",
	} {
		t.Run(name, func(t *testing.T) {
			lookup := func(key string) (string, bool) {
				if key == "STRUCTURAL_TEXT" {
					return value, true
				}
				return "", false
			}

			if got := envText(lookup, "STRUCTURAL_TEXT", "fallback"); got != "fallback" {
				t.Fatalf("envText() = %q, want fallback", got)
			}
		})
	}
}

func TestLoadPreservesOpaqueCredentialValues(t *testing.T) {
	const (
		appSecret     = "  app-secret  "
		redisPassword = "\tredis-password \r\n"
		dbPassword    = " db-password\t"
	)
	cfg, err := loadFrom(mapLookup(map[string]string{
		"APP_SECRET":     appSecret,
		"REDIS_PASSWORD": redisPassword,
		"DB_HOST":        "127.0.0.1",
		"DB_DATABASE":    "admin",
		"DB_USERNAME":    "admin_user",
		"DB_PASSWORD":    dbPassword,
	}))
	if err != nil {
		t.Fatalf("loadFrom(): %v", err)
	}

	if cfg.App.Secret != appSecret {
		t.Errorf("APP_SECRET = %q, want byte-preserved %q", cfg.App.Secret, appSecret)
	}
	if cfg.Redis.Password != redisPassword {
		t.Errorf("REDIS_PASSWORD = %q, want byte-preserved %q", cfg.Redis.Password, redisPassword)
	}
	wantDSN := "admin_user:" + dbPassword + "@tcp(127.0.0.1:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local"
	if cfg.MySQL.DSN != wantDSN {
		t.Errorf("legacy DB_PASSWORD produced DSN %q, want %q", cfg.MySQL.DSN, wantDSN)
	}
}

func TestEnvOpaqueUsesFallbackOnlyForMissingOrEmptyValues(t *testing.T) {
	if got := envOpaque(mapLookup(nil), "CREDENTIAL", "fallback"); got != "fallback" {
		t.Fatalf("missing credential = %q, want fallback", got)
	}
	if got := envOpaque(mapLookup(map[string]string{"CREDENTIAL": ""}), "CREDENTIAL", "fallback"); got != "fallback" {
		t.Fatalf("empty credential = %q, want fallback", got)
	}
	if got := envOpaque(mapLookup(map[string]string{"CREDENTIAL": "  "}), "CREDENTIAL", "fallback"); got != "  " {
		t.Fatalf("whitespace credential = %q, want byte-preserved whitespace", got)
	}
}

func TestEnvListReturnsFallbackCopy(t *testing.T) {
	fallback := []string{"first", "second"}
	got := envList(mapLookup(nil), "LIST", fallback)
	if !reflect.DeepEqual(got, fallback) {
		t.Fatalf("envList() = %#v, want %#v", got, fallback)
	}
	got[0] = "changed"
	if fallback[0] != "first" {
		t.Fatalf("envList() returned aliased fallback: %#v", fallback)
	}
}

func TestLoadRejectsMalformedEnvironment(t *testing.T) {
	tests := []struct {
		key, value, want string
	}{
		{"MYSQL_MAX_OPEN_CONNS", "many", "MYSQL_MAX_OPEN_CONNS: parse integer"},
		{"MYSQL_MAX_OPEN_CONNS", "0", "MYSQL_MAX_OPEN_CONNS: must be greater than zero"},
		{"MYSQL_MAX_IDLE_CONNS", "-1", "MYSQL_MAX_IDLE_CONNS: must not be negative"},
		{"REDIS_DB", "-1", "REDIS_DB: must not be negative"},
		{"REDIS_DB", "99999999999999999999999999999999999999", "REDIS_DB: parse integer"},
		{"TOKEN_REDIS_DB", "-1", "TOKEN_REDIS_DB: must not be negative"},
		{"QUEUE_REDIS_DB", "-1", "QUEUE_REDIS_DB: must not be negative"},
		{"REALTIME_REDIS_DB", "-1", "REALTIME_REDIS_DB: must not be negative"},
		{"MYSQL_CONN_MAX_LIFETIME", "tomorrow", "MYSQL_CONN_MAX_LIFETIME: parse duration"},
		{"QUEUE_ENABLED", "sometimes", "QUEUE_ENABLED: parse boolean"},
		{"REALTIME_ENABLED", "sometimes", "REALTIME_ENABLED: parse boolean"},
		{"SCHEDULER_ENABLED", "sometimes", "SCHEDULER_ENABLED: parse boolean"},
		{"QUEUE_CONCURRENCY", "0", "QUEUE_CONCURRENCY: must be greater than zero"},
		{"HTTP_READ_HEADER_TIMEOUT", "-1s", "HTTP_READ_HEADER_TIMEOUT: must be greater than zero"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			_, err := loadFrom(mapLookup(map[string]string{tt.key: tt.value}))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("loadFrom() error=%v, want substring %q", err, tt.want)
			}
		})
	}
}
