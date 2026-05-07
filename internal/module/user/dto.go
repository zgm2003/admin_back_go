package user

import (
	"time"

	"admin_back_go/internal/dict"
	"admin_back_go/internal/module/permission"
)

type InitInput struct {
	UserID   int64
	Platform string
}

type InitResponse struct {
	UserID      int64                  `json:"user_id"`
	Username    string                 `json:"username"`
	Avatar      string                 `json:"avatar"`
	RoleName    string                 `json:"role_name"`
	Permissions []permission.MenuItem  `json:"permissions"`
	Router      []permission.RouteItem `json:"router"`
	ButtonCodes []string               `json:"buttonCodes"`
	QuickEntry  []QuickEntry           `json:"quick_entry"`
}

type RoleOption = dict.Option[int]
type SexOption = dict.Option[int]
type PlatformOption = dict.Option[string]

type PageInitResponse struct {
	Dict PageInitDict `json:"dict"`
}

type PageInitDict struct {
	RoleArr         []RoleOption      `json:"roleArr"`
	AuthAddressTree []AddressTreeNode `json:"auth_address_tree"`
	SexArr          []SexOption       `json:"sexArr"`
	PlatformArr     []PlatformOption  `json:"platformArr"`
}

type VerifyTypeOption = dict.Option[string]

type UpdatePasswordInput struct {
	UserID          int64
	VerifyType      string
	OldPassword     string
	Account         string
	Code            string
	NewPassword     string
	ConfirmPassword string
}

type UpdateEmailInput struct {
	UserID int64
	Email  string
	Code   string
}

type UpdatePhoneInput struct {
	UserID int64
	Phone  string
	Code   string
}

type ProfileResponse struct {
	Profile ProfileDetail `json:"profile"`
	Dict    ProfileDict   `json:"dict"`
}

type ProfileDict struct {
	AuthAddressTree []AddressTreeNode  `json:"auth_address_tree"`
	SexArr          []SexOption        `json:"sexArr"`
	VerifyTypeArr   []VerifyTypeOption `json:"verify_type_arr"`
}

type ProfileDetail struct {
	UserID        int64  `json:"user_id"`
	Username      string `json:"username"`
	Email         string `json:"email"`
	Avatar        string `json:"avatar"`
	Phone         string `json:"phone"`
	RoleID        int64  `json:"role_id"`
	RoleName      string `json:"role_name"`
	AddressID     int64  `json:"address_id"`
	DetailAddress string `json:"detail_address"`
	Sex           int    `json:"sex"`
	Birthday      string `json:"birthday"`
	Bio           string `json:"bio"`
	IsSelf        int    `json:"is_self"`
	HasPassword   bool   `json:"has_password"`
}

type AddressTreeNode struct {
	ID       int64             `json:"id"`
	ParentID int64             `json:"parent_id"`
	Label    string            `json:"label"`
	Value    int64             `json:"value"`
	Children []AddressTreeNode `json:"children,omitempty"`
}

type ListQuery struct {
	CurrentPage   int
	PageSize      int
	Keyword       string
	Username      string
	Email         string
	DetailAddress string
	AddressIDs    []int64
	RoleID        int64
	Sex           *int
	DateRange     []string
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []ListItem `json:"list"`
	Page Page       `json:"page"`
}

type ListItem struct {
	ID            int64   `json:"id"`
	Username      string  `json:"username"`
	Email         string  `json:"email"`
	Phone         string  `json:"phone"`
	Avatar        *string `json:"avatar"`
	Sex           int     `json:"sex"`
	SexShow       string  `json:"sex_show"`
	RoleID        int64   `json:"role_id"`
	RoleName      string  `json:"role_name"`
	Bio           string  `json:"bio"`
	AddressShow   string  `json:"address_show"`
	AddressID     int64   `json:"address_id"`
	DetailAddress string  `json:"detail_address"`
	Status        int     `json:"status"`
	CreatedAt     string  `json:"created_at"`
}

type ListRow struct {
	ID            int64
	Username      string
	Email         string
	Phone         string
	Status        int
	RoleID        int64
	RoleName      string
	Avatar        *string
	Sex           *int
	AddressID     *int64
	DetailAddress *string
	Bio           *string
	CreatedAt     time.Time
}

type ExportInput struct {
	UserID   int64
	Platform string
	IDs      []int64
}

type ExportResponse struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
}

type ExportUserRow struct {
	ID       int64
	Username string
	Email    string
	Phone    string
	Avatar   string
	Sex      int
	RoleName string
}

type UpdateInput struct {
	Username      string
	Avatar        string
	RoleID        int64
	Sex           int
	AddressID     int64
	DetailAddress string
	Bio           string
}

type UpdateProfileInput struct {
	UserID        int64
	Username      string
	Avatar        string
	Sex           int
	Birthday      *string
	AddressID     int64
	DetailAddress string
	Bio           string
}

type BatchProfileField string

const (
	BatchProfileFieldSex           BatchProfileField = "sex"
	BatchProfileFieldAddressID     BatchProfileField = "address_id"
	BatchProfileFieldDetailAddress BatchProfileField = "detail_address"
)

type BatchProfileUpdate struct {
	IDs           []int64
	Field         BatchProfileField
	Sex           int
	AddressID     int64
	DetailAddress string
}
