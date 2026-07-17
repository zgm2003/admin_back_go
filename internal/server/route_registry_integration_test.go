package server_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"admin_back_go/internal/bootstrap"
	"admin_back_go/internal/server"
	"admin_back_go/internal/server/adminroute"
)

func TestRouteRegistryCompilesEveryActualRoute(t *testing.T) {
	registry, err := bootstrap.AdminRouteRegistry()
	if err != nil {
		t.Fatalf("build route registry: %v", err)
	}
	router, err := server.NewRouter(server.Dependencies{
		Core: server.CoreDependencies{RouteRegistry: registry},
	})
	if err != nil {
		t.Fatalf("compile actual routes: %v", err)
	}
	if got, want := len(registry.Definitions()), len(router.Routes()); got != want {
		t.Fatalf("definitions=%d routes=%d", got, want)
	}
}

func TestRoutePolicyGoldenIsAdminOnlyAndCurrent(t *testing.T) {
	registry, err := bootstrap.AdminRouteRegistry()
	if err != nil {
		t.Fatalf("build route registry: %v", err)
	}
	if _, err := server.NewRouter(server.Dependencies{
		Core: server.CoreDependencies{RouteRegistry: registry},
	}); err != nil {
		t.Fatalf("compile actual routes: %v", err)
	}

	definitions := registry.Definitions()
	adminDefinitions := make([]adminroute.Definition, 0, len(definitions))
	operationIDs := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if definition.OperationID == "" {
			t.Fatalf("missing operation ID for %s %s", definition.Method, definition.Path)
		}
		if _, exists := operationIDs[definition.OperationID]; exists {
			t.Fatalf("duplicate operation ID %q", definition.OperationID)
		}
		operationIDs[definition.OperationID] = struct{}{}
		if strings.HasPrefix(definition.Path, "/api/app/") || strings.HasPrefix(definition.Path, "/api/canvas/") {
			continue
		}
		if strings.HasPrefix(definition.Path, "/api/admin/") ||
			strings.HasPrefix(definition.Path, "/api/payment/callbacks/") ||
			definition.Path == "/health" || definition.Path == "/ready" {
			adminDefinitions = append(adminDefinitions, definition)
		}
	}
	data, err := json.MarshalIndent(struct {
		Routes []adminroute.Definition `json:"routes"`
	}{Routes: adminDefinitions}, "", "  ")
	if err != nil {
		t.Fatalf("marshal policy golden: %v", err)
	}
	data = append(data, '\n')
	if bytes.Contains(data, []byte("/api/app/")) || bytes.Contains(data, []byte("/api/canvas/")) {
		t.Fatalf("retired route leaked into admin policy golden")
	}

	goldenPath := filepath.Join("testdata", "admin_route_policy_golden.json")
	if os.Getenv("UPDATE_ADMIN_ROUTE_POLICY_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, data, 0o644); err != nil {
			t.Fatalf("write policy golden: %v", err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read policy golden: %v", err)
	}
	if !bytes.Equal(data, want) {
		t.Fatalf("admin route policy golden is stale; run with UPDATE_ADMIN_ROUTE_POLICY_GOLDEN=1")
	}
}
