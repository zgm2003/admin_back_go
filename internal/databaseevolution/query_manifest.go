package databaseevolution

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type QueryCandidate struct {
	Name               string         `json:"name"`
	RepositoryFile     string         `json:"repository_file"`
	SQL                string         `json:"sql"`
	Bindings           map[string]any `json:"bindings"`
	ExpectedOrder      []string       `json:"expected_order"`
	RowDistributionSQL string         `json:"row_distribution_sql"`
	ProposedIndex      string         `json:"proposed_index"`
	MaxRowsExamined    uint64         `json:"max_rows_examined"`
	MaxP95MS           uint64         `json:"max_p95_ms"`
}

var (
	queryCandidateNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,95}$`)
	bindingNamePattern        = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	selectStarPattern         = regexp.MustCompile(`(?i)\bselect\s+(?:distinct\s+)?\*`)
	createIndexPattern        = regexp.MustCompile(`(?is)^\s*create\s+(?:unique\s+)?index\s+[a-zA-Z0-9_]+\s+on\s+[a-zA-Z0-9_]+\s*\([^)]+\)\s*;?\s*$`)
)

func LoadQueryManifest(path string, repositoryRoot string) ([]QueryCandidate, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open query manifest: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var candidates []QueryCandidate
	if err := decoder.Decode(&candidates); err != nil {
		return nil, fmt.Errorf("decode query manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if err := ValidateQueryCandidates(candidates, repositoryRoot); err != nil {
		return nil, err
	}
	return candidates, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode query manifest trailer: %w", err)
	}
	return fmt.Errorf("query manifest contains trailing JSON")
}

func ValidateQueryCandidates(candidates []QueryCandidate, repositoryRoot string) error {
	if len(candidates) == 0 {
		return fmt.Errorf("query manifest contains no candidates")
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	names := make(map[string]struct{}, len(candidates))
	for index := range candidates {
		candidate := &candidates[index]
		candidate.Name = strings.TrimSpace(candidate.Name)
		if !queryCandidateNamePattern.MatchString(candidate.Name) {
			return fmt.Errorf("candidate %d has invalid name", index+1)
		}
		if _, exists := names[candidate.Name]; exists {
			return fmt.Errorf("duplicate candidate name %q", candidate.Name)
		}
		names[candidate.Name] = struct{}{}
		if err := validateRepositoryFile(candidate, root); err != nil {
			return fmt.Errorf("candidate %q: %w", candidate.Name, err)
		}
		if err := validateCandidateSQL(*candidate); err != nil {
			return fmt.Errorf("candidate %q: %w", candidate.Name, err)
		}
	}
	return nil
}

func validateRepositoryFile(candidate *QueryCandidate, root string) error {
	raw := strings.TrimSpace(candidate.RepositoryFile)
	if raw == "" || filepath.IsAbs(raw) {
		return fmt.Errorf("repository_file must be repository-relative")
	}
	normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(raw)))
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") || strings.Contains(normalized, "/../") {
		return fmt.Errorf("repository_file must not contain parent traversal")
	}
	if !strings.HasPrefix(normalized, "internal/module/") || !strings.EqualFold(filepath.Ext(normalized), ".go") {
		return fmt.Errorf("repository_file must be a Go file under internal/module")
	}
	fullPath := filepath.Join(root, filepath.FromSlash(normalized))
	resolved, err := filepath.Abs(fullPath)
	if err != nil || (resolved != root && !strings.HasPrefix(strings.ToLower(resolved), strings.ToLower(root+string(filepath.Separator)))) {
		return fmt.Errorf("repository_file resolves outside repository")
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return fmt.Errorf("repository_file does not exist")
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repository_file must be a regular file")
	}
	candidate.RepositoryFile = normalized
	return nil
}

func validateCandidateSQL(candidate QueryCandidate) error {
	sqlText := strings.TrimSpace(candidate.SQL)
	if sqlText == "" || !strings.HasPrefix(strings.ToUpper(sqlText), "SELECT ") {
		return fmt.Errorf("sql must be a SELECT statement")
	}
	if selectStarPattern.MatchString(sqlText) {
		return fmt.Errorf("sql must not contain SELECT *")
	}
	if len(candidate.Bindings) == 0 {
		return fmt.Errorf("bindings must not be empty")
	}
	for name, value := range candidate.Bindings {
		if !bindingNamePattern.MatchString(name) || !strings.Contains(sqlText, ":"+name) {
			return fmt.Errorf("binding %q is invalid or unused", name)
		}
		if value == nil {
			return fmt.Errorf("bindings must not contain null values")
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			return fmt.Errorf("bindings must not contain empty strings")
		}
	}
	if len(candidate.ExpectedOrder) == 0 {
		return fmt.Errorf("expected_order must not be empty")
	}
	hasIDTieBreaker := false
	normalizedSQL := normalizeQueryText(sqlText)
	for _, order := range candidate.ExpectedOrder {
		order = strings.TrimSpace(order)
		if order == "" || !strings.Contains(normalizedSQL, normalizeQueryText(order)) {
			return fmt.Errorf("expected_order is not present in sql")
		}
		field := strings.Fields(strings.ToLower(strings.ReplaceAll(order, "`", "")))[0]
		if field == "id" || strings.HasSuffix(field, ".id") {
			hasIDTieBreaker = true
		}
	}
	if !hasIDTieBreaker {
		return fmt.Errorf("expected_order requires an id tie-breaker")
	}
	distribution := strings.TrimSpace(candidate.RowDistributionSQL)
	if distribution == "" || !strings.HasPrefix(strings.ToUpper(distribution), "SELECT ") || selectStarPattern.MatchString(distribution) {
		return fmt.Errorf("row_distribution_sql must be an explicit SELECT")
	}
	ddl := strings.TrimSpace(candidate.ProposedIndex)
	if !createIndexPattern.MatchString(ddl) || strings.Count(ddl, ";") > 1 {
		return fmt.Errorf("proposed_index must contain one CREATE INDEX statement")
	}
	if candidate.MaxRowsExamined == 0 || candidate.MaxP95MS == 0 {
		return fmt.Errorf("query budgets must be positive")
	}
	return nil
}

func normalizeQueryText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(value, "`", ""))), " ")
}

func QueryManifestFiles(candidates []QueryCandidate) []string {
	unique := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		unique[candidate.RepositoryFile] = struct{}{}
	}
	files := make([]string, 0, len(unique))
	for file := range unique {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}
