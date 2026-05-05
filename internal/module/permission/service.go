package permission

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type Service struct {
	repository       Repository
	allowedPlatforms map[string]struct{}
	cacheInvalidator CacheInvalidator
	platforms        []string
}

type CacheInvalidator interface {
	Delete(ctx context.Context, key string) error
}

type ServiceOption func(*Service)

func WithCacheInvalidator(cacheInvalidator CacheInvalidator) ServiceOption {
	return func(s *Service) {
		s.cacheInvalidator = cacheInvalidator
	}
}

func NewService(repository Repository, allowedPlatforms []string, options ...ServiceOption) *Service {
	if len(allowedPlatforms) == 0 {
		allowedPlatforms = []string{"admin", "app"}
	}
	allowed := make(map[string]struct{}, len(allowedPlatforms))
	platforms := make([]string, 0, len(allowedPlatforms))
	for _, platform := range allowedPlatforms {
		platform = strings.TrimSpace(platform)
		if platform != "" {
			allowed[platform] = struct{}{}
			platforms = append(platforms, platform)
		}
	}
	service := &Service{
		repository:       repository,
		allowedPlatforms: allowed,
		platforms:        normalizePlatformList(platforms),
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) BuildContextByRole(ctx context.Context, roleID int64, platform string) (Context, *apperror.Error) {
	if roleID <= 0 {
		return Context{}, nil
	}
	if s == nil || s.repository == nil {
		return Context{}, apperror.Internal("权限仓储未配置")
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

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("权限仓储未配置")
	}

	allPermissions, err := s.repository.AllActivePermissions(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询权限字典失败", err)
	}

	return &InitResponse{
		Dict: PermissionDict{
			PermissionTree:        buildPermissionTreeNodes(allPermissions, s.platformLabels()),
			PermissionTypeArr:     permissionTypeOptions(),
			PermissionPlatformArr: s.platformOptions(),
		},
	}, nil
}

func (s *Service) List(ctx context.Context, query PermissionListQuery) ([]PermissionListItem, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("权限仓储未配置")
	}

	query = normalizeListQuery(query)
	if !s.isAllowedPlatform(query.Platform) {
		return nil, apperror.BadRequest("无效的平台标识: " + query.Platform)
	}

	rows, err := s.repository.ListPermissions(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询权限失败", err)
	}
	matchedRows := filterPermissions(rows, query)
	treeRows := matchedRows

	if hasPermissionListFilter(query) {
		allRows, err := s.repository.ListPermissions(ctx, PermissionListQuery{Platform: query.Platform})
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询权限失败", err)
		}
		treeRows = includeMatchedAncestors(filterPermissions(allRows, PermissionListQuery{Platform: query.Platform}), matchedRows)
	}

	return buildPermissionListTree(treeRows), nil
}

func (s *Service) Create(ctx context.Context, input PermissionMutationInput) (int64, *apperror.Error) {
	if s == nil || s.repository == nil {
		return 0, apperror.Internal("权限仓储未配置")
	}

	input, appErr := s.normalizeMutationInput(input)
	if appErr != nil {
		return 0, appErr
	}
	if appErr := s.assertValidParentAssignment(ctx, input.Type, input.ParentID, input.Platform, 0); appErr != nil {
		return 0, appErr
	}
	if appErr := s.assertUniqueMutationFields(ctx, input, 0); appErr != nil {
		return 0, appErr
	}

	row := permissionFromMutation(input)
	if input.Type == TypeButton {
		restoredID, appErr := s.restoreSoftDeletedButtonIfPresent(ctx, input, row)
		if appErr != nil {
			return 0, appErr
		}
		if restoredID > 0 {
			return restoredID, nil
		}
	}

	id, err := s.repository.CreatePermission(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增权限失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id int64, input PermissionMutationInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的权限ID")
	}
	if s == nil || s.repository == nil {
		return apperror.Internal("权限仓储未配置")
	}

	existing, err := s.repository.GetPermission(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询权限失败", err)
	}
	if existing == nil {
		return apperror.NotFound("权限不存在")
	}

	input, appErr := s.normalizeMutationInput(input)
	if appErr != nil {
		return appErr
	}
	if appErr := s.assertValidParentAssignment(ctx, input.Type, input.ParentID, input.Platform, id); appErr != nil {
		return appErr
	}
	if appErr := s.assertExistingChildrenCompatible(ctx, input.Type, id); appErr != nil {
		return appErr
	}
	if appErr := s.assertUniqueMutationFields(ctx, input, id); appErr != nil {
		return appErr
	}

	if err := s.repository.UpdatePermission(ctx, id, permissionUpdateMap(input)); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新权限失败", err)
	}
	if appErr := s.invalidatePermissionUsers(ctx, []int64{id}, true); appErr != nil {
		return appErr
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, ids []int64) *apperror.Error {
	if s == nil || s.repository == nil {
		return apperror.Internal("权限仓储未配置")
	}

	ids = normalizeIDsForMutation(ids)
	if len(ids) == 0 {
		return apperror.BadRequest("请选择要删除的权限")
	}

	hasChildren, err := s.repository.HasChildrenOutsideIDs(ctx, ids)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询子权限失败", err)
	}
	if hasChildren {
		return apperror.BadRequest("存在子节点未被勾选，不能删除")
	}

	roleIDs, appErr := s.roleIDsByPermissionIDs(ctx, ids, false)
	if appErr != nil {
		return appErr
	}
	if err := s.repository.DeletePermissions(ctx, ids); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除权限失败", err)
	}
	if appErr := s.invalidateRoleUsers(ctx, roleIDs); appErr != nil {
		return appErr
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的权限ID")
	}
	if status != CommonYes && status != CommonNo {
		return apperror.BadRequest("无效的状态")
	}
	if s == nil || s.repository == nil {
		return apperror.Internal("权限仓储未配置")
	}
	if err := s.repository.UpdatePermission(ctx, id, map[string]any{"status": status}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新权限状态失败", err)
	}
	if appErr := s.invalidatePermissionUsers(ctx, []int64{id}, true); appErr != nil {
		return appErr
	}
	return nil
}

func (s *Service) invalidatePermissionUsers(ctx context.Context, permissionIDs []int64, includeCascade bool) *apperror.Error {
	roleIDs, appErr := s.roleIDsByPermissionIDs(ctx, permissionIDs, includeCascade)
	if appErr != nil {
		return appErr
	}
	return s.invalidateRoleUsers(ctx, roleIDs)
}

func (s *Service) roleIDsByPermissionIDs(ctx context.Context, permissionIDs []int64, includeCascade bool) ([]int64, *apperror.Error) {
	permissionIDs = normalizeIDsForMutation(permissionIDs)
	if len(permissionIDs) == 0 || s.cacheInvalidator == nil {
		return []int64{}, nil
	}
	if includeCascade {
		cascadeIDs, err := s.repository.CascadeIDs(ctx, permissionIDs)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询子权限失败", err)
		}
		permissionIDs = normalizeIDsForMutation(cascadeIDs)
	}
	roleIDs, err := s.repository.RoleIDsByPermissionIDs(ctx, permissionIDs)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询权限角色失败", err)
	}
	return normalizeIDs(roleIDs), nil
}

func (s *Service) invalidateRoleUsers(ctx context.Context, roleIDs []int64) *apperror.Error {
	if s.cacheInvalidator == nil {
		return nil
	}
	roleIDs = normalizeIDs(roleIDs)
	if len(roleIDs) == 0 {
		return nil
	}
	userIDs, err := s.repository.UserIDsByRoleIDs(ctx, roleIDs)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询角色用户失败", err)
	}
	for _, userID := range normalizeIDs(userIDs) {
		for _, platform := range s.platforms {
			if err := s.cacheInvalidator.Delete(ctx, ButtonCacheKey(userID, platform)); err != nil {
				return apperror.Wrap(apperror.CodeInternal, 500, "清理权限缓存失败", err)
			}
		}
	}
	return nil
}

func (s *Service) isAllowedPlatform(platform string) bool {
	if len(s.allowedPlatforms) == 0 {
		return true
	}
	_, ok := s.allowedPlatforms[platform]
	return ok
}

func (s *Service) platformLabels() map[string]string {
	labels := map[string]string{
		"admin": "admin",
		"app":   "app",
	}
	for platform := range s.allowedPlatforms {
		if _, ok := labels[platform]; !ok {
			labels[platform] = platform
		}
	}
	return labels
}

func (s *Service) platformOptions() []dict.Option[string] {
	labels := s.platformLabels()
	options := make([]dict.Option[string], 0, len(s.allowedPlatforms))
	for platform := range s.allowedPlatforms {
		options = append(options, dict.Option[string]{Label: labels[platform], Value: platform})
	}
	sort.Slice(options, func(i, j int) bool {
		return options[i].Value < options[j].Value
	})
	return options
}

func (s *Service) normalizeMutationInput(input PermissionMutationInput) (PermissionMutationInput, *apperror.Error) {
	input.Platform = strings.TrimSpace(input.Platform)
	input.Name = strings.TrimSpace(input.Name)
	input.Icon = strings.TrimSpace(input.Icon)
	input.Path = strings.TrimSpace(input.Path)
	input.Component = strings.TrimSpace(input.Component)
	input.I18nKey = strings.TrimSpace(input.I18nKey)
	input.Code = strings.TrimSpace(input.Code)

	if !s.isAllowedPlatform(input.Platform) {
		return input, apperror.BadRequest("无效的平台标识: " + input.Platform)
	}
	if input.Type != TypeDir && input.Type != TypePage && input.Type != TypeButton {
		return input, apperror.BadRequest("无效的权限类型")
	}
	if input.Name == "" {
		return input, apperror.BadRequest("权限名称不能为空")
	}
	if input.Sort < 1 || input.Sort > 1000 {
		return input, apperror.BadRequest("排序必须在 1 到 1000 之间")
	}

	switch input.Type {
	case TypeDir:
		if input.I18nKey == "" {
			return input, apperror.BadRequest("i18n_key 不能为空")
		}
		if input.ShowMenu != CommonYes && input.ShowMenu != CommonNo {
			return input, apperror.BadRequest("菜单显示状态无效")
		}
	case TypePage:
		if input.Path == "" {
			return input, apperror.BadRequest("路由 path 不能为空")
		}
		if input.Component == "" {
			return input, apperror.BadRequest("组件路径不能为空")
		}
		if input.I18nKey == "" {
			return input, apperror.BadRequest("i18n_key 不能为空")
		}
		if input.ShowMenu != CommonYes && input.ShowMenu != CommonNo {
			return input, apperror.BadRequest("菜单显示状态无效")
		}
	case TypeButton:
		if input.Code == "" {
			return input, apperror.BadRequest("权限标识不能为空")
		}
	}

	return input, nil
}

func (s *Service) assertUniqueMutationFields(ctx context.Context, input PermissionMutationInput, excludeID int64) *apperror.Error {
	var (
		exists bool
		err    error
	)

	switch input.Type {
	case TypeDir:
		exists, err = s.repository.ExistsByPlatformI18nKey(ctx, input.Platform, input.I18nKey, excludeID)
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "校验 i18n_key 失败", err)
		}
		if exists {
			return apperror.BadRequest("该平台下 i18n_key 已存在")
		}
	case TypePage:
		exists, err = s.repository.ExistsByPlatformPath(ctx, input.Platform, input.Path, excludeID)
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "校验路由 path 失败", err)
		}
		if exists {
			return apperror.BadRequest("该平台下路由 path 已存在")
		}
		exists, err = s.repository.ExistsByPlatformI18nKey(ctx, input.Platform, input.I18nKey, excludeID)
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "校验 i18n_key 失败", err)
		}
		if exists {
			return apperror.BadRequest("该平台下 i18n_key 已存在")
		}
	case TypeButton:
		exists, err = s.repository.ExistsByPlatformCode(ctx, input.Platform, input.Code, excludeID)
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "校验权限标识失败", err)
		}
		if exists {
			return apperror.BadRequest("该平台下权限标识已存在")
		}
	}
	return nil
}

func (s *Service) restoreSoftDeletedButtonIfPresent(ctx context.Context, input PermissionMutationInput, row Permission) (int64, *apperror.Error) {
	deleted, err := s.repository.FindDeletedByPlatformCode(ctx, input.Platform, input.Code)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "查询已删除权限失败", err)
	}
	if deleted == nil {
		return 0, nil
	}
	if err := s.repository.RestoreDeletedPermission(ctx, deleted.ID, row); err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "恢复权限失败", err)
	}
	return deleted.ID, nil
}

func (s *Service) assertValidParentAssignment(ctx context.Context, permissionType int, parentID int64, platform string, currentID int64) *apperror.Error {
	if currentID > 0 {
		if parentID == currentID {
			return apperror.BadRequest("节点不能选择自己作为父级")
		}
		descendantIDs, err := s.repository.CascadeIDs(ctx, []int64{currentID})
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "查询子权限失败", err)
		}
		for _, descendantID := range descendantIDs {
			if descendantID != currentID && descendantID == parentID {
				return apperror.BadRequest("节点不能挂到自己的后代下面")
			}
		}
	}

	if parentID == RootParentID {
		if permissionType == TypeButton && platform == "admin" {
			return apperror.BadRequest("按钮类型的父节点只能是页面")
		}
		return nil
	}

	parent, err := s.repository.GetPermission(ctx, parentID)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询父权限失败", err)
	}
	if parent == nil {
		return apperror.NotFound("父节点不存在")
	}
	if parent.Platform != platform {
		return apperror.BadRequest("父节点与当前平台不一致")
	}

	switch permissionType {
	case TypeDir:
		if parent.Type != TypeDir {
			return apperror.BadRequest("目录类型的父节点只能是目录或根节点")
		}
	case TypePage:
		if parent.Type != TypeDir {
			return apperror.BadRequest("页面类型的父节点只能是目录或根节点")
		}
	case TypeButton:
		if parent.Type != TypePage {
			return apperror.BadRequest("按钮类型的父节点只能是页面")
		}
	}

	return nil
}

func (s *Service) assertExistingChildrenCompatible(ctx context.Context, permissionType int, id int64) *apperror.Error {
	children, err := s.repository.ActiveChildren(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询子权限失败", err)
	}
	if len(children) == 0 {
		return nil
	}

	if permissionType == TypeButton {
		return apperror.BadRequest("按钮类型不能包含子节点")
	}

	childTypes := make(map[int]struct{}, len(children))
	for _, child := range children {
		childTypes[child.Type] = struct{}{}
	}

	if permissionType == TypePage {
		for childType := range childTypes {
			if childType != TypeButton {
				return apperror.BadRequest("页面类型的子节点只能是按钮")
			}
		}
	}
	if permissionType == TypeDir {
		if _, ok := childTypes[TypeButton]; ok {
			return apperror.BadRequest("目录类型的子节点不能是按钮")
		}
	}
	return nil
}

func ButtonCacheKey(userID int64, platform string) string {
	return fmt.Sprintf("auth_perm_uid_%d_%s_%s", userID, platform, ButtonCacheKeySchema)
}

func permissionTypeOptions() []dict.Option[int] {
	return dict.PermissionTypeOptions()
}

func typeName(permissionType int) string {
	switch permissionType {
	case TypeDir:
		return "目录"
	case TypePage:
		return "页面"
	case TypeButton:
		return "按钮"
	default:
		return ""
	}
}

func normalizeListQuery(query PermissionListQuery) PermissionListQuery {
	query.Platform = strings.TrimSpace(query.Platform)
	query.Name = strings.TrimSpace(query.Name)
	query.Path = strings.TrimSpace(query.Path)
	return query
}

func hasPermissionListFilter(query PermissionListQuery) bool {
	return query.Name != "" || query.Path != "" || query.Type > 0
}

func filterPermissions(rows []Permission, query PermissionListQuery) []Permission {
	result := make([]Permission, 0, len(rows))
	for _, row := range rows {
		if row.Platform != query.Platform {
			continue
		}
		if query.Name != "" && !strings.Contains(row.Name, query.Name) {
			continue
		}
		if query.Path != "" && !strings.Contains(row.Path, query.Path) {
			continue
		}
		if query.Type > 0 && row.Type != query.Type {
			continue
		}
		result = append(result, row)
	}
	return result
}

func includeMatchedAncestors(allRows []Permission, matchedRows []Permission) []Permission {
	if len(matchedRows) == 0 {
		return []Permission{}
	}

	byID := make(map[int64]Permission, len(allRows))
	included := make(map[int64]struct{}, len(allRows))
	result := make([]Permission, 0, len(matchedRows))
	for _, row := range allRows {
		byID[row.ID] = row
	}

	var include func(id int64)
	include = func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := included[id]; ok {
			return
		}
		row, ok := byID[id]
		if !ok {
			return
		}
		if row.ParentID != RootParentID {
			include(row.ParentID)
		}
		included[id] = struct{}{}
		result = append(result, row)
	}

	for _, row := range matchedRows {
		include(row.ID)
	}
	return result
}

func buildPermissionListTree(rows []Permission) []PermissionListItem {
	if len(rows) == 0 {
		return []PermissionListItem{}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Sort == rows[j].Sort {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].Sort < rows[j].Sort
	})

	nodes := make(map[int64]PermissionListItem, len(rows))
	childrenByParent := make(map[int64][]int64, len(rows))
	rootIDs := make([]int64, 0)
	for _, row := range rows {
		nodes[row.ID] = PermissionListItem{
			ID:        row.ID,
			Name:      row.Name,
			Path:      row.Path,
			ParentID:  row.ParentID,
			Icon:      row.Icon,
			Component: row.Component,
			Status:    row.Status,
			Type:      row.Type,
			TypeName:  typeName(row.Type),
			Code:      row.Code,
			I18nKey:   row.I18nKey,
			Sort:      row.Sort,
			ShowMenu:  row.ShowMenu,
			Children:  []PermissionListItem{},
		}
		if row.ParentID == RootParentID {
			rootIDs = append(rootIDs, row.ID)
			continue
		}
		childrenByParent[row.ParentID] = append(childrenByParent[row.ParentID], row.ID)
	}

	var build func(id int64) PermissionListItem
	build = func(id int64) PermissionListItem {
		node := nodes[id]
		childIDs := childrenByParent[id]
		node.Children = make([]PermissionListItem, 0, len(childIDs))
		for _, childID := range childIDs {
			if _, ok := nodes[childID]; ok {
				node.Children = append(node.Children, build(childID))
			}
		}
		return node
	}

	tree := make([]PermissionListItem, 0, len(rootIDs))
	for _, rootID := range rootIDs {
		tree = append(tree, build(rootID))
	}
	return tree
}

func buildPermissionTreeNodes(rows []Permission, platformLabels map[string]string) []PermissionTreeNode {
	activeRows := make([]Permission, 0, len(rows))
	for _, row := range rows {
		if row.ID > 0 {
			activeRows = append(activeRows, row)
		}
	}
	sort.SliceStable(activeRows, func(i, j int) bool {
		if activeRows[i].Sort == activeRows[j].Sort {
			return activeRows[i].ID < activeRows[j].ID
		}
		return activeRows[i].Sort < activeRows[j].Sort
	})

	nodes := make(map[int64]PermissionTreeNode, len(activeRows))
	childrenByParent := make(map[int64][]int64, len(activeRows))
	rootIDs := make([]int64, 0)
	for _, row := range activeRows {
		label := row.Name
		if row.Platform != "" {
			platformLabel := platformLabels[row.Platform]
			if platformLabel == "" {
				platformLabel = row.Platform
			}
			label = "[" + platformLabel + "] " + row.Name
		}
		nodes[row.ID] = PermissionTreeNode{
			ID:       row.ID,
			Label:    label,
			Value:    row.ID,
			ParentID: row.ParentID,
			Platform: row.Platform,
			Type:     row.Type,
			Code:     row.Code,
			Children: []PermissionTreeNode{},
		}
		if row.ParentID == RootParentID {
			rootIDs = append(rootIDs, row.ID)
			continue
		}
		childrenByParent[row.ParentID] = append(childrenByParent[row.ParentID], row.ID)
	}

	var build func(id int64) PermissionTreeNode
	build = func(id int64) PermissionTreeNode {
		node := nodes[id]
		childIDs := childrenByParent[id]
		node.Children = make([]PermissionTreeNode, 0, len(childIDs))
		for _, childID := range childIDs {
			if _, ok := nodes[childID]; ok {
				node.Children = append(node.Children, build(childID))
			}
		}
		return node
	}

	tree := make([]PermissionTreeNode, 0, len(rootIDs))
	for _, rootID := range rootIDs {
		tree = append(tree, build(rootID))
	}
	return tree
}

func permissionFromMutation(input PermissionMutationInput) Permission {
	row := Permission{
		Name:     input.Name,
		ParentID: input.ParentID,
		Platform: input.Platform,
		Type:     input.Type,
		Sort:     input.Sort,
		Status:   CommonYes,
		IsDel:    CommonNo,
	}
	switch input.Type {
	case TypeDir:
		row.Icon = input.Icon
		row.I18nKey = input.I18nKey
		row.ShowMenu = input.ShowMenu
	case TypePage:
		row.Path = input.Path
		row.Icon = input.Icon
		row.Component = input.Component
		row.I18nKey = input.I18nKey
		row.ShowMenu = input.ShowMenu
	case TypeButton:
		row.Code = input.Code
		row.ShowMenu = CommonNo
	}
	return row
}

func permissionUpdateMap(input PermissionMutationInput) map[string]any {
	row := permissionFromMutation(input)
	fields := map[string]any{
		"name":      row.Name,
		"parent_id": row.ParentID,
		"platform":  row.Platform,
		"type":      row.Type,
		"sort":      row.Sort,
		"path":      "",
		"icon":      "",
		"component": "",
		"code":      "",
		"i18n_key":  "",
		"show_menu": CommonNo,
	}

	switch input.Type {
	case TypeDir:
		fields["icon"] = row.Icon
		fields["i18n_key"] = row.I18nKey
		fields["show_menu"] = row.ShowMenu
	case TypePage:
		fields["path"] = row.Path
		fields["icon"] = row.Icon
		fields["component"] = row.Component
		fields["i18n_key"] = row.I18nKey
		fields["show_menu"] = row.ShowMenu
	case TypeButton:
		fields["code"] = row.Code
	}
	return fields
}

func normalizeIDsForMutation(ids []int64) []int64 {
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

func normalizePlatformList(platforms []string) []string {
	seen := make(map[string]struct{}, len(platforms))
	result := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		platform = strings.TrimSpace(platform)
		if platform == "" {
			continue
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		result = append(result, platform)
	}
	sort.Strings(result)
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

		if (permission.Type == TypePage || permission.Type == TypeButton) && strings.TrimSpace(permission.Code) != "" {
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
