package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformRouteLineOwnership(t *testing.T) {
	root := backendRoot(t)
	for _, rel := range []string{
		"internal/module/auth/transport/canvas/route.go",
		"internal/module/auth/transport/canvas/handler.go",
		"internal/module/profile/transport/canvas/route.go",
		"internal/module/profile/transport/canvas/handler.go",
		"internal/module/payment/transport/canvas/route.go",
		"internal/module/payment/transport/canvas/handler.go",
		"internal/module/payment/wallet/transport/canvas/route.go",
		"internal/module/payment/wallet/transport/canvas/handler.go",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}

	routesAuth := readRouteLineSource(t, root, "internal/server/routes_auth.go")
	mustContainRouteLine(t, routesAuth, `authcanvas "admin_back_go/internal/module/auth/transport/canvas"`)
	mustContainRouteLine(t, routesAuth, `authcanvas.Register(router, authcanvas.Dependencies{`)
	mustNotContainRouteLine(t, routesAuth, `Prefix:         "/api/canvas/v1/auth"`)
	mustNotContainRouteLine(t, routesAuth, `authapp.RouteOptions`)

	routesUser := readRouteLineSource(t, root, "internal/server/routes_admin_user.go")
	mustContainRouteLine(t, routesUser, `profilecanvas "admin_back_go/internal/module/profile/transport/canvas"`)
	mustContainRouteLine(t, routesUser, `profilecanvas.RegisterRoutes(router, profilecanvas.Dependencies{`)
	mustNotContainRouteLine(t, routesUser, `RegisterRoutesWithOptions`)
	mustNotContainRouteLine(t, routesUser, `UsersPrefix: "/api/canvas/v1/users"`)

	routesCommerce := readRouteLineSource(t, root, "internal/server/routes_admin_commerce_rbac.go")
	mustContainRouteLine(t, routesCommerce, `paymentcanvas "admin_back_go/internal/module/payment/transport/canvas"`)
	mustContainRouteLine(t, routesCommerce, `walletcanvas "admin_back_go/internal/module/payment/wallet/transport/canvas"`)
	mustContainRouteLine(t, routesCommerce, `walletcanvas.RegisterRoutes(router, deps.WalletService)`)
	mustContainRouteLine(t, routesCommerce, `paymentcanvas.RegisterRechargeRoutes(router, deps.PaymentService)`)
	mustNotContainRouteLine(t, routesCommerce, `walletadmin.RegisterCurrentUserRoutes(router, "/api/canvas/v1/wallet"`)
	mustNotContainRouteLine(t, routesCommerce, `paymentadmin.RegisterRechargeRoutes(router, "/api/canvas/v1/payment/recharges"`)
}

func TestPlatformRouteLineNoCrossPlatformURLPrefixInsideWrongTransport(t *testing.T) {
	root := backendRoot(t)
	moduleRoot := filepath.Join(root, "internal", "module")
	var offenders []string
	err := filepath.WalkDir(moduleRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if !strings.Contains(rel, "/transport/") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		switch {
		case strings.Contains(rel, "/transport/app/") && strings.Contains(text, `"/api/canvas/v1`):
			offenders = append(offenders, rel+" contains canvas URL inside app transport")
		case strings.Contains(rel, "/transport/admin/") && strings.Contains(text, `"/api/canvas/v1`):
			offenders = append(offenders, rel+" contains canvas URL inside admin transport")
		case strings.Contains(rel, "/transport/canvas/") && strings.Contains(text, `"/api/app/v1`):
			offenders = append(offenders, rel+" contains app URL inside canvas transport")
		case strings.Contains(rel, "/transport/canvas/") && strings.Contains(text, `"/api/admin/v1`):
			offenders = append(offenders, rel+" contains admin URL inside canvas transport")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk transport files: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("platform URL prefix must stay inside matching transport package:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func readRouteLineSource(t *testing.T, root string, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(body)
}

func mustContainRouteLine(t *testing.T, text string, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("expected route-line source to contain %q", want)
	}
}

func mustNotContainRouteLine(t *testing.T, text string, forbidden string) {
	t.Helper()
	if strings.Contains(text, forbidden) {
		t.Fatalf("route-line source must not contain %q", forbidden)
	}
}
