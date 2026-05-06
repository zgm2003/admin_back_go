package crontask

import (
	"context"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"

	robfigcron "github.com/robfig/cron/v3"
)

const (
	maxNameLen        = 100
	maxTitleLen       = 100
	maxDescriptionLen = 500
	maxCronLen        = 100
	maxHandlerLen     = 255
)

type Service struct {
	repository Repository
	registry   Registry
	now        func() time.Time
}

func NewService(repository Repository, registry Registry) *Service {
	return &Service{repository: repository, registry: registry, now: time.Now}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		CronPresetArr: []dict.Option[string]{
			{Label: "每分钟", Value: "0 * * * * *"},
			{Label: "每5分钟", Value: "0 */5 * * * *"},
			{Label: "每小时", Value: "0 0 * * * *"},
			{Label: "每天零点", Value: "0 0 0 * * *"},
		},
		CronTaskStatusArr:      dict.CommonStatusOptions(),
		CronTaskRegistryStatus: registryStatusOptions(),
		CronTaskLogStatusArr: []dict.Option[int]{
			{Label: "成功", Value: LogStatusSuccess},
			{Label: "失败", Value: LogStatusFailed},
			{Label: "执行中", Value: LogStatusRunning},
		},
	}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query, appErr = normalizeListQuery(query)
	if appErr != nil {
		return nil, appErr
	}
	if query.RegistryStatus != "" {
		rows, total, err := s.listByRegistryStatus(ctx, repo, query)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询定时任务失败", err)
		}
		return &ListResponse{
			List: s.listItemsFromTasks(rows),
			Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
		}, nil
	}
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询定时任务失败", err)
	}
	return &ListResponse{
		List: s.listItemsFromTasks(rows),
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) listItemsFromTasks(rows []Task) []ListItem {
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, s.listItemFromTask(row))
	}
	return list
}

func (s *Service) listByRegistryStatus(ctx context.Context, repo Repository, query ListQuery) ([]Task, int64, error) {
	rows, err := repo.ListAll(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	filtered := make([]Task, 0, len(rows))
	for _, row := range rows {
		item := s.listItemFromTask(row)
		if item.RegistryStatus == query.RegistryStatus {
			filtered = append(filtered, row)
		}
	}
	total := int64(len(filtered))
	start := (query.CurrentPage - 1) * query.PageSize
	if start >= len(filtered) {
		return []Task{}, total, nil
	}
	end := start + query.PageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (s *Service) Create(ctx context.Context, input SaveInput) (*ListItem, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	input, appErr = normalizeSaveInput(input)
	if appErr != nil {
		return nil, appErr
	}
	exists, err := repo.NameExists(ctx, input.Name, 0)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "校验定时任务名称失败", err)
	}
	if exists {
		return nil, apperror.BadRequest("定时任务名称已存在")
	}
	now := s.now()
	id, err := repo.Create(ctx, Task{
		Name: input.Name, Title: input.Title, Description: input.Description, Cron: input.Cron,
		CronReadable: input.CronReadable, Handler: input.Handler, Status: input.Status, IsDel: CommonNo,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "新增定时任务失败", err)
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询定时任务失败", err)
	}
	return ptr(s.listItemFromTask(*row)), nil
}

func (s *Service) Update(ctx context.Context, id int64, input SaveInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的定时任务ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	input, appErr = normalizeSaveInput(input)
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return mapTaskLookupError(err)
	}
	if strings.TrimSpace(row.Name) != input.Name {
		return apperror.BadRequest("定时任务名称不允许修改")
	}
	exists, err := repo.NameExists(ctx, input.Name, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验定时任务名称失败", err)
	}
	if exists {
		return apperror.BadRequest("定时任务名称已存在")
	}
	if err := repo.Update(ctx, id, Task{Name: input.Name, Title: input.Title, Description: input.Description, Cron: input.Cron, CronReadable: input.CronReadable, Handler: input.Handler, Status: input.Status}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新定时任务失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的定时任务ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if _, err := repo.Get(ctx, id); err != nil {
		return mapTaskLookupError(err)
	}
	if err := repo.UpdateStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "更新定时任务状态失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, ids []int64) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return apperror.BadRequest("请选择要删除的定时任务")
	}
	for _, id := range ids {
		if _, err := repo.Get(ctx, id); err != nil {
			return mapTaskLookupError(err)
		}
	}
	if err := repo.Delete(ctx, ids); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除定时任务失败", err)
	}
	return nil
}

func (s *Service) Logs(ctx context.Context, query LogsQuery) (*LogsResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query, appErr = normalizeLogsQuery(query)
	if appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.Logs(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询定时任务日志失败", err)
	}
	list := make([]LogItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, logItemFromRow(row))
	}
	return &LogsResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "定时任务仓储未配置", ErrRepositoryNotConfigured)
	}
	return s.repository, nil
}

func normalizeListQuery(query ListQuery) (ListQuery, *apperror.Error) {
	if query.CurrentPage <= 0 {
		return query, apperror.BadRequest("当前页无效")
	}
	if query.PageSize < enum.PageSizeMin || query.PageSize > enum.PageSizeMax {
		return query, apperror.BadRequest("每页数量无效")
	}
	if query.Status != nil && !enum.IsCommonStatus(*query.Status) {
		return query, apperror.BadRequest("无效的状态")
	}
	query.Title = strings.TrimSpace(query.Title)
	query.Name = strings.TrimSpace(query.Name)
	query.RegistryStatus = strings.TrimSpace(query.RegistryStatus)
	if query.RegistryStatus != "" && !isRegistryStatus(query.RegistryStatus) {
		return query, apperror.BadRequest("无效的接入状态")
	}
	return query, nil
}

func normalizeLogsQuery(query LogsQuery) (LogsQuery, *apperror.Error) {
	if query.TaskID <= 0 {
		return query, apperror.BadRequest("无效的定时任务ID")
	}
	if query.CurrentPage <= 0 {
		return query, apperror.BadRequest("当前页无效")
	}
	if query.PageSize < enum.PageSizeMin || query.PageSize > enum.PageSizeMax {
		return query, apperror.BadRequest("每页数量无效")
	}
	if query.Status != nil && !isLogStatus(*query.Status) {
		return query, apperror.BadRequest("无效的日志状态")
	}
	query.StartDate = strings.TrimSpace(query.StartDate)
	query.EndDate = strings.TrimSpace(query.EndDate)
	return query, nil
}

func normalizeSaveInput(input SaveInput) (SaveInput, *apperror.Error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Cron = strings.TrimSpace(input.Cron)
	input.CronReadable = strings.TrimSpace(input.CronReadable)
	input.Handler = strings.TrimSpace(input.Handler)
	if input.Name == "" || len([]rune(input.Name)) > maxNameLen {
		return input, apperror.BadRequest("任务名称不能为空且不能超过100个字符")
	}
	if input.Title == "" || len([]rune(input.Title)) > maxTitleLen {
		return input, apperror.BadRequest("任务标题不能为空且不能超过100个字符")
	}
	if len([]rune(input.Description)) > maxDescriptionLen {
		return input, apperror.BadRequest("任务描述不能超过500个字符")
	}
	if input.Cron == "" || len([]rune(input.Cron)) > maxCronLen || !isValidCronExpression(input.Cron) {
		return input, apperror.BadRequest("Cron 表达式无效")
	}
	if len([]rune(input.Handler)) > maxHandlerLen {
		return input, apperror.BadRequest("Handler 不能超过255个字符")
	}
	if !enum.IsCommonStatus(input.Status) {
		return input, apperror.BadRequest("无效的状态")
	}
	return input, nil
}

func (s *Service) listItemFromTask(row Task) ListItem {
	entry, ok := s.registry.Lookup(row.Name)
	registryStatus := RegistryStatusMissing
	registryTaskType := ""
	registryDescription := ""
	if row.Status == CommonNo {
		registryStatus = RegistryStatusDisabled
	} else if !isValidCronExpression(row.Cron) {
		registryStatus = RegistryStatusInvalidCron
	} else if ok {
		registryStatus = RegistryStatusRegistered
		registryTaskType = entry.TaskType
		registryDescription = entry.Description
	}
	return ListItem{
		ID: row.ID, Name: row.Name, Title: row.Title, Description: row.Description,
		Cron: row.Cron, CronReadable: row.CronReadable, Handler: row.Handler,
		Status: row.Status, StatusName: statusName(row.Status), NextRunTime: nextRunTime(row.Cron),
		RegistryStatus: registryStatus, RegistryStatusText: registryStatusName(registryStatus), RegistryTaskType: registryTaskType,
		RegistryDescription: registryDescription, CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func logItemFromRow(row TaskLog) LogItem {
	return LogItem{
		ID: row.ID, TaskID: row.TaskID, TaskName: row.TaskName, StartTime: optionalTime(row.StartTime), EndTime: optionalTime(row.EndTime),
		DurationMS: row.DurationMS, Status: row.Status, StatusName: logStatusName(row.Status), Result: optionalString(row.Result), ErrorMsg: optionalString(row.ErrorMsg), CreatedAt: formatTime(row.CreatedAt),
	}
}

func mapTaskLookupError(err error) *apperror.Error {
	if err == nil {
		return nil
	}
	if err == ErrTaskNotFound {
		return apperror.NotFound("定时任务不存在")
	}
	return apperror.Wrap(apperror.CodeInternal, 500, "查询定时任务失败", err)
}

func statusName(status int) string {
	switch status {
	case CommonYes:
		return "启用"
	case CommonNo:
		return "禁用"
	default:
		return "未知"
	}
}

func registryStatusName(status string) string {
	switch status {
	case RegistryStatusRegistered:
		return "已接入"
	case RegistryStatusMissing:
		return "未接入"
	case RegistryStatusDisabled:
		return "已禁用"
	case RegistryStatusInvalidCron:
		return "表达式错误"
	default:
		return "未知"
	}
}

func logStatusName(status int) string {
	switch status {
	case LogStatusSuccess:
		return "成功"
	case LogStatusFailed:
		return "失败"
	case LogStatusRunning:
		return "执行中"
	default:
		return "未知"
	}
}

func isRegistryStatus(status string) bool {
	switch status {
	case RegistryStatusRegistered, RegistryStatusMissing, RegistryStatusDisabled, RegistryStatusInvalidCron:
		return true
	default:
		return false
	}
}

func isLogStatus(status int) bool {
	return status == LogStatusSuccess || status == LogStatusFailed || status == LogStatusRunning
}

func isValidCronExpression(expression string) bool {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return false
	}
	parser := robfigcron.NewParser(
		robfigcron.SecondOptional | robfigcron.Minute | robfigcron.Hour | robfigcron.Dom | robfigcron.Month | robfigcron.Dow | robfigcron.Descriptor,
	)
	_, err := parser.Parse(expression)
	return err == nil
}

func nextRunTime(expression string) string {
	if !isValidCronExpression(expression) {
		return enum.DefaultNull
	}
	return enum.DefaultNull
}

func registryStatusOptions() []dict.Option[string] {
	return []dict.Option[string]{
		{Label: registryStatusName(RegistryStatusRegistered), Value: RegistryStatusRegistered},
		{Label: registryStatusName(RegistryStatusMissing), Value: RegistryStatusMissing},
		{Label: registryStatusName(RegistryStatusDisabled), Value: RegistryStatusDisabled},
		{Label: registryStatusName(RegistryStatusInvalidCron), Value: RegistryStatusInvalidCron},
	}
}

func optionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	text := formatTime(*value)
	return &text
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
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

func ptr[T any](value T) *T {
	return &value
}
