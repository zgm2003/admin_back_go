package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/database"
	contextengine "admin_back_go/internal/module/ai/contextengine"
)

var errCutoverViolations = errors.New("AI context cutover preflight found violations")

type commandDependencies struct {
	getenv func(string) string
	open   func(config.MySQLConfig) (*database.Client, error)
	check  func(context.Context, contextengine.CutoverPreflightRepository) (contextengine.CutoverReport, error)
	stdout io.Writer
}

func main() {
	err := run(context.Background(), os.Args[1:], commandDependencies{
		getenv: os.Getenv,
		open:   func(cfg config.MySQLConfig) (*database.Client, error) { return database.Open(cfg) },
		check:  contextengine.RunCutoverPreflight,
		stdout: os.Stdout,
	})
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "AI context cutover preflight failed")
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, dependencies commandDependencies) error {
	if len(args) != 0 {
		return errors.New("ai-context-preflight accepts no arguments")
	}
	if dependencies.getenv == nil || dependencies.open == nil || dependencies.check == nil || dependencies.stdout == nil {
		return errors.New("preflight command dependencies are incomplete")
	}
	dsn := strings.TrimSpace(dependencies.getenv("MYSQL_DSN"))
	if dsn == "" {
		return errors.New("MYSQL_DSN is required")
	}
	client, err := dependencies.open(config.MySQLConfig{DSN: dsn, MaxOpenConns: 1, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
	if err != nil {
		return errors.New("open preflight database")
	}
	defer client.Close()
	if client.Gorm == nil || client.SQL == nil {
		return errors.New("preflight database is incomplete")
	}
	if err := client.Gorm.WithContext(ctx).Exec("SET SESSION TRANSACTION READ ONLY").Error; err != nil {
		return errors.New("make preflight database read-only")
	}
	report, err := dependencies.check(ctx, contextengine.NewGormCutoverPreflightRepository(client.Gorm))
	if err != nil {
		return errors.New("run AI context cutover preflight")
	}
	encoder := json.NewEncoder(dependencies.stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		return errors.New("write AI context cutover preflight report")
	}
	if len(report.Violations) != 0 {
		return errCutoverViolations
	}
	return nil
}
