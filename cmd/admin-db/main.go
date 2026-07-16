package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"admin_back_go/internal/databaseevolution"

	_ "github.com/go-sql-driver/mysql"
)

type commandDependencies struct {
	getenv        func(string) string
	openDatabase  func(string) (*sql.DB, error)
	capture       func(context.Context, *sql.DB, string) (databaseevolution.Fingerprint, error)
	write         func(string, databaseevolution.FingerprintDocument) error
	runInvariants func(context.Context, *sql.DB, string) (databaseevolution.InvariantResult, error)
	stdout        io.Writer
}

type fingerprintOptions struct {
	schema    string
	output    string
	gitCommit string
}

type invariantOptions struct {
	schema string
	file   string
}

type commandError struct {
	operation string
	cause     error
}

func (err *commandError) Error() string {
	switch {
	case errors.Is(err.cause, context.Canceled):
		return err.operation + ": canceled"
	case errors.Is(err.cause, context.DeadlineExceeded):
		return err.operation + ": deadline exceeded"
	default:
		return err.operation + ": failed"
	}
}

func (err *commandError) Unwrap() error {
	return err.cause
}

type singleStringFlag struct {
	name      string
	value     *string
	set       bool
	duplicate bool
}

func (value *singleStringFlag) String() string {
	if value == nil || value.value == nil {
		return ""
	}
	return *value.value
}

func (value *singleStringFlag) Set(input string) error {
	if value.set {
		value.duplicate = true
		return fmt.Errorf("duplicate flag")
	}
	value.set = true
	*value.value = input
	return nil
}

func main() {
	dependencies := commandDependencies{
		getenv:        os.Getenv,
		openDatabase:  func(dsn string) (*sql.DB, error) { return sql.Open("mysql", dsn) },
		capture:       databaseevolution.Capture,
		write:         databaseevolution.WriteFingerprintDocument,
		runInvariants: databaseevolution.RunInvariantFile,
		stdout:        os.Stdout,
	}
	if err := run(context.Background(), os.Args[1:], dependencies); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, dependencies commandDependencies) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: admin-db <fingerprint|invariants> [arguments]")
	}
	switch args[0] {
	case "fingerprint":
		options, err := parseFingerprintOptions(args[1:])
		if err != nil {
			return err
		}
		return runFingerprint(ctx, options, dependencies)
	case "invariants":
		options, err := parseInvariantOptions(args[1:])
		if err != nil {
			return err
		}
		return runInvariants(ctx, options, dependencies)
	default:
		return fmt.Errorf("unsupported subcommand")
	}
}

func parseFingerprintOptions(args []string) (fingerprintOptions, error) {
	flags := flag.NewFlagSet("fingerprint", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options fingerprintOptions
	schemaFlag := &singleStringFlag{name: "schema", value: &options.schema}
	outputFlag := &singleStringFlag{name: "out", value: &options.output}
	commitFlag := &singleStringFlag{name: "commit", value: &options.gitCommit}
	flags.Var(schemaFlag, "schema", "schema to fingerprint")
	flags.Var(outputFlag, "out", "output JSON path")
	flags.Var(commitFlag, "commit", "Git commit")
	if err := flags.Parse(args); err != nil {
		for _, value := range []*singleStringFlag{schemaFlag, outputFlag, commitFlag} {
			if value.duplicate {
				return fingerprintOptions{}, fmt.Errorf("--%s may be provided only once", value.name)
			}
		}
		return fingerprintOptions{}, fmt.Errorf("invalid fingerprint arguments")
	}
	if flags.NArg() != 0 {
		return fingerprintOptions{}, fmt.Errorf("unexpected argument")
	}
	options.schema = strings.TrimSpace(options.schema)
	options.output = strings.TrimSpace(options.output)
	options.gitCommit = strings.TrimSpace(options.gitCommit)
	if options.schema == "" {
		return fingerprintOptions{}, fmt.Errorf("--schema is required")
	}
	if options.output == "" {
		return fingerprintOptions{}, fmt.Errorf("--out is required")
	}
	if options.gitCommit == "" {
		return fingerprintOptions{}, fmt.Errorf("--commit is required")
	}
	if !isFullGitObjectID(options.gitCommit) {
		return fingerprintOptions{}, fmt.Errorf("--commit must be a full Git object ID")
	}
	return options, nil
}

func parseInvariantOptions(args []string) (invariantOptions, error) {
	flags := flag.NewFlagSet("invariants", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options invariantOptions
	schemaFlag := &singleStringFlag{name: "schema", value: &options.schema}
	fileFlag := &singleStringFlag{name: "file", value: &options.file}
	flags.Var(schemaFlag, "schema", "schema to verify")
	flags.Var(fileFlag, "file", "invariant SQL file")
	if err := flags.Parse(args); err != nil {
		for _, value := range []*singleStringFlag{schemaFlag, fileFlag} {
			if value.duplicate {
				return invariantOptions{}, fmt.Errorf("--%s may be provided only once", value.name)
			}
		}
		return invariantOptions{}, fmt.Errorf("invalid invariants arguments")
	}
	if flags.NArg() != 0 {
		return invariantOptions{}, fmt.Errorf("unexpected argument")
	}
	options.schema = strings.TrimSpace(options.schema)
	options.file = strings.TrimSpace(options.file)
	if options.schema == "" {
		return invariantOptions{}, fmt.Errorf("--schema is required")
	}
	if options.file == "" {
		return invariantOptions{}, fmt.Errorf("--file is required")
	}
	return options, nil
}

func isFullGitObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func runFingerprint(ctx context.Context, options fingerprintOptions, dependencies commandDependencies) error {
	if dependencies.getenv == nil || dependencies.openDatabase == nil || dependencies.capture == nil || dependencies.write == nil || dependencies.stdout == nil {
		return fmt.Errorf("fingerprint command dependencies are incomplete")
	}
	dsn := dependencies.getenv("MYSQL_DSN")
	if err := databaseevolution.ValidateSchemaDSN(dsn, options.schema); err != nil {
		return err
	}
	database, err := dependencies.openDatabase(dsn)
	if err != nil {
		return safeCommandError("open MySQL connection", err)
	}
	defer database.Close()

	fingerprint, err := dependencies.capture(ctx, database, options.schema)
	if err != nil {
		return safeCommandError("capture schema fingerprint", err)
	}
	document, err := databaseevolution.NewFingerprintDocument(options.gitCommit, fingerprint)
	if err != nil {
		return err
	}
	if err := dependencies.write(options.output, document); err != nil {
		return safeCommandError("write fingerprint document", err)
	}
	if _, err := fmt.Fprintln(dependencies.stdout, options.output); err != nil {
		return fmt.Errorf("print fingerprint output path: %w", err)
	}
	if _, err := fmt.Fprintln(dependencies.stdout, document.SchemaSHA256); err != nil {
		return fmt.Errorf("print fingerprint schema SHA-256: %w", err)
	}
	return nil
}

func runInvariants(ctx context.Context, options invariantOptions, dependencies commandDependencies) error {
	if dependencies.getenv == nil || dependencies.openDatabase == nil || dependencies.runInvariants == nil || dependencies.stdout == nil {
		return fmt.Errorf("invariants command dependencies are incomplete")
	}
	dsn := dependencies.getenv("MYSQL_DSN")
	if err := databaseevolution.ValidateSchemaDSN(dsn, options.schema); err != nil {
		return err
	}
	database, err := dependencies.openDatabase(dsn)
	if err != nil {
		return safeCommandError("open MySQL connection", err)
	}
	defer database.Close()

	result, runErr := dependencies.runInvariants(ctx, database, options.file)
	for _, check := range result.Checks {
		if _, err := fmt.Fprintf(dependencies.stdout, "%s\t%d\n", check.Name, check.Violations); err != nil {
			return fmt.Errorf("print invariant result: %w", err)
		}
	}
	if runErr != nil {
		return safeCommandError("run database invariants", runErr)
	}
	return nil
}

func safeCommandError(operation string, err error) error {
	return &commandError{operation: operation, cause: err}
}
