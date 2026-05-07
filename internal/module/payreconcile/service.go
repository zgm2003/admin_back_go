package payreconcile

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/payment"
	payalipay "admin_back_go/internal/platform/payment/alipay"
	"admin_back_go/internal/platform/secretbox"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
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

type secretDecrypter interface {
	Decrypt(ciphertext string) (string, error)
}

type certResolver interface {
	Resolve(storedPath string) (string, error)
}

type Dependencies struct {
	Repository    Repository
	AlipayGateway payalipay.Gateway
	Secretbox     secretDecrypter
	CertResolver  certResolver
}

type Service struct {
	repository       Repository
	alipayGateway    payalipay.Gateway
	secretbox        secretDecrypter
	certResolver     certResolver
	reportBaseDir    string
	reportPathPrefix string
}

func NewService(repository Repository) *Service {
	return NewServiceWithDependencies(Dependencies{Repository: repository})
}

func NewServiceWithDependencies(deps Dependencies) *Service {
	gateway := deps.AlipayGateway
	if gateway == nil {
		gateway = payalipay.NewGopayGateway(0)
	}
	box := deps.Secretbox
	if box == nil {
		defaultBox := secretbox.New("")
		box = defaultBox
	}
	resolver := deps.CertResolver
	if resolver == nil {
		resolver = payment.CertPathResolver{WorkingDir: "."}
	}
	return &Service{
		repository:    deps.Repository,
		alipayGateway: gateway,
		secretbox:     box,
		certResolver:  resolver,
	}
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
	var taskToExecute *Task
	err := s.repository.WithTx(ctx, func(repo Repository) error {
		task, err := repo.GetTaskForUpdate(ctx, taskID)
		if err != nil {
			return err
		}
		if task == nil {
			return fmt.Errorf("pay reconcile task not found")
		}
		result = taskResultFromTask(*task)
		if task.Status != ReconcilePending && task.Status != ReconcileFailed {
			return nil
		}
		now := time.Now()
		if err := repo.MarkTaskStatus(ctx, task.ID, ReconcileDownload, map[string]any{
			"started_at":  now,
			"finished_at": nil,
			"error_msg":   "",
			"updated_at":  now,
		}); err != nil {
			return err
		}
		copied := *task
		copied.Status = ReconcileDownload
		taskToExecute = &copied
		return nil
	})
	if err != nil || taskToExecute == nil {
		return result, err
	}
	return s.executeStartedTask(ctx, s.repository, *taskToExecute)
}

func (s *Service) executeStartedTask(ctx context.Context, repo Repository, task Task) (*ExecuteTaskResult, error) {
	result := taskResultFromTask(task)
	result.Status = ReconcileDownload
	failTask := func(reason error, fields map[string]any) (*ExecuteTaskResult, error) {
		now := time.Now()
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

	channel, err := repo.FindActiveAlipayChannel(ctx, task.ChannelID)
	if err != nil {
		return failTask(err, nil)
	}
	if channel == nil {
		return failTask(fmt.Errorf("支付宝支付渠道不可用: %d", task.ChannelID), nil)
	}
	cfg, appErr := s.alipayConfig(channel)
	if appErr != nil {
		return failTask(appErr, nil)
	}
	download, err := s.alipayGateway.DownloadBill(ctx, cfg, payalipay.BillDownloadRequest{
		BillDate: task.ReconcileDate.Format(dateLayout),
		BillType: "trade",
	})
	if err != nil {
		return failTask(err, nil)
	}
	if download == nil || len(download.Content) == 0 {
		return failTask(fmt.Errorf("支付宝平台账单内容为空"), nil)
	}
	platformRows, err := parseAlipayTradeBill(download.Content)
	if err != nil {
		return failTask(err, nil)
	}
	platformFileURL, err := s.writePlatformBill(task, platformRows)
	if err != nil {
		return failTask(err, nil)
	}
	platformCount, platformAmount := summarizePlatformRows(platformRows)
	result.PlatformCount = platformCount
	result.PlatformAmount = platformAmount

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
	diffRows := compareBills(rows, platformRows)
	diffFileURL, err := s.writeDiffBill(task, diffRows)
	if err != nil {
		return failTask(err, nil)
	}
	diffCount, diffAmount := summarizeDiffRows(diffRows)
	result.DiffCount = diffCount
	result.DiffAmount = diffAmount
	now := time.Now()
	if err := repo.MarkTaskStatus(ctx, task.ID, ReconcileComparing, map[string]any{
		"platform_count":    platformCount,
		"platform_amount":   platformAmount,
		"platform_file_url": platformFileURL,
		"local_count":       localCount,
		"local_amount":      localAmount,
		"local_file_url":    localFileURL,
		"diff_count":        diffCount,
		"diff_amount":       diffAmount,
		"diff_file_url":     diffFileURL,
		"updated_at":        now,
	}); err != nil {
		return nil, err
	}

	finalStatus := ReconcileSuccess
	if len(diffRows) > 0 {
		finalStatus = ReconcileDiff
	}
	if err := repo.MarkTaskStatus(ctx, task.ID, finalStatus, map[string]any{
		"platform_count":    platformCount,
		"platform_amount":   platformAmount,
		"platform_file_url": platformFileURL,
		"local_count":       localCount,
		"local_amount":      localAmount,
		"local_file_url":    localFileURL,
		"diff_count":        diffCount,
		"diff_amount":       diffAmount,
		"diff_file_url":     diffFileURL,
		"finished_at":       now,
		"error_msg":         "",
		"updated_at":        now,
	}); err != nil {
		return nil, err
	}
	result.Status = finalStatus
	return result, nil
}

func taskResultFromTask(task Task) *ExecuteTaskResult {
	return &ExecuteTaskResult{
		TaskID:         task.ID,
		Status:         task.Status,
		PlatformCount:  task.PlatformCount,
		PlatformAmount: task.PlatformAmount,
		LocalCount:     task.LocalCount,
		LocalAmount:    task.LocalAmount,
		DiffCount:      task.DiffCount,
		DiffAmount:     task.DiffAmount,
	}
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
	file, fileURL, err := s.createReportFile(task, "local")
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
	return fileURL, nil
}

func (s *Service) writePlatformBill(task Task, rows []PlatformBillRow) (string, error) {
	file, fileURL, err := s.createReportFile(task, "platform")
	if err != nil {
		return "", err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"transaction_no", "trade_no", "amount", "paid_at"}); err != nil {
		return "", err
	}
	for _, row := range rows {
		paidAt := ""
		if !row.PaidAt.IsZero() {
			paidAt = row.PaidAt.Format(timeLayout)
		}
		if err := writer.Write([]string{row.TransactionNo, row.TradeNo, formatAmount(row.Amount), paidAt}); err != nil {
			return "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return fileURL, nil
}

func (s *Service) writeDiffBill(task Task, rows []ReconcileDiffRow) (string, error) {
	file, fileURL, err := s.createReportFile(task, "diff")
	if err != nil {
		return "", err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"transaction_no", "trade_no", "reason", "platform_amount", "local_amount", "platform_trade_no", "local_trade_no"}); err != nil {
		return "", err
	}
	for _, row := range rows {
		tradeNo := row.TradeNo
		if tradeNo == "" {
			tradeNo = row.PlatformTradeNo
		}
		if tradeNo == "" {
			tradeNo = row.LocalTradeNo
		}
		if err := writer.Write([]string{
			row.TransactionNo,
			tradeNo,
			row.Reason,
			formatAmount(row.PlatformAmount),
			formatAmount(row.LocalAmount),
			row.PlatformTradeNo,
			row.LocalTradeNo,
		}); err != nil {
			return "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return fileURL, nil
}

func (s *Service) createReportFile(task Task, suffix string) (*os.File, string, error) {
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
		return nil, "", fmt.Errorf("empty reconcile date")
	}
	dir := filepath.Join(baseDir, dateText)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	filename := fmt.Sprintf("%d-%s.csv", task.ID, suffix)
	filePath := filepath.Join(dir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return nil, "", err
	}
	return file, path.Join(prefix, dateText, filename), nil
}

func summarizeBillRows(rows []BillTransactionRow) (int, int64) {
	var amount int64
	for _, row := range rows {
		amount += row.Amount
	}
	return len(rows), amount
}

func summarizePlatformRows(rows []PlatformBillRow) (int, int64) {
	var amount int64
	for _, row := range rows {
		amount += row.Amount
	}
	return len(rows), amount
}

func summarizeDiffRows(rows []ReconcileDiffRow) (int, int64) {
	var amount int64
	for _, row := range rows {
		switch row.Reason {
		case "amount_mismatch":
			amount += abs64(row.PlatformAmount - row.LocalAmount)
		default:
			if row.PlatformAmount > 0 {
				amount += row.PlatformAmount
			}
			if row.LocalAmount > 0 {
				amount += row.LocalAmount
			}
		}
	}
	return len(rows), amount
}

func compareBills(localRows []BillTransactionRow, platformRows []PlatformBillRow) []ReconcileDiffRow {
	localByTxn := make(map[string]BillTransactionRow, len(localRows))
	platformByTxn := make(map[string]PlatformBillRow, len(platformRows))
	keys := make(map[string]struct{}, len(localRows)+len(platformRows))
	for _, row := range localRows {
		key := strings.TrimSpace(row.TransactionNo)
		if key == "" {
			continue
		}
		localByTxn[key] = row
		keys[key] = struct{}{}
	}
	for _, row := range platformRows {
		key := strings.TrimSpace(row.TransactionNo)
		if key == "" {
			continue
		}
		platformByTxn[key] = row
		keys[key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)

	diff := make([]ReconcileDiffRow, 0)
	for _, key := range ordered {
		local, hasLocal := localByTxn[key]
		platform, hasPlatform := platformByTxn[key]
		switch {
		case !hasPlatform:
			diff = append(diff, ReconcileDiffRow{
				TransactionNo: key,
				TradeNo:       local.TradeNo,
				Reason:        "missing_platform",
				LocalAmount:   local.Amount,
				LocalTradeNo:  local.TradeNo,
			})
		case !hasLocal:
			diff = append(diff, ReconcileDiffRow{
				TransactionNo:   key,
				TradeNo:         platform.TradeNo,
				Reason:          "missing_local",
				PlatformAmount:  platform.Amount,
				PlatformTradeNo: platform.TradeNo,
			})
		default:
			if platform.Amount != local.Amount {
				diff = append(diff, ReconcileDiffRow{
					TransactionNo:   key,
					TradeNo:         chooseNonEmpty(platform.TradeNo, local.TradeNo),
					Reason:          "amount_mismatch",
					PlatformAmount:  platform.Amount,
					LocalAmount:     local.Amount,
					PlatformTradeNo: platform.TradeNo,
					LocalTradeNo:    local.TradeNo,
				})
				continue
			}
			if strings.TrimSpace(platform.TradeNo) != "" && strings.TrimSpace(local.TradeNo) != "" && strings.TrimSpace(platform.TradeNo) != strings.TrimSpace(local.TradeNo) {
				diff = append(diff, ReconcileDiffRow{
					TransactionNo:   key,
					TradeNo:         chooseNonEmpty(platform.TradeNo, local.TradeNo),
					Reason:          "trade_no_mismatch",
					PlatformAmount:  platform.Amount,
					LocalAmount:     local.Amount,
					PlatformTradeNo: platform.TradeNo,
					LocalTradeNo:    local.TradeNo,
				})
			}
		}
	}
	return diff
}

func truncateReconcileError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len([]rune(message)) <= 500 {
		return message
	}
	return string([]rune(message)[:500])
}

func (s *Service) alipayConfig(channel *Channel) (payalipay.ChannelConfig, *apperror.Error) {
	if channel == nil {
		return payalipay.ChannelConfig{}, apperror.BadRequest("支付渠道不可用")
	}
	privateKey, err := s.secretbox.Decrypt(channel.AppPrivateKeyEnc)
	if err != nil {
		return payalipay.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "解密支付渠道私钥失败", err)
	}
	appCert, err := s.certResolver.Resolve(channel.PublicCertPath)
	if err != nil {
		return payalipay.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "解析支付宝应用证书失败", err)
	}
	alipayCert, err := s.certResolver.Resolve(channel.PlatformCertPath)
	if err != nil {
		return payalipay.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "解析支付宝平台证书失败", err)
	}
	rootCert, err := s.certResolver.Resolve(channel.RootCertPath)
	if err != nil {
		return payalipay.ChannelConfig{}, apperror.Wrap(apperror.CodeInternal, 500, "解析支付宝根证书失败", err)
	}
	return payalipay.ChannelConfig{
		ChannelID:      channel.ID,
		AppID:          channel.AppID,
		PrivateKey:     privateKey,
		AppCertPath:    appCert,
		AlipayCertPath: alipayCert,
		RootCertPath:   rootCert,
		NotifyURL:      channel.NotifyURL,
		IsSandbox:      channel.IsSandbox == enum.CommonYes,
	}, nil
}

func parseAlipayTradeBill(content []byte) ([]PlatformBillRow, error) {
	content, err := unzipFirstFileIfNeeded(content)
	if err != nil {
		return nil, err
	}
	text, err := decodeAlipayBillText(content)
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(strings.NewReader(text))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	var header map[string]int
	sawHeader := false
	var rows []PlatformBillRow
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解析支付宝账单 CSV 失败: %w", err)
		}
		record = trimCSVRecord(record)
		if len(record) == 0 || isEmptyCSVRecord(record) {
			continue
		}
		if header == nil {
			if candidate := alipayHeaderMap(record); candidate != nil {
				header = candidate
				sawHeader = true
			}
			continue
		}
		if isAlipaySummaryRecord(record) {
			continue
		}
		row, ok, err := platformRowFromRecord(header, record)
		if err != nil {
			return nil, err
		}
		if ok {
			rows = append(rows, row)
		}
	}
	if !sawHeader {
		return nil, fmt.Errorf("支付宝平台账单缺少交易明细表头")
	}
	return rows, nil
}

func unzipFirstFileIfNeeded(content []byte) ([]byte, error) {
	if len(content) < 4 || !bytes.Equal(content[:4], []byte{'P', 'K', 0x03, 0x04}) {
		return content, nil
	}
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("读取支付宝账单 zip 失败: %w", err)
	}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("打开支付宝账单 zip 文件失败: %w", err)
		}
		data, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("读取支付宝账单 zip 文件失败: %w", readErr)
		}
		if closeErr != nil {
			return nil, closeErr
		}
		return data, nil
	}
	return nil, fmt.Errorf("支付宝账单 zip 内没有明细文件")
}

func decodeAlipayBillText(content []byte) (string, error) {
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(content) {
		return string(content), nil
	}
	decoded, err := io.ReadAll(transform.NewReader(bytes.NewReader(content), simplifiedchinese.GBK.NewDecoder()))
	if err != nil {
		return "", fmt.Errorf("支付宝账单 GBK 解码失败: %w", err)
	}
	return string(decoded), nil
}

func alipayHeaderMap(record []string) map[string]int {
	header := make(map[string]int, len(record))
	for i, col := range record {
		key := normalizeAlipayColumn(col)
		if key != "" {
			header[key] = i
		}
	}
	if header["transaction_no"] < 0 {
		return nil
	}
	if _, ok := header["transaction_no"]; !ok {
		return nil
	}
	if _, ok := header["amount"]; !ok {
		return nil
	}
	return header
}

func normalizeAlipayColumn(value string) string {
	value = strings.TrimSpace(strings.Trim(value, "\ufeff"))
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "\t", "")
	value = strings.ToLower(value)
	switch value {
	case "商户订单号", "商户订单号(out_trade_no)", "out_trade_no", "merchantorderno":
		return "transaction_no"
	case "支付宝交易号", "支付宝交易号(trade_no)", "trade_no", "alipaytradeno":
		return "trade_no"
	case "商家实收（元）", "商家实收(元)", "实收金额", "收入金额（+元）", "收入金额(+元)", "total_amount", "amount":
		return "amount"
	case "订单金额（元）", "订单金额(元)", "交易金额", "trans_amount":
		return "fallback_amount"
	case "付款时间", "支付时间", "交易付款时间", "gmt_payment", "paid_at":
		return "paid_at"
	case "交易创建时间", "gmt_create":
		return "created_at"
	case "交易状态", "trade_status":
		return "status"
	default:
		return ""
	}
}

func platformRowFromRecord(header map[string]int, record []string) (PlatformBillRow, bool, error) {
	transactionNo := fieldByHeader(header, record, "transaction_no")
	if transactionNo == "" {
		return PlatformBillRow{}, false, nil
	}
	amountText := fieldByHeader(header, record, "amount")
	if amountText == "" {
		amountText = fieldByHeader(header, record, "fallback_amount")
	}
	if amountText == "" {
		return PlatformBillRow{}, false, nil
	}
	if strings.HasPrefix(strings.TrimSpace(amountText), "-") {
		return PlatformBillRow{}, false, nil
	}
	status := fieldByHeader(header, record, "status")
	if status != "" && !isAlipaySuccessBillStatus(status) {
		return PlatformBillRow{}, false, nil
	}
	amount, err := parseAmountToCents(amountText)
	if err != nil {
		return PlatformBillRow{}, false, fmt.Errorf("解析支付宝账单金额失败: %w", err)
	}
	paidAt := parseBillTime(chooseNonEmpty(fieldByHeader(header, record, "paid_at"), fieldByHeader(header, record, "created_at")))
	raw := make(map[string]string, len(header))
	for key, idx := range header {
		if idx >= 0 && idx < len(record) {
			raw[key] = strings.TrimSpace(record[idx])
		}
	}
	return PlatformBillRow{
		TransactionNo: transactionNo,
		TradeNo:       fieldByHeader(header, record, "trade_no"),
		Amount:        amount,
		PaidAt:        paidAt,
		Raw:           raw,
	}, true, nil
}

func isAlipaySuccessBillStatus(status string) bool {
	status = strings.TrimSpace(strings.ToUpper(status))
	if status == "" {
		return true
	}
	return strings.Contains(status, "成功") ||
		strings.Contains(status, "完成") ||
		strings.Contains(status, "SUCCESS") ||
		strings.Contains(status, "FINISHED")
}

func fieldByHeader(header map[string]int, record []string, key string) string {
	idx, ok := header[key]
	if !ok || idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

func trimCSVRecord(record []string) []string {
	out := make([]string, 0, len(record))
	for _, field := range record {
		out = append(out, strings.TrimSpace(strings.Trim(field, "\ufeff")))
	}
	return out
}

func isEmptyCSVRecord(record []string) bool {
	for _, field := range record {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}

func isAlipaySummaryRecord(record []string) bool {
	first := ""
	if len(record) > 0 {
		first = strings.TrimSpace(record[0])
	}
	if first == "" && len(record) > 1 {
		first = strings.TrimSpace(record[1])
	}
	return strings.Contains(first, "合计") || strings.Contains(first, "汇总") || strings.Contains(first, "总笔数")
}

func parseAmountToCents(amount string) (int64, error) {
	amount = strings.TrimSpace(strings.ReplaceAll(amount, ",", ""))
	amount = strings.TrimPrefix(amount, "+")
	if amount == "" {
		return 0, fmt.Errorf("empty amount")
	}
	if strings.HasPrefix(amount, "-") {
		return 0, fmt.Errorf("negative amount %s", amount)
	}
	parts := strings.Split(amount, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid amount %s", amount)
	}
	yuan, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount %s", amount)
	}
	var fen int64
	if len(parts) == 2 {
		decimals := strings.TrimSpace(parts[1])
		if len(decimals) > 2 {
			return 0, fmt.Errorf("invalid amount precision %s", amount)
		}
		if len(decimals) == 1 {
			decimals += "0"
		}
		if decimals != "" {
			fen, err = strconv.ParseInt(decimals, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid amount %s", amount)
			}
		}
	}
	return yuan*100 + fen, nil
}

func parseBillTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{timeLayout, "2006-01-02 15:04", dateLayout} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func formatAmount(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100)
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func chooseNonEmpty(first string, second string) string {
	first = strings.TrimSpace(first)
	if first != "" {
		return first
	}
	return strings.TrimSpace(second)
}
