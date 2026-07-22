package architecture

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type permissionSeedRow struct {
	id       int64
	parentID int64
	platform string
	typeID   int
	code     string
	showMenu int
	status   int
	isDel    int
}

func TestLocalPermissionSeed(t *testing.T) {
	root := backendRoot(t)
	seedPath := filepath.Join(root, "database", "seeds", "admin_permissions.sql")
	body, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("read local permission seed: %v", err)
	}
	seed := string(body)
	normalized := strings.ToLower(strings.NewReplacer("`", "", "\r", " ", "\n", " ", "\t", " ").Replace(seed))
	for _, table := range []string{"users", "roles", "role_permissions"} {
		for _, statement := range []string{
			"insert into " + table,
			"update " + table,
			"delete from " + table,
			"replace into " + table,
			"truncate table " + table,
		} {
			if strings.Contains(normalized, statement) {
				t.Fatalf("permission seed contains forbidden write %q", statement)
			}
		}
	}
	for _, retired := range []string{"'app'", "'canvas'", "clientversion", "client_version"} {
		if strings.Contains(normalized, retired) {
			t.Fatalf("permission seed contains retired surface %q", retired)
		}
	}

	guardIndex := strings.Index(normalized, "create temporary table _admin_permission_seed_guard")
	guardSentinelIndex := strings.Index(normalized, "insert into _admin_permission_seed_guard (value) values (1)")
	guardCollisionIndex := strings.Index(normalized, "insert into _admin_permission_seed_guard (value) select 1 from permissions limit 1")
	insertIndex := strings.Index(normalized, "insert into permissions")
	if guardIndex < 0 || guardSentinelIndex < guardIndex || guardCollisionIndex < guardSentinelIndex || insertIndex < guardCollisionIndex {
		t.Fatal("permission seed must guard an empty table before inserting permissions")
	}

	rows, err := parsePermissionSeedRows(seed)
	if err != nil {
		t.Fatalf("parse local permission seed: %v", err)
	}
	if len(rows) != 125 {
		t.Fatalf("permission seed row count=%d want 125", len(rows))
	}

	ids := make(map[int64]struct{}, len(rows))
	codes := make(map[string]int64, len(rows))
	var previousID int64
	for _, row := range rows {
		if row.id <= previousID {
			t.Fatalf("permission seed ids are not strictly ordered: %d after %d", row.id, previousID)
		}
		previousID = row.id
		if _, exists := ids[row.id]; exists {
			t.Fatalf("permission seed contains duplicate id %d", row.id)
		}
		ids[row.id] = struct{}{}
		if row.platform != "admin" || row.status != 1 || row.isDel != 2 {
			t.Fatalf("permission %d has invalid lifecycle fields", row.id)
		}
		if row.typeID < 1 || row.typeID > 3 {
			t.Fatalf("permission %d has invalid type %d", row.id, row.typeID)
		}
		if row.showMenu != 1 && row.showMenu != 2 {
			t.Fatalf("permission %d has invalid show_menu %d", row.id, row.showMenu)
		}
		if row.code != "" {
			if existingID, exists := codes[row.code]; exists {
				t.Fatalf("permission code %q is duplicated by ids %d and %d", row.code, existingID, row.id)
			}
			codes[row.code] = row.id
		}
	}
	for _, row := range rows {
		if _, ok := ids[row.parentID]; row.parentID != 0 && !ok {
			t.Fatalf("permission %d references missing parent %d", row.id, row.parentID)
		}
	}
	if len(codes) != 101 {
		t.Fatalf("permission seed code count=%d want 101", len(codes))
	}
}

func parsePermissionSeedRows(seed string) ([]permissionSeedRow, error) {
	const marker = "INSERT INTO `permissions`"
	insertIndex := strings.Index(seed, marker)
	if insertIndex < 0 {
		return nil, fmt.Errorf("permission insert is missing")
	}
	valuesIndex := strings.Index(seed[insertIndex:], "VALUES")
	if valuesIndex < 0 {
		return nil, fmt.Errorf("permission values clause is missing")
	}

	var rows []permissionSeedRow
	scanner := bufio.NewScanner(strings.NewReader(seed[insertIndex+valuesIndex+len("VALUES"):]))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "(") {
			break
		}
		fields, err := splitSQLTuple(line)
		if err != nil {
			return nil, err
		}
		if len(fields) != 14 {
			return nil, fmt.Errorf("permission tuple has %d fields, want 14", len(fields))
		}
		id, err := parseSeedInt(fields[0], "id")
		if err != nil {
			return nil, err
		}
		parentID, err := parseSeedInt(fields[4], "parent_id")
		if err != nil {
			return nil, err
		}
		typeID, err := parseSeedInt(fields[7], "type")
		if err != nil {
			return nil, err
		}
		showMenu, err := parseSeedInt(fields[11], "show_menu")
		if err != nil {
			return nil, err
		}
		status, err := parseSeedInt(fields[12], "status")
		if err != nil {
			return nil, err
		}
		isDel, err := parseSeedInt(fields[13], "is_del")
		if err != nil {
			return nil, err
		}
		rows = append(rows, permissionSeedRow{
			id:       id,
			parentID: parentID,
			platform: sqlSeedString(fields[6]),
			typeID:   int(typeID),
			code:     sqlSeedString(fields[9]),
			showMenu: int(showMenu),
			status:   int(status),
			isDel:    int(isDel),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func splitSQLTuple(line string) ([]string, error) {
	line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(line, ","), ";"))
	if len(line) < 2 || line[0] != '(' || line[len(line)-1] != ')' {
		return nil, fmt.Errorf("invalid permission tuple %q", line)
	}
	line = line[1 : len(line)-1]

	fields := make([]string, 0, 14)
	start := 0
	inQuote := false
	escaped := false
	for index := 0; index < len(line); index++ {
		character := line[index]
		if escaped {
			escaped = false
			continue
		}
		if inQuote && character == '\\' {
			escaped = true
			continue
		}
		if character == '\'' {
			inQuote = !inQuote
			continue
		}
		if character == ',' && !inQuote {
			fields = append(fields, strings.TrimSpace(line[start:index]))
			start = index + 1
		}
	}
	if inQuote || escaped {
		return nil, fmt.Errorf("unterminated permission tuple string")
	}
	fields = append(fields, strings.TrimSpace(line[start:]))
	return fields, nil
}

func parseSeedInt(value string, field string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse permission %s: %w", field, err)
	}
	return parsed, nil
}

func sqlSeedString(value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "NULL") {
		return ""
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		value = value[1 : len(value)-1]
	}
	value = strings.ReplaceAll(value, "\\'", "'")
	value = strings.ReplaceAll(value, "\\\\", "\\")
	return value
}
