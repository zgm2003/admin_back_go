package database

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"admin_back_go/internal/telemetry"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const defaultSlowThreshold = 200 * time.Millisecond

type telemetryLogger struct {
	delegate      gormlogger.Interface
	recorder      telemetry.Recorder
	slowThreshold time.Duration
}

func newTelemetryLogger(delegate gormlogger.Interface, recorder telemetry.Recorder, slowThreshold time.Duration) gormlogger.Interface {
	if delegate == nil {
		delegate = gormlogger.Default
	}
	if recorder == nil {
		recorder = telemetry.Noop()
	}
	if slowThreshold <= 0 {
		slowThreshold = defaultSlowThreshold
	}
	return &telemetryLogger{delegate: delegate, recorder: recorder, slowThreshold: slowThreshold}
}

func (logger *telemetryLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	clone := *logger
	clone.delegate = logger.delegate.LogMode(level)
	return &clone
}

func (logger *telemetryLogger) Info(ctx context.Context, message string, data ...interface{}) {
	logger.delegate.Info(ctx, message, data...)
}

func (logger *telemetryLogger) Warn(ctx context.Context, message string, data ...interface{}) {
	logger.delegate.Warn(ctx, message, data...)
}

func (logger *telemetryLogger) Error(ctx context.Context, message string, data ...interface{}) {
	logger.delegate.Error(ctx, message, data...)
}

func (logger *telemetryLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), traceErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var (
		once      sync.Once
		statement string
		rows      int64
	)
	cached := func() (string, int64) {
		once.Do(func() {
			if fc != nil {
				statement, rows = fc()
			}
		})
		return statement, rows
	}
	statement, _ = cached()
	duration := time.Since(begin)
	operation, table := classifyStatement(statement)
	outcome := "ok"
	if traceErr != nil && !errors.Is(traceErr, gorm.ErrRecordNotFound) {
		outcome = "error"
	}
	attributes := telemetry.Attributes{
		"db.system":    "mysql",
		"db.operation": operation,
		"db.table":     table,
		"outcome":      outcome,
	}
	logger.recorder.Count("db.queries", 1, attributes)
	logger.recorder.Observe("db.duration_seconds", duration.Seconds(), attributes)
	if duration >= logger.slowThreshold {
		slow := cloneDatabaseAttributes(attributes)
		slow["db.slow_digest"] = telemetry.SlowDigest(statement)
		logger.recorder.Count("db.slow_queries", 1, slow)
	}
	logger.delegate.Trace(ctx, begin, cached, traceErr)
}

func classifyStatement(statement string) (string, string) {
	normalized := strings.NewReplacer("`", "", "\n", " ", "\r", " ", "\t", " ").Replace(statement)
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return "UNKNOWN", "unknown"
	}
	operation := strings.ToUpper(strings.Trim(fields[0], "();"))
	table := "unknown"
	switch operation {
	case "SELECT", "DELETE":
		table = tokenAfter(fields, "FROM")
	case "INSERT", "REPLACE":
		table = tokenAfter(fields, "INTO")
	case "UPDATE":
		if len(fields) > 1 {
			table = cleanTable(fields[1])
		}
	}
	return operation, table
}

func tokenAfter(fields []string, marker string) string {
	for index := 0; index+1 < len(fields); index++ {
		if strings.EqualFold(strings.Trim(fields[index], "();"), marker) {
			return cleanTable(fields[index+1])
		}
	}
	return "unknown"
}

func cleanTable(value string) string {
	value = strings.Trim(value, " ,();")
	if separator := strings.LastIndex(value, "."); separator >= 0 {
		value = value[separator+1:]
	}
	if value == "" {
		return "unknown"
	}
	return value
}

func cloneDatabaseAttributes(attributes telemetry.Attributes) telemetry.Attributes {
	clone := make(telemetry.Attributes, len(attributes)+1)
	for key, value := range attributes {
		clone[key] = value
	}
	return clone
}

var _ gormlogger.Interface = (*telemetryLogger)(nil)
