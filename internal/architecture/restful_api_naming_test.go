package architecture

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestAdminRoutesExposePageInitForPageDictionariesWithoutLegacyInitAliases(t *testing.T) {
	root := backendRoot(t)
	routes := readRouteSnapshot(t, root)
	pageInitRoutes := []string{
		"GET /api/admin/v1/auth-platforms/page-init",
		"GET /api/admin/v1/cron-tasks/page-init",
		"GET /api/admin/v1/notifications/page-init",
		"GET /api/admin/v1/notification-tasks/page-init",
		"GET /api/admin/v1/operation-logs/page-init",
		"GET /api/admin/v1/permissions/page-init",
		"GET /api/admin/v1/roles/page-init",
		"GET /api/admin/v1/system-logs/page-init",
		"GET /api/admin/v1/system-settings/page-init",
		"GET /api/admin/v1/upload-drivers/page-init",
		"GET /api/admin/v1/upload-rules/page-init",
		"GET /api/admin/v1/upload-settings/page-init",
	}

	for _, route := range pageInitRoutes {
		assertRouteSnapshotContains(t, routes, route)
	}
	for route := range routes {
		if strings.HasSuffix(routePath(route), "/init") {
			t.Fatalf("legacy init route remains: %s", route)
		}
	}
}

func TestCurrentUserBootstrapUsesUsersMeForEveryPlatform(t *testing.T) {
	routes := readRouteSnapshot(t, backendRoot(t))
	for _, route := range []string{
		"GET /api/admin/v1/users/me",
		"GET /api/app/v1/users/me",
		"GET /api/canvas/v1/users/me",
	} {
		assertRouteSnapshotContains(t, routes, route)
	}
	for route := range routes {
		if strings.Contains(routePath(route), "/users/init") {
			t.Fatalf("current-user bootstrap must not expose users/init: %s", route)
		}
	}
}

func TestAdminRoutesDoNotUseLegacyActionPaths(t *testing.T) {
	routes := readRouteSnapshot(t, backendRoot(t))
	legacySegments := map[string]struct{}{"list": {}, "add": {}, "edit": {}, "del": {}}
	var offenders []string

	for route := range routes {
		path := routePath(route)
		for _, segment := range strings.Split(strings.Trim(path, "/"), "/") {
			if _, ok := legacySegments[segment]; ok {
				offenders = append(offenders, route)
				break
			}
		}
	}

	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Fatalf("legacy action route paths remain:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func readRouteSnapshot(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "internal", "server", "testdata", "admin_routes_golden.txt"))
	if err != nil {
		t.Fatalf("read route snapshot: %v", err)
	}
	routes := make(map[string]struct{})
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		routes[line] = struct{}{}
	}
	return routes
}

func routePath(route string) string {
	fields := strings.Fields(route)
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

func assertRouteSnapshotContains(t *testing.T, routes map[string]struct{}, route string) {
	t.Helper()
	if _, ok := routes[route]; !ok {
		t.Fatalf("route snapshot must contain %s", route)
	}
}
