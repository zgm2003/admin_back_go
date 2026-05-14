package exporttask

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

const (
	defaultPageSize       = 20
	maxPageSize           = 50
	exportTaskTimeLayout  = "2006-01-02 15:04:05"
	defaultExpireDuration = 7 * 24 * time.Hour
)

type Service struct {
	repository Repository
	provider   ExportDataProvider
	writer     FileWriter
	uploader   FileUploader
	notifier   Notifier
	logger     *slog.Logger
	now        func() time.Time
}

type Option func(*Service)

func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithExportDataProvider(provider ExportDataProvider) Option {
	return func(s *Service) {
		s.provider = provider
	}
}

func WithFileWriter(writer FileWriter) Option {
	return func(s *Service) {
		s.writer = writer
	}
}

func WithFileUploader(uploader FileUploader) Option {
	return func(s *Service) {
		s.uploader = uploader
	}
}

func WithNotifier(notifier Notifier) Option {
	return func(s *Service) {
		s.notifier = notifier
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(s *Service) {
		if logger != nil {
			s.logger = logger
		}
	}
}

func NewService(repository Repository, opts ...Option) *Service {
	service := &Service{repository: repository, now: time.Now, logger: slog.Default()}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

type ExportDataProvider interface {
	BuildExportData(ctx context.Context, kind string, ids []int64) (*FileData, error)
}

type FileWriter interface {
	Write(data FileData) ([]byte, error)
}

type FileUploader interface {
	Upload(ctx context.Context, input UploadInput) (*UploadResult, error)
}

type Notifier interface {
	NotifyExportSuccess(ctx context.Context, input NotifyInput) error
	NotifyExportFailed(ctx context.Context, input NotifyInput) error
}

type NotifyInput struct {
	TaskID   int64
	UserID   int64
	Title    string
	Platform string
	Link     string
	ErrorMsg string
}

func (s *Service) StatusCount(ctx context.Context, query StatusCountQuery) ([]StatusCountItem, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeStatusCountQuery(query)
	if query.UserID <= 0 {
		return nil, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if err := repo.CleanExpired(ctx, s.now()); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "exporttask.cleanup_failed", nil, "清理过期导出任务失败", err)
	}
	counts, err := repo.CountByStatus(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "exporttask.status_count_failed", nil, "查询导出任务状态统计失败", err)
	}
	return statusCountItems(counts), nil
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
	if err := repo.CleanExpired(ctx, s.now()); err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "exporttask.cleanup_failed", nil, "清理过期导出任务失败", err)
	}
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "exporttask.query_failed", nil, "查询导出任务失败", err)
	}
	items := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, listItemFromTask(row))
	}
	return &ListResponse{List: items, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) CreatePending(ctx context.Context, input CreatePendingInput) (int64, error) {
	repo, err := s.requireRepositoryError()
	if err != nil {
		return 0, err
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.UserID <= 0 || input.Title == "" {
		return 0, apperror.BadRequestKey("exporttask.create_pending.invalid", nil, "导出任务参数错误")
	}
	now := s.now()
	expireAt := now.Add(defaultExpireDuration)
	return repo.Create(ctx, Task{
		UserID:    input.UserID,
		Title:     input.Title,
		Status:    enum.ExportTaskStatusPending,
		ExpireAt:  &expireAt,
		IsDel:     enum.CommonNo,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (s *Service) MarkSuccess(ctx context.Context, id int64, result SuccessResult) error {
	repo, err := s.requireRepositoryError()
	if err != nil {
		return err
	}
	if id <= 0 {
		return apperror.BadRequestKey("exporttask.id.invalid", nil, "无效的导出任务ID")
	}
	return repo.MarkSuccess(ctx, id, result)
}

func (s *Service) MarkFailed(ctx context.Context, id int64, message string) error {
	repo, err := s.requireRepositoryError()
	if err != nil {
		return err
	}
	if id <= 0 {
		return apperror.BadRequestKey("exporttask.id.invalid", nil, "无效的导出任务ID")
	}
	return repo.MarkFailed(ctx, id, capRunes(message, 500))
}

func (s *Service) Delete(ctx context.Context, input DeleteInput) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if input.UserID <= 0 {
		return apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	ids := normalizeIDs(input.IDs)
	if len(ids) == 0 {
		return apperror.BadRequestKey("exporttask.delete.empty", nil, "请选择要删除的导出任务")
	}
	if err := repo.DeleteByUser(ctx, input.UserID, ids); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "exporttask.delete_failed", nil, "删除导出任务失败", err)
	}
	return nil
}

func (s *Service) Run(ctx context.Context, input RunInput) error {
	if err := validateRunInput(input); err != nil {
		return err
	}
	repo, err := s.requireRepositoryError()
	if err != nil {
		return err
	}
	task, err := repo.Get(ctx, input.TaskID)
	if err != nil {
		return fmt.Errorf("load export task %d: %w", input.TaskID, err)
	}
	if task == nil || task.IsDel == enum.CommonYes || task.Status == enum.ExportTaskStatusSuccess {
		return nil
	}
	if s.provider == nil || s.writer == nil || s.uploader == nil {
		return s.failRun(ctx, *task, input, fmt.Errorf("export task runtime is not configured"))
	}

	data, err := s.provider.BuildExportData(ctx, input.Kind, input.IDs)
	if err != nil {
		return s.failRun(ctx, *task, input, fmt.Errorf("build export data: %w", err))
	}
	if data == nil {
		return s.failRun(ctx, *task, input, fmt.Errorf("build export data: empty result"))
	}
	body, err := s.writer.Write(*data)
	if err != nil {
		return s.failRun(ctx, *task, input, fmt.Errorf("write xlsx: %w", err))
	}
	result, err := s.uploader.Upload(ctx, UploadInput{TaskID: input.TaskID, Prefix: data.Prefix, Body: body, RowCount: int64(len(data.Rows))})
	if err != nil {
		return s.failRun(ctx, *task, input, fmt.Errorf("upload xlsx: %w", err))
	}
	if result == nil {
		return s.failRun(ctx, *task, input, fmt.Errorf("upload xlsx: empty result"))
	}
	if err := repo.MarkSuccess(ctx, input.TaskID, SuccessResult{
		FileName: result.FileName,
		FileURL:  result.FileURL,
		FileSize: result.FileSize,
		RowCount: result.RowCount,
	}); err != nil {
		return fmt.Errorf("mark export task success: %w", err)
	}
	s.notifySuccess(ctx, *task, input)
	return nil
}

func (s *Service) failRun(ctx context.Context, task Task, input RunInput, runErr error) error {
	message := ""
	if runErr != nil {
		message = runErr.Error()
	}
	if repo, err := s.requireRepositoryError(); err == nil {
		if markErr := repo.MarkFailed(ctx, input.TaskID, capRunes(message, 500)); markErr != nil && s.logger != nil {
			s.logger.WarnContext(ctx, "failed to mark export task failed", "task_id", input.TaskID, "error", markErr)
		}
	}
	s.notifyFailed(ctx, task, input, message)
	return runErr
}

func (s *Service) notifySuccess(ctx context.Context, task Task, input RunInput) {
	if s == nil || s.notifier == nil {
		return
	}
	if err := s.notifier.NotifyExportSuccess(ctx, NotifyInput{
		TaskID:   input.TaskID,
		UserID:   input.UserID,
		Title:    task.Title,
		Platform: input.Platform,
		Link:     "/system/exportTask?status=2",
	}); err != nil && s.logger != nil {
		s.logger.WarnContext(ctx, "failed to send export success notification", "task_id", input.TaskID, "error", err)
	}
}

func (s *Service) notifyFailed(ctx context.Context, task Task, input RunInput, message string) {
	if s == nil || s.notifier == nil {
		return
	}
	if err := s.notifier.NotifyExportFailed(ctx, NotifyInput{
		TaskID:   input.TaskID,
		UserID:   input.UserID,
		Title:    task.Title,
		Platform: input.Platform,
		Link:     "/system/exportTask?status=3",
		ErrorMsg: capRunes(message, 500),
	}); err != nil && s.logger != nil {
		s.logger.WarnContext(ctx, "failed to send export failed notification", "task_id", input.TaskID, "error", err)
	}
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey("exporttask.repository_missing", nil, "导出任务仓储未配置")
	}
	return s.repository, nil
}

func (s *Service) requireRepositoryError() (Repository, error) {
	if s == nil || s.repository == nil {
		return nil, ErrRepositoryNotConfigured
	}
	return s.repository, nil
}

func normalizeStatusCountQuery(query StatusCountQuery) StatusCountQuery {
	query.Title = strings.TrimSpace(query.Title)
	query.FileName = strings.TrimSpace(query.FileName)
	return query
}

func normalizeListQuery(query ListQuery) (ListQuery, *apperror.Error) {
	query.Title = strings.TrimSpace(query.Title)
	query.FileName = strings.TrimSpace(query.FileName)
	if query.UserID <= 0 {
		return query, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = defaultPageSize
	}
	if query.PageSize > maxPageSize {
		query.PageSize = maxPageSize
	}
	if query.Status != nil && !enum.IsExportTaskStatus(*query.Status) {
		return query, apperror.BadRequestKey("exporttask.status.invalid", nil, "导出任务状态错误")
	}
	return query, nil
}

func statusCountItems(counts map[int]int64) []StatusCountItem {
	statuses := []int{enum.ExportTaskStatusPending, enum.ExportTaskStatusSuccess, enum.ExportTaskStatusFailed}
	items := make([]StatusCountItem, 0, len(statuses))
	for _, status := range statuses {
		items = append(items, StatusCountItem{Label: enum.ExportTaskStatusLabels[status], Value: status, Num: counts[status]})
	}
	return items
}

func listItemFromTask(row Task) ListItem {
	return ListItem{
		ID:           row.ID,
		Title:        row.Title,
		FileName:     optionalString(row.FileName),
		FileURL:      optionalString(row.FileURL),
		FileSizeText: formatFileSize(row.FileSize),
		RowCount:     row.RowCount,
		Status:       row.Status,
		StatusText:   enum.ExportTaskStatusLabels[row.Status],
		ErrorMsg:     optionalString(row.ErrorMsg),
		ExpireAt:     optionalTime(row.ExpireAt),
		CreatedAt:    formatTime(row.CreatedAt),
	}
}

func normalizeIDs(ids []int64) []int64 {
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			set[id] = struct{}{}
		}
	}
	result := make([]int64, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func capRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func formatFileSize(size *int64) string {
	if size == nil || *size <= 0 {
		return "-"
	}
	value := *size
	if value < 1024 {
		return strconv.FormatInt(value, 10) + " B"
	}
	if value < 1024*1024 {
		return formatRounded(float64(value)/1024) + " KB"
	}
	return formatRounded(float64(value)/(1024*1024)) + " MB"
}

func formatRounded(value float64) string {
	value = math.Round(value*100) / 100
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := formatTime(*value)
	return &formatted
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(exportTaskTimeLayout)
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}
