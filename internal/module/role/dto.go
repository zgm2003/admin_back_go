package role

import (
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/pagination"
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

type ListResponse pagination.Result[ListItem]

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
