package config

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

func Validate(process Process, cfg Config) error {
	if process != ProcessAPI && process != ProcessWorker {
		return fmt.Errorf("process is unsupported")
	}
	if err := validateAppEnvironment(cfg.App.Env); err != nil {
		return err
	}
	if err := ValidateRuntimeSecrets(cfg); err != nil {
		return err
	}
	if err := validateHTTPAddress(process, cfg.HTTP.Addr); err != nil {
		return err
	}
	production := strings.EqualFold(strings.TrimSpace(cfg.App.Env), "production")
	if err := validateMySQLConfig(cfg.MySQL, production); err != nil {
		return err
	}
	if err := validateRedisConfig(cfg.Redis, production); err != nil {
		return err
	}
	if err := validateQdrantConfig(cfg.Qdrant, production); err != nil {
		return err
	}
	if cfg.Token.RedisDB < 0 {
		return fmt.Errorf("TOKEN_REDIS_DB must not be negative")
	}
	if cfg.Queue.RedisDB < 0 {
		return fmt.Errorf("QUEUE_REDIS_DB must not be negative")
	}
	if cfg.Realtime.RedisDB < 0 {
		return fmt.Errorf("REALTIME_REDIS_DB must not be negative")
	}
	if process == ProcessWorker && !cfg.Queue.Enabled {
		return fmt.Errorf("QUEUE_ENABLED must be true for admin-worker")
	}
	if cfg.Queue.Concurrency <= 0 {
		return fmt.Errorf("QUEUE_CONCURRENCY must be positive")
	}
	if err := validateSchedulerConfig(cfg.Scheduler, cfg.Queue.Enabled); err != nil {
		return err
	}
	if err := validateRealtimeConfig(cfg.Realtime, production); err != nil {
		return err
	}
	if process == ProcessAPI {
		if err := validateCORSConfig(cfg.CORS, production); err != nil {
			return err
		}
	}
	if err := validatePaymentConfig(cfg.Payment); err != nil {
		return err
	}

	return nil
}

func validateAppEnvironment(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "local", "development", "test", "staging", "production":
		return nil
	default:
		return fmt.Errorf("APP_ENV must be local, development, test, staging, or production")
	}
}

func validateHTTPAddress(process Process, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		if process == ProcessWorker {
			return nil
		}
		return fmt.Errorf("HTTP_ADDR is required for admin-api")
	}
	if err := validateHostPort(value, false); err != nil {
		return fmt.Errorf("HTTP_ADDR must be a valid host:port with a numeric port between 1 and 65535")
	}
	return nil
}

func validateHostPort(value string, requireHost bool) error {
	host, portText, err := net.SplitHostPort(value)
	if err != nil || strings.TrimSpace(portText) == "" {
		return fmt.Errorf("invalid host port")
	}
	if requireHost && strings.TrimSpace(host) == "" {
		return fmt.Errorf("host is required")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port")
	}
	return nil
}

func validateMySQLConfig(cfg MySQLConfig, production bool) error {
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		return fmt.Errorf("MYSQL_DSN is required")
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("MYSQL_DSN is malformed")
	}
	if strings.TrimSpace(parsed.DBName) == "" {
		return fmt.Errorf("MYSQL_DSN must include a database name")
	}
	if cfg.MaxOpenConns <= 0 {
		return fmt.Errorf("MYSQL_MAX_OPEN_CONNS must be positive")
	}
	if cfg.MaxIdleConns < 0 || cfg.MaxIdleConns > cfg.MaxOpenConns {
		return fmt.Errorf("MYSQL_MAX_IDLE_CONNS must be between 0 and MYSQL_MAX_OPEN_CONNS")
	}
	if cfg.ConnMaxLifetime <= 0 {
		return fmt.Errorf("MYSQL_CONN_MAX_LIFETIME must be positive")
	}
	if !production {
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(parsed.Net)) {
	case "tcp", "tcp4", "tcp6":
	default:
		return fmt.Errorf("MYSQL_DSN must use a non-local TCP dependency in production")
	}
	if err := validateHostPort(parsed.Addr, true); err != nil {
		return fmt.Errorf("MYSQL_DSN must contain a valid TCP host and port in production")
	}
	host, _, _ := net.SplitHostPort(parsed.Addr)
	if isLocalOrUnusableDependencyHost(host) {
		return fmt.Errorf("MYSQL_DSN must not use a local or unusable host in production")
	}
	return nil
}

func validateRedisConfig(cfg RedisConfig, production bool) error {
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		return fmt.Errorf("REDIS_ADDR is required")
	}
	if err := validateHostPort(addr, true); err != nil {
		return fmt.Errorf("REDIS_ADDR must be a valid host:port with a numeric port between 1 and 65535")
	}
	if cfg.DB < 0 {
		return fmt.Errorf("REDIS_DB must not be negative")
	}
	if production {
		host, _, _ := net.SplitHostPort(addr)
		if isLocalOrUnusableDependencyHost(host) {
			return fmt.Errorf("REDIS_ADDR must not use a local or unusable host in production")
		}
	}
	return nil
}

func validateQdrantConfig(cfg QdrantConfig, production bool) error {
	addr := strings.TrimSpace(cfg.Addr)
	if err := validateHostPort(addr, true); err != nil || strings.ContainsAny(addr, "@/?#") {
		return fmt.Errorf("QDRANT_ADDR must be a credential-free host:port with a numeric port between 1 and 65535")
	}
	prefix := strings.TrimSpace(cfg.CollectionPrefix)
	if prefix == "" || len(prefix) > 191 {
		return fmt.Errorf("QDRANT_COLLECTION_PREFIX must contain 1 to 191 lowercase ASCII characters")
	}
	for index, char := range prefix {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || index > 0 && char == '_' {
			continue
		}
		return fmt.Errorf("QDRANT_COLLECTION_PREFIX must start with lowercase ASCII and contain only lowercase ASCII, digits, and underscores")
	}
	if prefix[0] < 'a' || prefix[0] > 'z' {
		return fmt.Errorf("QDRANT_COLLECTION_PREFIX must start with lowercase ASCII")
	}
	if production && !cfg.TLS {
		return fmt.Errorf("QDRANT_TLS must be true in production")
	}
	if production && strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("QDRANT_API_KEY is required in production")
	}
	return nil
}

func validateSchedulerConfig(cfg SchedulerConfig, queueEnabled bool) error {
	if !cfg.Enabled {
		return nil
	}
	if !queueEnabled {
		return fmt.Errorf("SCHEDULER_ENABLED requires QUEUE_ENABLED")
	}
	timezone := strings.TrimSpace(cfg.Timezone)
	if timezone == "" {
		return fmt.Errorf("SCHEDULER_TIMEZONE must name a valid IANA timezone")
	}
	if strings.EqualFold(timezone, "Local") {
		return fmt.Errorf("SCHEDULER_TIMEZONE must name a valid IANA timezone")
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return fmt.Errorf("SCHEDULER_TIMEZONE must name a valid IANA timezone")
	}
	return nil
}

func validateRealtimeConfig(cfg RealtimeConfig, production bool) error {
	publisher := strings.TrimSpace(cfg.Publisher)
	switch publisher {
	case RealtimePublisherLocal, RealtimePublisherNoop, RealtimePublisherRedis:
	default:
		return fmt.Errorf("REALTIME_PUBLISHER must be local, noop, or redis")
	}
	if production && cfg.Enabled && publisher != RealtimePublisherRedis {
		return fmt.Errorf("REALTIME_PUBLISHER must be redis when realtime is enabled in production")
	}
	return nil
}

func validateCORSConfig(cfg CORSConfig, production bool) error {
	if len(cfg.AllowOrigins) == 0 {
		return fmt.Errorf("CORS_ALLOW_ORIGINS must contain at least one origin for admin-api")
	}
	for _, value := range cfg.AllowOrigins {
		rawOrigin := strings.TrimSpace(value)
		if strings.Contains(rawOrigin, "#") {
			return fmt.Errorf("CORS_ALLOW_ORIGINS must not contain a fragment")
		}
		origin, err := url.Parse(rawOrigin)
		if err != nil || !origin.IsAbs() || origin.Opaque != "" || origin.Host == "" || origin.Hostname() == "" {
			return fmt.Errorf("CORS_ALLOW_ORIGINS must contain only absolute HTTP or HTTPS origins")
		}
		if !strings.EqualFold(origin.Scheme, "http") && !strings.EqualFold(origin.Scheme, "https") {
			return fmt.Errorf("CORS_ALLOW_ORIGINS must contain only HTTP or HTTPS origins")
		}
		if strings.HasSuffix(origin.Host, ":") {
			return fmt.Errorf("CORS_ALLOW_ORIGINS must not contain an empty port")
		}
		if portText := origin.Port(); portText != "" {
			port, err := strconv.Atoi(portText)
			if err != nil || port < 1 || port > 65535 {
				return fmt.Errorf("CORS_ALLOW_ORIGINS port must be between 1 and 65535")
			}
		}
		if origin.User != nil || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" || origin.RawFragment != "" {
			return fmt.Errorf("CORS_ALLOW_ORIGINS must not contain user info, a query, or a fragment")
		}
		if origin.Path != "" && origin.Path != "/" {
			return fmt.Errorf("CORS_ALLOW_ORIGINS must not contain a non-root path")
		}
		if production && !strings.EqualFold(origin.Scheme, "https") {
			return fmt.Errorf("CORS_ALLOW_ORIGINS must use HTTPS in production")
		}
		if production && isLocalOrPrivateHost(origin.Hostname()) {
			return fmt.Errorf("CORS_ALLOW_ORIGINS must not use a local or private host in production")
		}
	}
	return nil
}

func validatePaymentConfig(cfg PaymentConfig) error {
	if strings.TrimSpace(cfg.CertBaseDir) == "" {
		return nil
	}
	normalized := filepath.FromSlash(cfg.CertBaseDir)
	cleaned := filepath.Clean(normalized)
	if !filepath.IsAbs(cleaned) {
		return fmt.Errorf("PAYMENT_CERT_BASE_DIR must be absolute")
	}
	if cleaned != normalized {
		return fmt.Errorf("PAYMENT_CERT_BASE_DIR must be clean")
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return fmt.Errorf("PAYMENT_CERT_BASE_DIR must be an existing directory")
	}
	if !info.IsDir() {
		return fmt.Errorf("PAYMENT_CERT_BASE_DIR must be a directory")
	}
	return nil
}

type hostClassification struct {
	localName bool
	zoned     bool
	addr      netip.Addr
}

func classifyHost(host string) hostClassification {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") {
		return hostClassification{localName: true}
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return hostClassification{}
	}
	classification := hostClassification{
		zoned: addr.Zone() != "",
		addr:  addr.WithZone("").Unmap(),
	}
	return classification
}

func (classification hostClassification) isLocalOrUnusableDependency() bool {
	if classification.localName || classification.zoned {
		return true
	}
	if !classification.addr.IsValid() {
		return false
	}
	return classification.addr.IsLoopback() ||
		classification.addr.IsUnspecified() ||
		classification.addr.IsLinkLocalUnicast() ||
		classification.addr.IsMulticast()
}

func isLocalOrUnusableDependencyHost(host string) bool {
	return classifyHost(host).isLocalOrUnusableDependency()
}

func isLocalOrPrivateHost(host string) bool {
	classification := classifyHost(host)
	return classification.isLocalOrUnusableDependency() ||
		classification.addr.IsValid() && classification.addr.IsPrivate()
}
