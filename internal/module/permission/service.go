package permission

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"admin_back_go/internal/apperror"
)

type Service struct {
	repository       Repository
	allowedPlatforms map[string]struct{}
}

func NewService(repository Repository, allowedPlatforms []string) *Service {
	if len(allowedPlatforms) == 0 {
		allowedPlatforms = []string{"admin", "app"}
	}
	allowed := make(map[string]struct{}, len(allowedPlatforms))
	for _, platform := range allowedPlatforms {
		platform = strings.TrimSpace(platform)
		if platform != "" {
			allowed[platform] = struct{}{}
		}
	}
	return &Service{repository: repository, allowedPlatforms: allowed}
}

func (s *Service) BuildContextByRole(ctx context.Context, roleID int64, platform string) (Context, *apperror.Error) {
	if roleID <= 0 {
		return Context{}, nil
	}
	if s == nil || s.repository == nil {
		return Context{}, apperror.Internal("权限仓储未配置")
	}

	role, err := s.repository.FindRole(ctx, roleID)
	if err != nil {
		return Context{}, apperror.Wrap(apperror.CodeInternal, 500, "查询角色失败", err)
	}
	if role == nil {
		return Context{}, nil
	}

	platform = strings.TrimSpace(platform)
	if !s.isAllowedPlatform(platform) {
		return Context{}, apperror.BadRequest("无效的平台标识: " + platform)
	}

	grantedIDs, err := s.repository.PermissionIDsByRoleID(ctx, roleID)
	if err != nil {
		return Context{}, apperror.Wrap(apperror.CodeInternal, 500, "查询角色权限失败", err)
	}
	grantedIDs = normalizeIDs(grantedIDs)
	if len(grantedIDs) == 0 {
		return Context{}, nil
	}

	allPermissions, err := s.repository.AllActivePermissions(ctx)
	if err != nil {
		return Context{}, apperror.Wrap(apperror.CodeInternal, 500, "查询权限失败", err)
	}

	permMap := permissionMapByPlatform(allPermissions, platform)
	enabledIDs := resolveEnabledIDs(grantedIDs, permMap)
	return buildContext(enabledIDs, permMap), nil
}

func (s *Service) isAllowedPlatform(platform string) bool {
	if len(s.allowedPlatforms) == 0 {
		return true
	}
	_, ok := s.allowedPlatforms[platform]
	return ok
}

func ButtonCacheKey(userID int64, platform string) string {
	return fmt.Sprintf("auth_perm_uid_%d_%s_%s", userID, platform, ButtonCacheKeySchema)
}

func permissionMapByPlatform(permissions []Permission, platform string) map[int64]Permission {
	result := make(map[int64]Permission, len(permissions))
	for _, permission := range permissions {
		if permission.Platform != platform {
			continue
		}
		if permission.ID <= 0 {
			continue
		}
		result[permission.ID] = permission
	}
	return result
}

func normalizeIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func resolveEnabledIDs(grantedIDs []int64, permMap map[int64]Permission) []int64 {
	included := make(map[int64]struct{}, len(grantedIDs))
	result := make([]int64, 0, len(grantedIDs))

	include := func(id int64) {
		if _, ok := included[id]; ok {
			return
		}
		included[id] = struct{}{}
		result = append(result, id)
	}

	for _, grantedID := range grantedIDs {
		current := grantedID
		if _, ok := permMap[current]; !ok {
			continue
		}

		path := make([]int64, 0, 4)
		visited := map[int64]struct{}{}
		valid := false

		for {
			permission, ok := permMap[current]
			if !ok {
				break
			}
			if _, ok := visited[current]; ok {
				break
			}
			visited[current] = struct{}{}
			path = append(path, current)

			if _, ok := included[current]; ok {
				valid = true
				break
			}
			if permission.ParentID == RootParentID {
				valid = true
				break
			}
			current = permission.ParentID
		}

		if valid {
			for _, id := range path {
				include(id)
			}
		}
	}

	return result
}

func buildContext(enabledIDs []int64, permMap map[int64]Permission) Context {
	menus := make([]Permission, 0, len(enabledIDs))
	router := make([]RouteItem, 0)
	buttonCodes := make([]string, 0)
	seenButton := map[string]struct{}{}

	for _, id := range enabledIDs {
		permission, ok := permMap[id]
		if !ok {
			continue
		}

		if permission.Type == TypeButton && strings.TrimSpace(permission.Code) != "" {
			if _, ok := seenButton[permission.Code]; !ok {
				seenButton[permission.Code] = struct{}{}
				buttonCodes = append(buttonCodes, permission.Code)
			}
		}

		if permission.Type == TypePage && strings.TrimSpace(permission.Path) != "" && strings.TrimSpace(permission.Component) != "" {
			router = append(router, buildRouteRecord(permission))
		}

		if permission.Type == TypeDir || permission.Type == TypePage {
			menus = append(menus, permission)
		}
	}

	sort.SliceStable(menus, func(i, j int) bool {
		return menus[i].Sort < menus[j].Sort
	})

	return Context{
		Permissions: buildPermissionTree(menus),
		Router:      router,
		ButtonCodes: buttonCodes,
	}
}

func buildRouteRecord(permission Permission) RouteItem {
	return RouteItem{
		Name:    "menu_" + strconv.FormatInt(permission.ID, 10),
		Path:    permission.Path,
		ViewKey: strings.TrimLeft(permission.Component, "/"),
		Meta: map[string]string{
			"menuId": strconv.FormatInt(permission.ID, 10),
		},
	}
}

func buildPermissionTree(items []Permission) []MenuItem {
	if len(items) == 0 {
		return []MenuItem{}
	}

	nodes := make(map[int64]MenuItem, len(items))
	childrenByParent := make(map[int64][]int64, len(items))
	rootIDs := make([]int64, 0)

	for _, item := range items {
		showMenu := item.ShowMenu
		if showMenu == 0 {
			showMenu = CommonYes
		}
		nodes[item.ID] = MenuItem{
			Index:    strconv.FormatInt(item.ID, 10),
			Label:    item.Name,
			Path:     item.Path,
			Icon:     item.Icon,
			Children: []MenuItem{},
			I18nKey:  item.I18nKey,
			Sort:     item.Sort,
			ShowMenu: showMenu,
			ParentID: item.ParentID,
		}

		if item.ParentID == RootParentID {
			rootIDs = append(rootIDs, item.ID)
			continue
		}
		childrenByParent[item.ParentID] = append(childrenByParent[item.ParentID], item.ID)
	}

	var buildNode func(id int64) MenuItem
	buildNode = func(id int64) MenuItem {
		node := nodes[id]
		childIDs := childrenByParent[id]
		node.Children = make([]MenuItem, 0, len(childIDs))
		for _, childID := range childIDs {
			if _, ok := nodes[childID]; !ok {
				continue
			}
			node.Children = append(node.Children, buildNode(childID))
		}
		return node
	}

	tree := make([]MenuItem, 0, len(rootIDs))
	for _, rootID := range rootIDs {
		tree = append(tree, buildNode(rootID))
	}
	return tree
}
