package payreconcile

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
)

const timeLayout = "2006-01-02 15:04:05"
const dateLayout = "2006-01-02"

const (
	defaultReconcileDailyLimit   = 100
	maxReconcileDailyLimit       = 100
	defaultReconcileExecuteLimit = 20
	maxReconcileExecuteLimit     = 100
	defaultReconcileReportPrefix = "runtime/reconcile_reports"
)

type Service struct {
	repository       Repository
	reportBaseDir    string
	reportPathPrefix string
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	channels, err := repo.ActivePayChannels(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付渠道字典失败", err)
	}
	channelOptions := make([]dict.Option[int], 0, len(channels))
	for _, channel := range channels {
		channelOptions = append(channelOptions, dict.Option[int]{Label: channel.Name, Value: int(channel.ID)})
	}
	return &InitResponse{Dict: InitDict{
		PayChannelArr:      channelOptions,
		ChannelArr:         dict.PayChannelOptions(),
		ReconcileStatusArr: ReconcileStatusOptions(),
		BillTypeArr:        BillTypeOptions(),
	}}, nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询对账任务失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItem(row))
	}
	return &ListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error) {
	row, appErr := s.getTask(ctx, id)
	if appErr != nil {
		return nil, appErr
	}
	return &DetailResponse{Task: detailTask(*row)}, nil
}

func (s *Service) Retry(ctx context.Context, id int64) *apperror.Error {
	row, appErr := s.getTask(ctx, id)
	if appErr != nil {
		return appErr
	}
	if row.Status != ReconcileFailed {
		return apperror.BadRequest("当前状态不支持重试")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	fields := map[string]any{
		"status":            ReconcilePending,
		"started_at":        nil,
		"finished_at":       nil,
		"platform_count":    0,
		"platform_amount":   0,
		"local_count":       0,
		"local_amount":      0,
		"diff_count":        0,
		"diff_amount":       0,
		"platform_file_url": "",
		"local_file_url":    "",
		"diff_file_url":     "",
		"error_msg":         "",
		"updated_at":        time.Now(),
	}
	if err := repo.UpdateRetry(ctx, id, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "重试对账任务失败", err)
	}
	return nil
}

func (s *Service) File(ctx context.Context, id int64, fileType string) (*FileResponse, *apperror.Error) {
	fileType = strings.TrimSpace(fileType)
	row, appErr := s.getTask(ctx, id)
	if appErr != nil {
		return nil, appErr
	}
	var rawURL string
	switch fileType {
	case "platform":
		rawURL = row.PlatformFileURL
	case "local":
		rawURL = row.LocalFileURL
	case "diff":
		rawURL = row.DiffFileURL
	default:
		return nil, apperror.BadRequest("不支持的文件类型")
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, apperror.BadRequest("当前文件不存在")
	}
	return &FileResponse{URL: rawURL, Filename: filenameFromURL(rawURL)}, nil
}

func (s *Service) getTask(ctx context.Context, id int64) (*Task, *apperror.Error) {
	if id <= 0 {
		return nil, apperror.BadRequest("无效的对账任务ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Detail(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询对账任务详情失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("对账任务不存在")
	}
	return row, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("对账任务仓储未配置")
	}
	return s.repository, nil
}

func normalizeListQuery(query ListQuery) ListQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.StartDate = strings.TrimSpace(query.StartDate)
	query.EndDate = strings.TrimSpace(query.EndDate)
	return query
}

func listItem(row ListRow) ListItem {
	return ListItem{
		ID: row.ID, ReconcileDate: formatDate(row.ReconcileDate), Channel: row.Channel, ChannelText: enum.PayChannelLabels[row.Channel],
		BillType: row.BillType, BillTypeText: BillTypeLabels[row.BillType], Status: row.Status, StatusText: ReconcileStatusLabels[row.Status],
		PlatformCount: row.PlatformCount, PlatformAmount: row.PlatformAmount, LocalCount: row.LocalCount, LocalAmount: row.LocalAmount,
		DiffCount: row.DiffCount, DiffAmount: row.DiffAmount, StartedAt: formatOptionalTime(row.StartedAt), FinishedAt: formatOptionalTime(row.FinishedAt), CreatedAt: formatTime(row.CreatedAt),
	}
}

func detailTask(row Task) DetailTask {
	return DetailTask{
		ID: row.ID, ReconcileDate: formatDate(row.ReconcileDate), Channel: row.Channel, ChannelText: enum.PayChannelLabels[row.Channel], ChannelID: row.ChannelID,
		BillType: row.BillType, BillTypeText: BillTypeLabels[row.BillType], Status: row.Status, StatusText: ReconcileStatusLabels[row.Status],
		PlatformCount: row.PlatformCount, PlatformAmount: row.PlatformAmount, LocalCount: row.LocalCount, LocalAmount: row.LocalAmount,
		DiffCount: row.DiffCount, DiffAmount: row.DiffAmount, PlatformFileURL: row.PlatformFileURL, LocalFileURL: row.LocalFileURL, DiffFileURL: row.DiffFileURL,
		StartedAt: formatOptionalTime(row.StartedAt), FinishedAt: formatOptionalTime(row.FinishedAt), ErrorMsg: row.ErrorMsg, CreatedAt: formatTime(row.CreatedAt),
	}
}

func ReconcileStatusOptions() []dict.Option[int] {
	options := make([]dict.Option[int], 0, len(ReconcileStatuses))
	for _, value := range ReconcileStatuses {
		options = append(options, dict.Option[int]{Label: ReconcileStatusLabels[value], Value: value})
	}
	return options
}

func BillTypeOptions() []dict.Option[int] {
	return []dict.Option[int]{{Label: BillTypeLabels[BillTypePay], Value: BillTypePay}}
}

func IsReconcileStatus(value int) bool {
	for _, status := range ReconcileStatuses {
		if status == value {
			return true
		}
	}
	return false
}

func IsBillType(value int) bool { return value == BillTypePay }

func formatOptionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	text := value.Format(timeLayout)
	return &text
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}

func formatDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(dateLayout)
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func filenameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Path != "" {
		name := path.Base(parsed.Path)
		if name != "." && name != "/" {
			return name
		}
	}
	return path.Base(strings.TrimSpace(rawURL))
}

func (s *Service) CreateDailyTasks(ctx context.Context, input CreateDailyTasksInput) (*CreateDailyTasksResult, error) {
	if s == nil || s.repository == nil {
		return nil, ErrRepositoryNotConfigured
	}
	reconcileDate, err := normalizeCreateDailyDate(input)
	if err != nil {
		return nil, err
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	limit := normalizeReconcileLimit(input.Limit, defaultReconcileDailyLimit)
	channels, err := s.repository.ActivePayChannels(ctx)
	if err != nil {
		return nil, err
	}
	result := &CreateDailyTasksResult{Date: reconcileDate.Format(dateLayout)}
	if len(channels) > limit {
		result.Skipped += len(channels) - limit
		channels = channels[:limit]
	}
	result.Scanned = len(channels)
	for _, channel := range channels {
		if channel.ID <= 0 || !enum.IsPayChannel(channel.Channel) {
			result.Skipped++
			continue
		}
		existing, err := s.repository.FindReconcileTask(ctx, channel.ID, reconcileDate, BillTypePay)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			result.Existing++
			continue
		}
		task := Task{
			ReconcileDate: reconcileDate,
			Channel:       channel.Channel,
			ChannelID:     channel.ID,
			BillType:      BillTypePay,
			Status:        ReconcilePending,
			IsDel:         enum.CommonNo,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := s.repository.CreateReconcileTask(ctx, task); err != nil {
			if isDuplicateTaskError(err) {
				result.Existing++
				continue
			}
			return nil, err
		}
		result.Created++
	}
	return result, nil
}

func (s *Service) ExecutePendingTasks(ctx context.Context, input ExecutePendingTasksInput) (*ExecutePendingTasksResult, error) {
	if s == nil || s.repository == nil {
		return nil, ErrRepositoryNotConfigured
	}
	limit := normalizeExecuteLimit(input.Limit)
	tasks, err := s.repository.ListPendingTasks(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := &ExecutePendingTasksResult{Scanned: len(tasks)}
	for _, task := range tasks {
		taskResult, err := s.ExecuteTask(ctx, task.ID)
		if err != nil {
			result.Failed++
			continue
		}
		switch taskResult.Status {
		case ReconcileSuccess:
			result.Success++
		case ReconcileDiff:
			result.Diff++
		case ReconcileFailed:
			result.Failed++
		default:
			result.Skipped++
		}
	}
	return result, nil
}

func (s *Service) ExecuteTask(ctx context.Context, taskID int64) (*ExecuteTaskResult, error) {
	if s == nil || s.repository == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if taskID <= 0 {
		return nil, fmt.Errorf("invalid reconcile task id")
	}
	var result *ExecuteTaskResult
	err := s.repository.WithTx(ctx, func(repo Repository) error {
		task, err := repo.GetTaskForUpdate(ctx, taskID)
		if err != nil {
			return err
		}
		if task == nil {
			return fmt.Errorf("pay reconcile task not found")
		}
		res, err := s.executeTaskInTx(ctx, repo, *task)
		result = res
		return err
	})
	return result, err
}

func (s *Service) executeTaskInTx(ctx context.Context, repo Repository, task Task) (*ExecuteTaskResult, error) {
	result := &ExecuteTaskResult{
		TaskID:         task.ID,
		Status:         task.Status,
		PlatformCount:  task.PlatformCount,
		PlatformAmount: task.PlatformAmount,
		LocalCount:     task.LocalCount,
		LocalAmount:    task.LocalAmount,
		DiffCount:      task.DiffCount,
		DiffAmount:     task.DiffAmount,
	}
	if task.Status != ReconcilePending && task.Status != ReconcileFailed {
		return result, nil
	}
	now := time.Now()
	if err := repo.MarkTaskStatus(ctx, task.ID, ReconcileDownload, map[string]any{
		"started_at":  now,
		"finished_at": nil,
		"error_msg":   "",
		"updated_at":  now,
	}); err != nil {
		return nil, err
	}

	failTask := func(reason error, fields map[string]any) (*ExecuteTaskResult, error) {
		if fields == nil {
			fields = map[string]any{}
		}
		fields["finished_at"] = now
		fields["error_msg"] = truncateReconcileError(reason)
		fields["updated_at"] = now
		if err := repo.MarkTaskStatus(ctx, task.ID, ReconcileFailed, fields); err != nil {
			return nil, err
		}
		result.Status = ReconcileFailed
		return result, nil
	}

	if task.Channel != enum.PayChannelAlipay {
		return failTask(fmt.Errorf("当前渠道不支持 Go 对账执行: %d", task.Channel), nil)
	}

	rows, err := repo.ListSuccessfulTransactionsForBill(ctx, task.ChannelID, task.ReconcileDate)
	if err != nil {
		return failTask(err, nil)
	}
	localFileURL, err := s.writeLocalBill(task, rows)
	if err != nil {
		return failTask(err, nil)
	}
	localCount, localAmount := summarizeBillRows(rows)
	result.LocalCount = localCount
	result.LocalAmount = localAmount
	if err := repo.MarkTaskStatus(ctx, task.ID, ReconcileComparing, map[string]any{
		"local_count":    localCount,
		"local_amount":   localAmount,
		"local_file_url": localFileURL,
		"updated_at":     now,
	}); err != nil {
		return nil, err
	}

	return failTask(ErrPlatformBillDownloadNotImplemented, map[string]any{
		"local_count":     localCount,
		"local_amount":    localAmount,
		"local_file_url":  localFileURL,
		"platform_count":  0,
		"platform_amount": 0,
		"diff_count":      0,
		"diff_amount":     0,
	})
}

func normalizeCreateDailyDate(input CreateDailyTasksInput) (time.Time, error) {
	rawDate := strings.TrimSpace(input.Date)
	if rawDate == "" {
		now := input.Now
		if now.IsZero() {
			now = time.Now()
		}
		yesterday := now.AddDate(0, 0, -1)
		return time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.Local), nil
	}
	reconcileDate, err := time.ParseInLocation(dateLayout, rawDate, time.Local)
	if err != nil {
		return time.Time{}, err
	}
	return reconcileDate, nil
}

func normalizeReconcileLimit(limit int, defaultLimit int) int {
	if defaultLimit <= 0 {
		defaultLimit = defaultReconcileDailyLimit
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxReconcileDailyLimit {
		return maxReconcileDailyLimit
	}
	return limit
}

func normalizeExecuteLimit(limit int) int {
	if limit <= 0 {
		limit = defaultReconcileExecuteLimit
	}
	if limit > maxReconcileExecuteLimit {
		return maxReconcileExecuteLimit
	}
	return limit
}

func isDuplicateTaskError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate") || strings.Contains(message, "duplicated") || strings.Contains(message, "1062")
}

func (s *Service) writeLocalBill(task Task, rows []BillTransactionRow) (string, error) {
	baseDir := s.reportBaseDir
	if strings.TrimSpace(baseDir) == "" {
		baseDir = filepath.Join("runtime", "reconcile_reports")
	}
	prefix := strings.Trim(strings.TrimSpace(s.reportPathPrefix), "/\\")
	if prefix == "" {
		prefix = defaultReconcileReportPrefix
	}
	dateText := task.ReconcileDate.Format(dateLayout)
	if dateText == "" {
		return "", fmt.Errorf("empty reconcile date")
	}
	dir := filepath.Join(baseDir, dateText)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%d-local.csv", task.ID)
	filePath := filepath.Join(dir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"transaction_no", "order_no", "trade_no", "amount", "paid_at"}); err != nil {
		return "", err
	}
	for _, row := range rows {
		paidAt := ""
		if !row.PaidAt.IsZero() {
			paidAt = row.PaidAt.Format(timeLayout)
		}
		record := []string{
			row.TransactionNo,
			row.OrderNo,
			row.TradeNo,
			fmt.Sprintf("%.2f", float64(row.Amount)/100),
			paidAt,
		}
		if err := writer.Write(record); err != nil {
			return "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return path.Join(prefix, dateText, filename), nil
}

func summarizeBillRows(rows []BillTransactionRow) (int, int64) {
	var amount int64
	for _, row := range rows {
		amount += row.Amount
	}
	return len(rows), amount
}

func truncateReconcileError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrPlatformBillDownloadNotImplemented) {
		return "平台账单下载未实现"
	}
	message := strings.TrimSpace(err.Error())
	if len([]rune(message)) <= 500 {
		return message
	}
	return string([]rune(message)[:500])
}
