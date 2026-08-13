package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestAnonymousListResponseDetectionIgnoresFormatting(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{
			name:   "single line",
			source: `package admin; var response = gin.H{"list": result}`,
			want:   true,
		},
		{
			name: "multiline",
			source: `package admin
var response = gin.H{
	"list": result,
}`,
			want: true,
		},
		{
			name:   "named response",
			source: `package admin; var response = module.ListResponse{List: result}`,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hasAnonymousListResponse([]byte(tt.source))
			if err != nil {
				t.Fatalf("parse source: %v", err)
			}
			if got != tt.want {
				t.Fatalf("hasAnonymousListResponse()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestAdminHandlersDoNotBuildAnonymousListResponses(t *testing.T) {
	root := filepath.Join(backendRoot(t), "internal", "module")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "handler.go" || filepath.Base(filepath.Dir(path)) != "admin" {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		anonymous, err := hasAnonymousListResponse(source)
		if err != nil {
			return err
		}
		if anonymous {
			t.Errorf("%s builds an anonymous list response instead of its route contract DTO", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan Admin handlers: %v", err)
	}
}

func hasAnonymousListResponse(source []byte) (bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "handler.go", source, 0)
	if err != nil {
		return false, err
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		selector, ok := literal.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "H" {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || pkg.Name != "gin" {
			return true
		}
		for _, element := range literal.Elts {
			pair, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := pair.Key.(*ast.BasicLit)
			if ok && key.Kind == token.STRING && key.Value == `"list"` {
				found = true
				return false
			}
		}
		return true
	})
	return found, nil
}
