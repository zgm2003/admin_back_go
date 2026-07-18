package admincontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"admin_back_go/internal/bootstrap"
	"admin_back_go/internal/server"
	"admin_back_go/internal/server/adminroute"
)

var fullGitCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type BuildOptions struct {
	BackendCommit string
}

type Bundle struct {
	Artifacts map[string][]byte
	Manifest  Manifest
	manifest  []byte
}

func Build(options BuildOptions) (Bundle, error) {
	backendCommit := strings.TrimSpace(options.BackendCommit)
	if !fullGitCommitPattern.MatchString(backendCommit) {
		return Bundle{}, errors.New("backend commit must be an explicit 40-character lowercase Git SHA")
	}

	definitions, err := loadRuntimeDefinitions()
	if err != nil {
		return Bundle{}, err
	}
	views := buildViewsDocument()
	permissions := buildPermissionsDocument(definitions, views)
	views.UsersMe.ResponseSchema = usersMeResponseSchema(viewKeys(views.Views), permissions.PermissionCodes)
	openAPI, err := buildOpenAPI(definitions)
	if err != nil {
		return Bundle{}, fmt.Errorf("build OpenAPI: %w", err)
	}
	envelopeSchema, eventsSchema, err := buildRealtimeSchemas()
	if err != nil {
		return Bundle{}, fmt.Errorf("build realtime schemas: %w", err)
	}

	artifacts := make(map[string][]byte, 5)
	for name, value := range map[string]any{
		"openapi.json":                  openAPI,
		"permissions.json":              permissions,
		"views.json":                    views,
		"realtime/envelope.schema.json": envelopeSchema,
		"realtime/events.schema.json":   eventsSchema,
	} {
		data, marshalErr := marshalDocument(value)
		if marshalErr != nil {
			return Bundle{}, fmt.Errorf("marshal %s: %w", name, marshalErr)
		}
		artifacts[name] = data
	}

	manifest := newManifest(backendCommit, artifacts)
	manifestData, err := marshalDocument(manifest)
	if err != nil {
		return Bundle{}, fmt.Errorf("marshal manifest: %w", err)
	}
	return Bundle{Artifacts: artifacts, Manifest: manifest, manifest: manifestData}, nil
}

func (bundle Bundle) Files() map[string][]byte {
	files := make(map[string][]byte, len(bundle.Artifacts)+1)
	for name, data := range bundle.Artifacts {
		files[name] = append([]byte(nil), data...)
	}
	files["manifest.json"] = append([]byte(nil), bundle.manifest...)
	return files
}

func loadRuntimeDefinitions() ([]adminroute.Definition, error) {
	registry, err := bootstrap.AdminRouteRegistry()
	if err != nil {
		return nil, fmt.Errorf("build Admin route registry: %w", err)
	}
	if _, err := server.NewRouter(server.Dependencies{
		Core: server.CoreDependencies{RouteRegistry: registry},
	}); err != nil {
		return nil, fmt.Errorf("compile runtime routes: %w", err)
	}

	definitions := make([]adminroute.Definition, 0)
	operationIDs := make(map[string]struct{})
	for _, definition := range registry.Definitions() {
		if !isAdminContractPath(definition.Path) {
			continue
		}
		if definition.OperationID == "" {
			return nil, fmt.Errorf("route %s %s has no operation ID", definition.Method, definition.Path)
		}
		if _, duplicate := operationIDs[definition.OperationID]; duplicate {
			return nil, fmt.Errorf("duplicate operation ID %q", definition.OperationID)
		}
		operationIDs[definition.OperationID] = struct{}{}
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(left int, right int) bool {
		if definitions[left].Path != definitions[right].Path {
			return definitions[left].Path < definitions[right].Path
		}
		if definitions[left].Method != definitions[right].Method {
			return definitions[left].Method < definitions[right].Method
		}
		return definitions[left].OperationID < definitions[right].OperationID
	})
	return definitions, nil
}

func isAdminContractPath(path string) bool {
	return path == "/health" ||
		path == "/ready" ||
		strings.HasPrefix(path, "/api/admin/") ||
		strings.HasPrefix(path, "/api/payment/callbacks/")
}

func marshalDocument(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func sortedFileNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func WriteAtomic(output string, bundle Bundle) error {
	output, err := normalizedOutputPath(output)
	if err != nil {
		return err
	}
	files := bundle.Files()
	if len(files) == 0 || len(bundle.Artifacts) == 0 || len(bundle.manifest) == 0 {
		return errors.New("contract bundle is empty")
	}

	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create contract parent: %w", err)
	}
	temporary, err := os.MkdirTemp(parent, "."+filepath.Base(output)+".tmp-")
	if err != nil {
		return fmt.Errorf("create contract staging directory: %w", err)
	}
	defer os.RemoveAll(temporary)

	names := sortedFileNames(files)
	for _, name := range names {
		if name == "manifest.json" {
			continue
		}
		if err := writeBundleFile(temporary, name, files[name]); err != nil {
			return err
		}
	}
	if err := writeBundleFile(temporary, "manifest.json", files["manifest.json"]); err != nil {
		return err
	}

	if _, err := os.Stat(output); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(temporary, output); err != nil {
			return fmt.Errorf("publish contract bundle: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect existing contract bundle: %w", err)
	}

	backup, err := os.MkdirTemp(parent, "."+filepath.Base(output)+".backup-")
	if err != nil {
		return fmt.Errorf("reserve contract backup path: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("prepare contract backup path: %w", err)
	}
	if err := os.Rename(output, backup); err != nil {
		return fmt.Errorf("stage existing contract bundle: %w", err)
	}
	if err := os.Rename(temporary, output); err != nil {
		restoreErr := os.Rename(backup, output)
		return errors.Join(fmt.Errorf("publish contract bundle: %w", err), restoreErr)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove previous contract bundle: %w", err)
	}
	return nil
}

func Check(output string, bundle Bundle) error {
	output, err := normalizedOutputPath(output)
	if err != nil {
		return err
	}
	expected := bundle.Files()
	actualNames := make([]string, 0, len(expected))
	err = filepath.WalkDir(output, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, relErr := filepath.Rel(output, path)
		if relErr != nil {
			return relErr
		}
		actualNames = append(actualNames, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return fmt.Errorf("read generated contract bundle: %w", err)
	}
	sort.Strings(actualNames)
	expectedNames := sortedFileNames(expected)
	if len(actualNames) != len(expectedNames) {
		return fmt.Errorf("contract file set differs: expected %v, found %v", expectedNames, actualNames)
	}
	for index, name := range expectedNames {
		if actualNames[index] != name {
			return fmt.Errorf("contract file set differs: expected %v, found %v", expectedNames, actualNames)
		}
		actual, readErr := os.ReadFile(filepath.Join(output, filepath.FromSlash(name)))
		if readErr != nil {
			return fmt.Errorf("read %s: %w", name, readErr)
		}
		if !bytes.Equal(actual, expected[name]) {
			return fmt.Errorf("contract artifact %s differs from generated bytes", name)
		}
	}
	return nil
}

func normalizedOutputPath(output string) (string, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return "", errors.New("contract output directory is required")
	}
	absolute, err := filepath.Abs(output)
	if err != nil {
		return "", fmt.Errorf("resolve contract output directory: %w", err)
	}
	if filepath.Base(absolute) == "." || filepath.Base(absolute) == string(filepath.Separator) {
		return "", errors.New("contract output directory must not be a filesystem root")
	}
	return filepath.Clean(absolute), nil
}

func writeBundleFile(root string, name string, data []byte) error {
	cleanName := filepath.Clean(filepath.FromSlash(name))
	if cleanName == "." || filepath.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid contract artifact path %q", name)
	}
	path := filepath.Join(root, cleanName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", name, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
