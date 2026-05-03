package role

import (
	"admin_back_go/internal/dict"
	"admin_back_go/internal/module/permission"
)

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	PermissionTree        []permission.PermissionTreeNode `json:"permission_tree"`
	PermissionPlatformArr []dict.Option[string]           `json:"permission_platform_arr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Name        string
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
	Name          string  `json:"name"`
	PermissionIDs []int64 `json:"permission_id"`
	IsDefault     int     `json:"is_default"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type MutationInput struct {
	Name          string
	PermissionIDs []int64
}
