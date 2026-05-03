package permission

import "testing"

func TestPermissionCreateFieldsStoreNullCodeForRowsWithoutButtonCode(t *testing.T) {
	fields := permissionCreateFields(Permission{
		Name:     "系统",
		ParentID: RootParentID,
		Platform: "admin",
		Type:     TypeDir,
		Sort:     1,
		Code:     "",
		I18nKey:  "menu.system",
		ShowMenu: CommonYes,
		Status:   CommonYes,
		IsDel:    CommonNo,
	})

	value, ok := fields["code"]
	if !ok {
		t.Fatalf("expected code field to be explicit")
	}
	if value != nil {
		t.Fatalf("expected blank code to be stored as SQL NULL, got %#v", value)
	}
}

func TestPermissionCreateFieldsKeepButtonCode(t *testing.T) {
	fields := permissionCreateFields(Permission{
		Name:     "新增",
		ParentID: 12,
		Platform: "admin",
		Type:     TypeButton,
		Sort:     1,
		Code:     "permission_permission_add",
		ShowMenu: CommonNo,
		Status:   CommonYes,
		IsDel:    CommonNo,
	})

	if fields["code"] != "permission_permission_add" {
		t.Fatalf("expected button code to be preserved, got %#v", fields["code"])
	}
}

func TestPermissionUpdateFieldsStoreNullCodeWhenClearingButtonCode(t *testing.T) {
	fields := permissionUpdateFields(map[string]any{
		"name": "页面",
		"code": "",
	})

	value, ok := fields["code"]
	if !ok {
		t.Fatalf("expected code field to be explicit")
	}
	if value != nil {
		t.Fatalf("expected cleared code to be stored as SQL NULL, got %#v", value)
	}
}
