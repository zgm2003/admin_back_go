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

type UpdateInput struct {
	Username      string
	Avatar        string
	RoleID        int64
	Sex           int
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
