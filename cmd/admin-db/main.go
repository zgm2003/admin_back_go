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
	getenv              func(string) string
	openDatabase        func(string) (*sql.DB, error)
	capture             func(context.Context, *sql.DB, string) (databaseevolution.Fingerprint, error)
	write               func(string, databaseevolution.FingerprintDocument) error
	runInvariants       func(context.Context, *sql.DB, string) (databaseevolution.InvariantResult, error)
	verifyCOSReferences func(context.Context, *sql.DB, string) ([]databaseevolution.COSReferenceResult, error)
	writeCOSManifest    func(string, []databaseevolution.COSReferenceResult) error
	queryManifestFiles  func(string) ([]string, error)
	stdout              io.Writer
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

type cosReferenceOptions struct {
	schema string
	output string
}

type queryManifestOptions struct {
	manifest string
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
		getenv:              os.Getenv,
		openDatabase:        func(dsn string) (*sql.DB, error) { return sql.Open("mysql", dsn) },
		capture:             databaseevolution.Capture,
		write:               databaseevolution.WriteFingerprintDocument,
		runInvariants:       databaseevolution.RunInvariantFile,
		verifyCOSReferences: databaseevolution.VerifyStoredCOSReferences,
		writeCOSManifest:    databaseevolution.WriteCOSReferenceManifest,
		queryManifestFiles:  loadQueryManifestFiles,
		stdout:              os.Stdout,
	}
	if err := run(context.Background(), os.Args[1:], dependencies); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, dependencies commandDependencies) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: admin-db <fingerprint|invariants|cos-references|query-manifest> [arguments]")
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
	case "cos-references":
		options, err := parseCOSReferenceOptions(args[1:])
		if err != nil {
			return err
		}
		return runCOSReferences(ctx, options, dependencies)
	case "query-manifest":
		if len(args) < 2 || args[1] != "files" {
			return fmt.Errorf("usage: admin-db query-manifest files --manifest <path>")
		}
		options, err := parseQueryManifestOptions(args[2:])
		if err != nil {
			return err
		}
		return runQueryManifestFiles(options, dependencies)
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

func parseCOSReferenceOptions(args []string) (cosReferenceOptions, error) {
	flags := flag.NewFlagSet("cos-references", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options cosReferenceOptions
	schemaFlag := &singleStringFlag{name: "schema", value: &options.schema}
	outputFlag := &singleStringFlag{name: "out", value: &options.output}
	flags.Var(schemaFlag, "schema", "schema containing COS references")
	flags.Var(outputFlag, "out", "evidence manifest path")
	if err := flags.Parse(args); err != nil {
		for _, value := range []*singleStringFlag{schemaFlag, outputFlag} {
			if value.duplicate {
				return cosReferenceOptions{}, fmt.Errorf("--%s may be provided only once", value.name)
			}
		}
		return cosReferenceOptions{}, fmt.Errorf("invalid COS reference arguments")
	}
	if flags.NArg() != 0 {
		return cosReferenceOptions{}, fmt.Errorf("unexpected argument")
	}
	options.schema = strings.TrimSpace(options.schema)
	options.output = strings.TrimSpace(options.output)
	if options.schema == "" {
		return cosReferenceOptions{}, fmt.Errorf("--schema is required")
	}
	if options.output == "" {
		return cosReferenceOptions{}, fmt.Errorf("--out is required")
	}
	return options, nil
}

func parseQueryManifestOptions(args []string) (queryManifestOptions, error) {
	flags := flag.NewFlagSet("query-manifest files", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options queryManifestOptions
	manifestFlag := &singleStringFlag{name: "manifest", value: &options.manifest}
	flags.Var(manifestFlag, "manifest", "query candidate manifest")
	if err := flags.Parse(args); err != nil {
		if manifestFlag.duplicate {
			return queryManifestOptions{}, fmt.Errorf("--manifest may be provided only once")
		}
		return queryManifestOptions{}, fmt.Errorf("invalid query-manifest arguments")
	}
	if flags.NArg() != 0 {
		return queryManifestOptions{}, fmt.Errorf("unexpected argument")
	}
	options.manifest = strings.TrimSpace(options.manifest)
	if options.manifest == "" {
		return queryManifestOptions{}, fmt.Errorf("--manifest is required")
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

func runCOSReferences(ctx context.Context, options cosReferenceOptions, dependencies commandDependencies) error {
	if dependencies.getenv == nil || dependencies.openDatabase == nil || dependencies.verifyCOSReferences == nil || dependencies.writeCOSManifest == nil || dependencies.stdout == nil {
		return fmt.Errorf("COS reference command dependencies are incomplete")
	}
	dsn := dependencies.getenv("MYSQL_DSN")
	if err := databaseevolution.ValidateSchemaDSN(dsn, options.schema); err != nil {
		return err
	}
	rootSecret := dependencies.getenv("APP_SECRET")
	if strings.TrimSpace(rootSecret) == "" {
		return fmt.Errorf("APP_SECRET is required")
	}
	database, err := dependencies.openDatabase(dsn)
	if err != nil {
		return safeCommandError("open MySQL connection", err)
	}
	defer database.Close()

	results, err := dependencies.verifyCOSReferences(ctx, database, rootSecret)
	if err != nil {
		return safeCommandError("verify COS references", err)
	}
	if err := dependencies.writeCOSManifest(options.output, results); err != nil {
		return safeCommandError("write COS reference manifest", err)
	}
	counts := map[string]int{
		databaseevolution.COSReferenceReachable:  0,
		databaseevolution.COSReferenceNotFound:   0,
		databaseevolution.COSReferenceDependency: 0,
	}
	for _, result := range results {
		counts[result.Status]++
	}
	if _, err := fmt.Fprintln(dependencies.stdout, options.output); err != nil {
		return fmt.Errorf("print COS reference manifest path: %w", err)
	}
	for _, status := range []string{databaseevolution.COSReferenceReachable, databaseevolution.COSReferenceNotFound, databaseevolution.COSReferenceDependency} {
		if _, err := fmt.Fprintf(dependencies.stdout, "%s\t%d\n", status, counts[status]); err != nil {
			return fmt.Errorf("print COS reference summary: %w", err)
		}
	}
	if counts[databaseevolution.COSReferenceNotFound]+counts[databaseevolution.COSReferenceDependency] != 0 {
		return safeCommandError("verify COS references", errors.New("one or more COS references are not reachable"))
	}
	return nil
}

func runQueryManifestFiles(options queryManifestOptions, dependencies commandDependencies) error {
	if dependencies.queryManifestFiles == nil || dependencies.stdout == nil {
		return fmt.Errorf("query-manifest command dependencies are incomplete")
	}
	files, err := dependencies.queryManifestFiles(options.manifest)
	if err != nil {
		return safeCommandError("validate query manifest", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("query manifest produced no repository files")
	}
	for _, file := range files {
		if _, err := fmt.Fprintln(dependencies.stdout, file); err != nil {
			return fmt.Errorf("print query manifest file: %w", err)
		}
	}
	return nil
}

func loadQueryManifestFiles(path string) ([]string, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	candidates, err := databaseevolution.LoadQueryManifest(path, root)
	if err != nil {
		return nil, err
	}
	return databaseevolution.QueryManifestFiles(candidates), nil
}

func safeCommandError(operation string, err error) error {
	return &commandError{operation: operation, cause: err}
}
