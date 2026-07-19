package architecture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/admincontract"
	"admin_back_go/internal/module/permission"
)

func TestCredentialContractHasNoPublicRefreshFieldsOrAccessCookieFallback(t *testing.T) {
	root := backendRoot(t)
	bundle, err := admincontract.Build(admincontract.BuildOptions{BackendCommit: strings.Repeat("a", 40)})
	if err != nil {
		t.Fatalf("build Admin contract: %v", err)
	}
	var document any
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatalf("decode Admin OpenAPI: %v", err)
	}
	for _, property := range []string{"refresh_token", "refresh_expires_in"} {
		var exposed []string
		findJSONProperty(document, "$", property, &exposed)
		if len(exposed) != 0 {
			sort.Strings(exposed)
			t.Fatalf("Admin OpenAPI exposes %s:\n  %s", property, strings.Join(exposed, "\n  "))
		}
	}

	cookieFallback := regexp.MustCompile(`(?m)(?:Cookie|GetCookie)\s*\(\s*["` + "`" + `]access_token["` + "`" + `]`)
	queryFallback := regexp.MustCompile(`(?m)(?:Query|GetQuery)\s*\(\s*["` + "`" + `]access_token["` + "`" + `]`)
	var offenders []string
	err = filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		if cookieFallback.Match(body) {
			offenders = append(offenders, filepath.ToSlash(relative)+" reads access_token from a cookie")
		}
		if queryFallback.Match(body) {
			offenders = append(offenders, filepath.ToSlash(relative)+" reads access_token from a query")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan credential transports: %v", err)
	}
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Fatalf("legacy access credential fallbacks exist:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestIdentityRefreshRotationRequiresExpectedHashCAS(t *testing.T) {
	root := backendRoot(t)
	repository := string(identityReadFile(t, root, "internal/module/auth/session_repository.go"))
	casBody := identityFunctionBody(t, repository, "func (r *SessionGormRepository) RotateIfRefreshHash")
	for _, required := range []string{
		`Where("id = ? AND refresh_token_hash = ?", sessionID, previousHash)`,
		`return result.RowsAffected == 1, result.Error`,
	} {
		if !strings.Contains(casBody, required) {
			t.Fatalf("refresh CAS is missing %q", required)
		}
	}

	methodPattern := regexp.MustCompile(`func \(r \*SessionGormRepository\) ([A-Za-z0-9_]*Rotate[A-Za-z0-9_]*)\([^\{]*\{`)
	var unsafe []string
	for _, match := range methodPattern.FindAllStringSubmatchIndex(repository, -1) {
		name := repository[match[2]:match[3]]
		body := identityFunctionBodyAt(t, repository, match[0])
		if strings.Contains(body, `"refresh_token_hash"`) && (!strings.Contains(body, "previousHash") || !strings.Contains(body, "RowsAffected == 1")) {
			unsafe = append(unsafe, name)
		}
	}
	if len(unsafe) > 0 {
		t.Fatalf("refresh-token write methods bypass expected-hash CAS: %s", strings.Join(unsafe, ", "))
	}

	lifecycle := string(identityReadFile(t, root, "internal/module/auth/session_lifecycle.go"))
	rotateBody := identityFunctionBody(t, lifecycle, "func (a *SessionLifecycle) Rotate")
	if !strings.Contains(rotateBody, "RotateIfRefreshHash(ctx, session.ID, previousRefreshHash, rotation)") {
		t.Fatal("SessionLifecycle.Rotate does not pass the expected previous refresh hash to the repository CAS")
	}
}

func TestIdentityPermissionCacheErrorsFailClosed(t *testing.T) {
	repository := &identityCountingPrincipalRepository{}
	service := permission.NewPrincipalService(repository, identityFailingPrincipalCache{}, permission.PrincipalServiceOptions{})

	appErr := service.Authorize(context.Background(), 7, "admin", "user_userManager_list")

	if appErr == nil || appErr.Code != "permission.principal_cache_unavailable" {
		t.Fatalf("Redis cache failure did not fail closed: %#v", appErr)
	}
	if repository.loads != 0 {
		t.Fatalf("Redis cache failure fell back to SQL %d time(s)", repository.loads)
	}
}

func TestRoutePolicyCatalogAndMutationClassificationAreComplete(t *testing.T) {
	root := backendRoot(t)
	var catalog struct {
		PermissionCodes []string `json:"permission_codes"`
	}
	if err := json.Unmarshal(identityReadFile(t, root, "contracts/admin/v1/permissions.json"), &catalog); err != nil {
		t.Fatalf("decode permission catalog: %v", err)
	}
	known := make(map[string]struct{}, len(catalog.PermissionCodes))
	for _, code := range catalog.PermissionCodes {
		known[code] = struct{}{}
	}
	var policy struct {
		Routes []identityRoutePolicy `json:"routes"`
	}
	if err := json.Unmarshal(identityReadFile(t, root, "internal/server/testdata/admin_route_policy_golden.json"), &policy); err != nil {
		t.Fatalf("decode route-policy golden: %v", err)
	}
	if len(policy.Routes) == 0 {
		t.Fatal("route-policy golden is empty")
	}
	seen := make(map[string]struct{}, len(policy.Routes))
	var violations []string
	for _, route := range policy.Routes {
		label := route.Method + " " + route.Path
		if _, duplicate := seen[label]; duplicate {
			violations = append(violations, label+": duplicate route")
		}
		seen[label] = struct{}{}
		switch route.Access.Kind {
		case "public", "authenticated":
			if route.Access.PermissionCode != "" {
				violations = append(violations, label+": non-permission route carries a permission code")
			}
		case "permission":
			if _, exists := known[route.Access.PermissionCode]; !exists {
				violations = append(violations, label+": unknown permission code "+route.Access.PermissionCode)
			}
		default:
			violations = append(violations, label+": missing access classification")
		}
		if identityMutationMethod(route.Method) {
			if route.Audit.Enabled {
				if route.Audit.Module == "" || route.Audit.Action == "" || route.Audit.Title == "" {
					violations = append(violations, label+": incomplete audit metadata")
				}
			} else if strings.TrimSpace(route.Audit.Reason) == "" {
				violations = append(violations, label+": mutation has no audit decision")
			}
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("identity/route policy violations:\n  %s", strings.Join(violations, "\n  "))
	}
}

func TestRoutePolicyHasNoLegacyMetadataMaps(t *testing.T) {
	root := backendRoot(t)
	allowedPrefix := filepath.ToSlash(filepath.Join("internal", "server", "adminroute")) + "/"
	banned := []string{"permissionRouteRules", "operationRouteRules", "route_meta.go", "map[middleware.RouteKey]"}
	var offenders []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		relative = filepath.ToSlash(relative)
		if strings.HasPrefix(relative, allowedPrefix) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, token := range banned {
			if strings.Contains(string(body), token) {
				offenders = append(offenders, relative+" contains "+token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan route metadata: %v", err)
	}
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Fatalf("legacy route metadata maps exist:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestIdentityRoutingGateIsBlockingInRepositoryVerification(t *testing.T) {
	root := backendRoot(t)
	gate := identityReadFile(t, root, "scripts/verify-identity-routing.ps1")
	for _, required := range []string{
		"-race", "./internal/module/auth", "./internal/module/permission", "./internal/module/role", "./internal/module/user",
		"./internal/middleware", "./internal/server/...", "./internal/architecture", "TestIdentity|TestRoutePolicy|TestCredential",
		"scripts/check-admin-contract.ps1",
	} {
		if !strings.Contains(string(gate), required) {
			t.Errorf("identity routing gate is missing %q", required)
		}
	}
	backend := identityReadFile(t, root, "scripts/verify-backend.ps1")
	if !strings.Contains(string(backend), "verify-identity-routing.ps1") {
		t.Error("verify-backend.ps1 does not invoke the blocking identity routing gate")
	}
}

type identityRoutePolicy struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Access struct {
		Kind           string `json:"kind"`
		PermissionCode string `json:"permission_code"`
	} `json:"access"`
	Audit struct {
		Enabled bool   `json:"enabled"`
		Module  string `json:"module"`
		Action  string `json:"action"`
		Title   string `json:"title"`
		Reason  string `json:"reason"`
	} `json:"audit"`
}

type identityCountingPrincipalRepository struct {
	loads int
}

func (repository *identityCountingPrincipalRepository) LoadSnapshot(context.Context, int64, string) (permission.PrincipalSnapshot, error) {
	repository.loads++
	return permission.PrincipalSnapshot{}, errors.New("SQL must not be reached")
}

func (*identityCountingPrincipalRepository) CurrentVersions(context.Context, []permission.PrincipalSubject) ([]permission.PrincipalVersion, error) {
	return nil, nil
}

func (*identityCountingPrincipalRepository) AllVersions(context.Context) ([]permission.PrincipalVersion, error) {
	return nil, nil
}

type identityFailingPrincipalCache struct{}

func (identityFailingPrincipalCache) Load(context.Context, int64, string) (*permission.PrincipalSnapshot, permission.PrincipalCacheState, error) {
	return nil, permission.PrincipalCacheMiss, errors.New("redis unavailable")
}

func (identityFailingPrincipalCache) Store(context.Context, permission.PrincipalSnapshot, time.Duration) (bool, error) {
	return false, errors.New("redis unavailable")
}

func (identityFailingPrincipalCache) Begin(context.Context, []permission.PrincipalVersion, string) error {
	return errors.New("redis unavailable")
}

func (identityFailingPrincipalCache) Publish(context.Context, []permission.PrincipalVersion, []permission.PrincipalVersion, string) error {
	return errors.New("redis unavailable")
}

func (identityFailingPrincipalCache) Abort(context.Context, []permission.PrincipalVersion, string) error {
	return errors.New("redis unavailable")
}

func (identityFailingPrincipalCache) Reconcile(context.Context, []permission.PrincipalVersion) error {
	return errors.New("redis unavailable")
}

func findJSONProperty(value any, path string, property string, matches *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		if properties, ok := typed["properties"].(map[string]any); ok {
			if _, exposed := properties[property]; exposed {
				*matches = append(*matches, path+".properties."+property)
			}
		}
		for key, child := range typed {
			findJSONProperty(child, path+"."+key, property, matches)
		}
	case []any:
		for index, child := range typed {
			findJSONProperty(child, fmt.Sprintf("%s[%d]", path, index), property, matches)
		}
	}
}

func identityMutationMethod(method string) bool {
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

func identityFunctionBody(t *testing.T, source string, signature string) string {
	t.Helper()
	start := strings.Index(source, signature)
	if start < 0 {
		t.Fatalf("function signature %q was not found", signature)
	}
	return identityFunctionBodyAt(t, source, start)
}

func identityFunctionBodyAt(t *testing.T, source string, start int) string {
	t.Helper()
	open := strings.Index(source[start:], "{")
	if open < 0 {
		t.Fatal("function body has no opening brace")
	}
	open += start
	depth := 0
	for index := open; index < len(source); index++ {
		switch source[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[start : index+1]
			}
		}
	}
	t.Fatal("function body has no closing brace")
	return ""
}

func identityReadFile(t *testing.T, root string, relative string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return body
}
