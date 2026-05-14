package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestFallbackMessageIDIsStable(t *testing.T) {
	got := FallbackMessageID("账号不存在")
	if got != "legacy.37f36ca852b6" {
		t.Fatalf("unexpected fallback message id: %s", got)
	}
	if FallbackMessageID(" 账号不存在 ") != got {
		t.Fatalf("fallback message id should trim whitespace")
	}
	if FallbackMessageID("") != "" {
		t.Fatalf("empty fallback should not produce a message id")
	}
}

func TestLegacyFallbackCatalogCoversPublicErrorLiterals(t *testing.T) {
	keys, err := CatalogKeys("en-US")
	if err != nil {
		t.Fatalf("load en-US catalog keys: %v", err)
	}

	root := projectRoot(t)
	messages := publicErrorStringLiterals(t, filepath.Join(root, "internal"))
	for msg := range messages {
		key := FallbackMessageID(msg)
		if _, ok := keys[key]; !ok {
			t.Fatalf("legacy fallback catalog missing %s for %q", key, msg)
		}
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func publicErrorStringLiterals(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	result := map[string]struct{}{}
	fset := token.NewFileSet()
	err := filepath.Walk(root, func(path string, info fs.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		clean := filepath.ToSlash(path)
		if strings.Contains(clean, "/internal/apperror/") || strings.Contains(clean, "/internal/i18n/") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			if isBareAppErrorCall(call) {
				for _, idx := range publicMessageArgIndexes(call) {
					if idx >= len(call.Args) {
						continue
					}
					if msg, ok := stringLiteral(call.Args[idx]); ok && containsCJK(msg) {
						result[msg] = struct{}{}
					}
				}
				return true
			}
			if isDynamicPublicErrorHelperCall(call) {
				for _, arg := range call.Args {
					if msg, ok := stringLiteral(arg); ok && containsCJK(msg) {
						result[msg] = struct{}{}
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan public error literals: %v", err)
	}
	return result
}

func isBareAppErrorCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "apperror" {
		return false
	}
	switch selector.Sel.Name {
	case "BadRequest", "Unauthorized", "Forbidden", "NotFound", "Internal", "Wrap", "New":
		return true
	default:
		return false
	}
}

func publicMessageArgIndexes(call *ast.CallExpr) []int {
	selector := call.Fun.(*ast.SelectorExpr)
	switch selector.Sel.Name {
	case "Wrap", "New":
		return []int{2}
	default:
		return []int{0}
	}
}

func isDynamicPublicErrorHelperCall(call *ast.CallExpr) bool {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	switch ident.Name {
	case "routeID", "routeInt64", "parseIDs", "normalizeSchema", "normalizeStrictSchema", "schemaMap":
		return true
	default:
		return false
	}
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(value), value != ""
}

func containsCJK(value string) bool {
	for _, r := range value {
		if r >= '\u4e00' && r <= '\u9fff' {
			return true
		}
	}
	return false
}
