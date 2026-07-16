package databaseevolution

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
)

type InvariantCheck struct {
	Name       string
	Violations uint64
}

type InvariantResult struct {
	Name       string
	Violations uint64
	Checks     []InvariantCheck
}

func RunInvariantFile(ctx context.Context, database *sql.DB, path string) (InvariantResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return InvariantResult{}, fmt.Errorf("read invariant file: %w", err)
	}
	result := InvariantResult{}
	var firstViolation *InvariantCheck
	for _, rawStatement := range strings.Split(string(data), ";") {
		statement := strings.TrimSpace(rawStatement)
		if statement == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToUpper(statement), "SELECT ") {
			return result, fmt.Errorf("invariant file must contain SELECT statements")
		}

		var check InvariantCheck
		if err := database.QueryRowContext(ctx, statement).Scan(&check.Name, &check.Violations); err != nil {
			return result, fmt.Errorf("execute invariant query: %w", err)
		}
		result.Name = check.Name
		result.Violations = check.Violations
		result.Checks = append(result.Checks, check)
		if check.Violations != 0 && firstViolation == nil {
			violation := check
			firstViolation = &violation
		}
	}
	if len(result.Checks) == 0 {
		return InvariantResult{}, fmt.Errorf("invariant file contains no SELECT statements")
	}
	if firstViolation != nil {
		result.Name = firstViolation.Name
		result.Violations = firstViolation.Violations
		return result, fmt.Errorf("invariant %q reported %d violations", result.Name, result.Violations)
	}
	return result, nil
}
