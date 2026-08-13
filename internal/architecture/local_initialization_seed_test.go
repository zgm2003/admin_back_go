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
	id        int64
	name      string
	path      string
	icon      string
	parentID  int64
	component string
	platform  string
	typeID    int
	sort      int
	code      string
	i18nKey   string
	showMenu  int
	status    int
	isDel     int
}

func TestLocalPermissionSeed(t *testing.T) {
	root := backendRoot(t)
	seedPath := filepath.Join(root, "database", "seeds", "admin_permissions.sql")
	body, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("read local permission seed: %v", err)
	}
	seed := string(body)
	normalized := strings.Join(strings.Fields(strings.ToLower(strings.NewReplacer("`", "", "\r", " ", "\n", " ", "\t", " ").Replace(seed))), " ")
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
	if len(rows) != 138 {
		t.Fatalf("permission seed row count=%d want 138", len(rows))
	}

	ids := make(map[int64]struct{}, len(rows))
	codes := make(map[string]int64, len(rows))
	rowsByID := make(map[int64]permissionSeedRow, len(rows))
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
		rowsByID[row.id] = row
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
	if len(codes) != 108 {
		t.Fatalf("permission seed code count=%d want 108", len(codes))
	}

	officialModels := map[int64]permissionSeedRow{
		921: {id: 921, name: "官方模型", path: "/ai/official-models", icon: "", parentID: 5, component: "ai/official-models", platform: "admin", typeID: 2, sort: 7, code: "ai_official_model_list", i18nKey: "menu.ai_official_models", showMenu: 1, status: 1, isDel: 2},
		922: {id: 922, name: "同步官方模型价格", path: "", icon: "", parentID: 921, component: "", platform: "admin", typeID: 3, sort: 1, code: "ai_official_model_price_sync", i18nKey: "", showMenu: 2, status: 1, isDel: 2},
	}
	for id, want := range officialModels {
		got, ok := rowsByID[id]
		if !ok {
			t.Fatalf("AI official model permission %d is missing", id)
		}
		if got != want {
			t.Fatalf("AI official model permission %d=%+v want %+v", id, got, want)
		}
	}

	walletRedeemCodes := map[int64]permissionSeedRow{
		912: {id: 912, name: "兑换码管理", path: "/payment/redeem-codes", icon: "Ticket", parentID: 437, component: "payment/redeem-codes", platform: "admin", typeID: 2, sort: 35, code: "payment_redeem_code_list", i18nKey: "menu.payment_redeem_codes", showMenu: 1, status: 1, isDel: 2},
		913: {id: 913, name: "批量生成兑换码", path: "", icon: "", parentID: 912, component: "", platform: "admin", typeID: 3, sort: 1, code: "payment_redeem_code_generate", i18nKey: "", showMenu: 2, status: 1, isDel: 2},
		914: {id: 914, name: "作废兑换码", path: "", icon: "", parentID: 912, component: "", platform: "admin", typeID: 3, sort: 2, code: "payment_redeem_code_void", i18nKey: "", showMenu: 2, status: 1, isDel: 2},
	}
	for id, want := range walletRedeemCodes {
		got, ok := rowsByID[id]
		if !ok {
			t.Fatalf("wallet redeem-code permission %d is missing", id)
		}
		if got != want {
			t.Fatalf("wallet redeem-code permission %d=%+v want %+v", id, got, want)
		}
	}

	wantMailDiagnostic := permissionSeedRow{
		id: 515, name: "查看邮件日志及验证码", path: "", icon: "", parentID: 506,
		component: "", platform: "admin", typeID: 3, sort: 9,
		code: "system_mail_logView", i18nKey: "", showMenu: 2, status: 1, isDel: 2,
	}
	if got, ok := rowsByID[wantMailDiagnostic.id]; !ok {
		t.Fatal("mail diagnostic permission 515 is missing")
	} else if got != wantMailDiagnostic {
		t.Fatalf("mail diagnostic permission 515=%+v want %+v", got, wantMailDiagnostic)
	}

	restored := map[int64]permissionSeedRow{
		4:  {id: 4, name: "组件演示", path: "", icon: "Menu", parentID: 0, component: "", platform: "admin", typeID: 1, sort: 4, code: "", i18nKey: "menu.component", showMenu: 1, status: 1, isDel: 2},
		40: {id: 40, name: "上传", path: "/component/upload", icon: "", parentID: 4, component: "component/upload", platform: "admin", typeID: 2, sort: 1, code: "", i18nKey: "menu.component_upload", showMenu: 1, status: 1, isDel: 2},
		41: {id: 41, name: "表单", path: "/component/form", icon: "", parentID: 4, component: "component/form", platform: "admin", typeID: 2, sort: 2, code: "", i18nKey: "menu.component_form", showMenu: 1, status: 1, isDel: 2},
		42: {id: 42, name: "展示", path: "/component/display", icon: "", parentID: 4, component: "component/display", platform: "admin", typeID: 2, sort: 3, code: "", i18nKey: "menu.component_display", showMenu: 1, status: 1, isDel: 2},
		43: {id: 43, name: "特效", path: "/component/effect", icon: "", parentID: 4, component: "component/effect", platform: "admin", typeID: 2, sort: 4, code: "", i18nKey: "menu.component_effect", showMenu: 1, status: 1, isDel: 2},
		80: {id: 80, name: "下载管理器", path: "/component/download", icon: "", parentID: 4, component: "component/download", platform: "admin", typeID: 2, sort: 5, code: "", i18nKey: "menu.component_download", showMenu: 1, status: 1, isDel: 2},
	}
	for id, want := range restored {
		got, ok := rowsByID[id]
		if !ok {
			t.Fatalf("restored component permission %d is missing", id)
		}
		if got != want {
			t.Fatalf("restored component permission %d=%+v want %+v", id, got, want)
		}
	}
}

func TestLocalSystemSettingSeedPreservesDefaultAvatar(t *testing.T) {
	root := backendRoot(t)
	seedPath := filepath.Join(root, "database", "seed.sql")
	body, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("read local initialization seed: %v", err)
	}
	normalized := strings.Join(strings.Fields(strings.ToLower(string(body))), " ")
	want := "(1, 'user.default_avatar', 'https://cos.zgm2003.cn/avatars/1769948592140-20.png', 1, '用户注册头像', 1, 2)"
	if !strings.Contains(normalized, strings.ToLower(strings.Join(strings.Fields(want), " "))) {
		t.Fatal("local initialization seed must preserve the active default avatar system setting")
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
		sort, err := parseSeedInt(fields[8], "sort")
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
			id:        id,
			name:      sqlSeedString(fields[1]),
			path:      sqlSeedString(fields[2]),
			icon:      sqlSeedString(fields[3]),
			parentID:  parentID,
			component: sqlSeedString(fields[5]),
			platform:  sqlSeedString(fields[6]),
			typeID:    int(typeID),
			sort:      int(sort),
			code:      sqlSeedString(fields[9]),
			i18nKey:   sqlSeedString(fields[10]),
			showMenu:  int(showMenu),
			status:    int(status),
			isDel:     int(isDel),
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
