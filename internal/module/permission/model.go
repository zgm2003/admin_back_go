package permission

const (
	CommonYes = 1
	CommonNo  = 2

	TypeDir    = 1
	TypePage   = 2
	TypeButton = 3

	RootParentID = 0
	StatusActive = 1

	ButtonCacheKeySchema = "rbac_page_grants"
)

type Permission struct {
	ID        int64  `gorm:"column:id"`
	Name      string `gorm:"column:name"`
	Path      string `gorm:"column:path"`
	Icon      string `gorm:"column:icon"`
	ParentID  int64  `gorm:"column:parent_id"`
	Component string `gorm:"column:component"`
	Platform  string `gorm:"column:platform"`
	Type      int    `gorm:"column:type"`
	Sort      int    `gorm:"column:sort"`
	Code      string `gorm:"column:code"`
	I18nKey   string `gorm:"column:i18n_key"`
	ShowMenu  int    `gorm:"column:show_menu"`
	Status    int    `gorm:"column:status"`
	IsDel     int    `gorm:"column:is_del"`
}

func (Permission) TableName() string {
	return "permissions"
}

type RolePermission struct {
	ID           int64 `gorm:"column:id"`
	RoleID       int64 `gorm:"column:role_id"`
	PermissionID int64 `gorm:"column:permission_id"`
	IsDel        int   `gorm:"column:is_del"`
}

func (RolePermission) TableName() string {
	return "role_permissions"
}

type Context struct {
	Permissions []MenuItem  `json:"permissions"`
	Router      []RouteItem `json:"router"`
	ButtonCodes []string    `json:"buttonCodes"`
}

type MenuItem struct {
	Index    string     `json:"index"`
	Label    string     `json:"label"`
	Path     string     `json:"path"`
	Icon     string     `json:"icon"`
	Children []MenuItem `json:"children"`
	I18nKey  string     `json:"i18n_key"`
	Sort     int        `json:"sort"`
	ShowMenu int        `json:"show_menu"`
	ParentID int64      `json:"parent_id"`
}

type RouteItem struct {
	Name    string            `json:"name"`
	Path    string            `json:"path"`
	ViewKey string            `json:"view_key"`
	Meta    map[string]string `json:"meta"`
}
