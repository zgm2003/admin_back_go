package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestRESTfulRouteNamingGuard(t *testing.T) {
	root := backendRoot(t)
	moduleRoot := filepath.Join(root, "internal", "module")

	var violations []string
	routeActionPattern := regexp.MustCompile(`\.(GET|POST|PUT|PATCH|DELETE)\("/(list|add|edit|del)"`)
	pageInitInitPattern := regexp.MustCompile(`\.GET\("/page-init",\s*handler\.([A-Za-z]*Init)\)`)
	pageDictionaryFuncPattern := regexp.MustCompile(`func \([^)]*\*(Handler|Service)[^)]*\) ([A-Za-z]*Init)\(`)

	err := filepath.WalkDir(moduleRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(body)
		rel := filepath.ToSlash(mustRel(t, root, path))

		for _, match := range routeActionPattern.FindAllStringSubmatchIndex(source, -1) {
			violations = append(violations, formatGoViolation(source, rel, match[0], "legacy CRUD action URL segment"))
		}

		if strings.HasSuffix(filepath.ToSlash(path), "/internal/module/user/transport/admin/route.go") {
			source = strings.ReplaceAll(source, `users.GET("/init", handler.Init)`, "")
		}
		for _, match := range pageInitInitPattern.FindAllStringSubmatchIndex(source, -1) {
			name := source[match[2]:match[3]]
			if name == "PageInit" || strings.HasSuffix(name, "PageInit") {
				continue
			}
			violations = append(violations, formatGoViolation(source, rel, match[0], "/page-init bound to handler."+name))
		}
		for _, match := range pageDictionaryFuncPattern.FindAllStringSubmatchIndex(source, -1) {
			name := source[match[4]:match[5]]
			if name == "Init" && strings.HasPrefix(filepath.ToSlash(rel), "internal/module/user/") {
				continue
			}
			if name == "PageInit" || strings.HasSuffix(name, "PageInit") {
				continue
			}
			violations = append(violations, formatGoViolation(source, rel, match[0], "page dictionary function must be PageInit-style, not "+name))
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk module files: %v", err)
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("RESTful route naming violations:\n  %s", strings.Join(violations, "\n  "))
	}
}

func mustRel(t *testing.T, base, target string) string {
	t.Helper()
	rel, err := filepath.Rel(base, target)
	if err != nil {
		t.Fatalf("rel %s from %s: %v", target, base, err)
	}
	return rel
}

func formatGoViolation(source, rel string, offset int, reason string) string {
	return rel + ":" + lineNumber(source, offset) + ": " + reason
}

func lineNumber(source string, offset int) string {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	return strconv.Itoa(strings.Count(source[:offset], "\n") + 1)
}
