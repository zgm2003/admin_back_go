package permission

type DictOption[T string | int] struct {
	Label string `json:"label"`
	Value T      `json:"value"`
}

type InitResponse struct {
	Dict PermissionDict `json:"dict"`
}

type PermissionDict struct {
	PermissionTree        []PermissionTreeNode `json:"permission_tree"`
	PermissionTypeArr     []DictOption[int]    `json:"permission_type_arr"`
	PermissionPlatformArr []DictOption[string] `json:"permission_platform_arr"`
}

type PermissionTreeNode struct {
	ID       int64                `json:"id"`
	Label    string               `json:"label"`
	Value    int64                `json:"value"`
	ParentID int64                `json:"parent_id"`
	Platform string               `json:"platform"`
	Type     int                  `json:"type"`
	Code     string               `json:"code,omitempty"`
	Children []PermissionTreeNode `json:"children,omitempty"`
}

type PermissionListQuery struct {
	Platform string
	Name     string
	Path     string
	Type     int
}

type PermissionListItem struct {
	ID        int64                `json:"id"`
	Name      string               `json:"name"`
	Path      string               `json:"path"`
	ParentID  int64                `json:"parent_id"`
	Icon      string               `json:"icon"`
	Component string               `json:"component"`
	Status    int                  `json:"status"`
	Type      int                  `json:"type"`
	TypeName  string               `json:"type_name"`
	Code      string               `json:"code"`
	I18nKey   string               `json:"i18n_key"`
	Sort      int                  `json:"sort"`
	ShowMenu  int                  `json:"show_menu"`
	Children  []PermissionListItem `json:"children,omitempty"`
}

type PermissionMutationInput struct {
	Platform  string
	Type      int
	Name      string
	ParentID  int64
	Icon      string
	Path      string
	Component string
	I18nKey   string
	Code      string
	Sort      int
	ShowMenu  int
}
