package i18n

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type sourceMessageKey struct {
	Key      string
	Location string
}

func TestCatalogCoversSourceResponseMessages(t *testing.T) {
	zhKeys, err := CatalogKeys("zh-CN")
	if err != nil {
		t.Fatalf("load zh-CN catalog keys: %v", err)
	}
	enKeys, err := CatalogKeys("en-US")
	if err != nil {
		t.Fatalf("load en-US catalog keys: %v", err)
	}

	root := projectRoot(t)
	explicit, legacy := sourceResponseMessageKeys(t, filepath.Join(root, "internal"))

	var missing []string
	for _, item := range append(explicit, legacy...) {
		if _, ok := zhKeys[item.Key]; !ok {
			missing = append(missing, item.Key+" missing zh-CN at "+item.Location)
		}
		if _, ok := enKeys[item.Key]; !ok {
			missing = append(missing, item.Key+" missing en-US at "+item.Location)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("catalog is missing source response message keys:\n%s", strings.Join(missing, "\n"))
	}
}

func sourceResponseMessageKeys(t *testing.T, root string) ([]sourceMessageKey, []sourceMessageKey) {
	t.Helper()
	fset := token.NewFileSet()
	explicit := map[string]string{}
	legacy := map[string]string{}

	for msg := range publicErrorStringLiterals(t, root) {
		legacy[FallbackMessageID(msg)] = "public app error fallback"
	}

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
			location := sourceLocation(fset, call)
			if key, ok := explicitAppErrorMessageID(call); ok {
				explicit[key] = location
				return true
			}
			if key, ok := responseMessageID(call); ok {
				explicit[key] = location
				return true
			}
			if msg, ok := responseLegacyMessage(call); ok && containsCJK(msg) {
				legacy[FallbackMessageID(msg)] = location
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan source response message keys: %v", err)
	}

	return flattenSourceMessageMap(explicit), flattenSourceMessageMap(legacy)
}

func explicitAppErrorMessageID(call *ast.CallExpr) (string, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "apperror" {
		return "", false
	}
	idx := -1
	switch selector.Sel.Name {
	case "BadRequestKey", "UnauthorizedKey", "ForbiddenKey", "NotFoundKey", "InternalKey":
		idx = 0
	case "NewKey", "WrapKey":
		idx = 2
	}
	if idx < 0 || idx >= len(call.Args) {
		return "", false
	}
	return stringLiteral(call.Args[idx])
}

func responseMessageID(call *ast.CallExpr) (string, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "response" || selector.Sel.Name != "OKWithMessageKey" {
		return "", false
	}
	if len(call.Args) < 3 {
		return "", false
	}
	return stringLiteral(call.Args[2])
}

func responseLegacyMessage(call *ast.CallExpr) (string, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "response" || selector.Sel.Name != "OKWithMessage" {
		return "", false
	}
	if len(call.Args) < 3 {
		return "", false
	}
	return stringLiteral(call.Args[2])
}

func flattenSourceMessageMap(items map[string]string) []sourceMessageKey {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]sourceMessageKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, sourceMessageKey{Key: key, Location: items[key]})
	}
	return out
}

func sourceLocation(fset *token.FileSet, call ast.Node) string {
	pos := fset.Position(call.Pos())
	if !pos.IsValid() {
		return "unknown"
	}
	return fmt.Sprintf("%s:%d", filepath.ToSlash(pos.Filename), pos.Line)
}
