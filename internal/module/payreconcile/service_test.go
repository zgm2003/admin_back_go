package payreconcile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	channels             []ChannelSummary
	listRows             []ListRow
	total                int64
	detail               *Task
	existingTasks        map[string]*Task
	createdTasks         []Task
	pendingTasks         []Task
	billRows             []BillTransactionRow
	markedStatuses       []int
	markedFields         []map[string]any
	lastQuery            ListQuery
	lastID               int64
	updatedID            int64
	updates              map[string]any
	activeChannelCalls   int
	findReconcileLookups []string
}

func (f *fakeRepository) ActivePayChannels(ctx context.Context) ([]ChannelSummary, error) {
	f.activeChannelCalls++
	return f.channels, nil
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.lastQuery = query
	return f.listRows, f.total, nil
}

func (f *fakeRepository) Detail(ctx context.Context, id int64) (*Task, error) {
	f.lastID = id
	return f.detail, nil
}

func (f *fakeRepository) UpdateRetry(ctx context.Context, id int64, fields map[string]any) error {
	f.updatedID = id
	f.updates = fields
	return nil
}

func (f *fakeRepository) FindReconcileTask(ctx context.Context, channelID int64, reconcileDate time.Time, billType int) (*Task, error) {
	key := reconcileKey(channelID, reconcileDate, billType)
	f.findReconcileLookups = append(f.findReconcileLookups, key)
	if f.existingTasks == nil {
		return nil, nil
	}
	return f.existingTasks[key], nil
}

func (f *fakeRepository) CreateReconcileTask(ctx context.Context, task Task) error {
	f.createdTasks = append(f.createdTasks, task)
	return nil
}

func (f *fakeRepository) ListPendingTasks(ctx context.Context, limit int) ([]Task, error) {
	if limit > 0 && len(f.pendingTasks) > limit {
		return f.pendingTasks[:limit], nil
	}
	return f.pendingTasks, nil
}

func (f *fakeRepository) GetTaskForUpdate(ctx context.Context, id int64) (*Task, error) {
	f.lastID = id
	if f.detail != nil {
		return f.detail, nil
	}
	for i := range f.pendingTasks {
		if f.pendingTasks[i].ID == id {
			return &f.pendingTasks[i], nil
		}
	}
	return nil, nil
}

func (f *fakeRepository) MarkTaskStatus(ctx context.Context, id int64, status int, fields map[string]any) error {
	f.updatedID = id
	f.markedStatuses = append(f.markedStatuses, status)
	f.markedFields = append(f.markedFields, fields)
	return nil
}

func (f *fakeRepository) ListSuccessfulTransactionsForBill(ctx context.Context, channelID int64, reconcileDate time.Time) ([]BillTransactionRow, error) {
	return f.billRows, nil
}

func (f *fakeRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	return fn(f)
}

func TestInitReturnsPayReconcileDicts(t *testing.T) {
	repo := &fakeRepository{channels: []ChannelSummary{{ID: 1, Name: "支付宝官方沙盒", Channel: enum.PayChannelAlipay}}}
	service := NewService(repo)

	got, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	if len(got.Dict.PayChannelArr) != 1 || got.Dict.PayChannelArr[0].Value != 1 || got.Dict.PayChannelArr[0].Label != "支付宝官方沙盒" {
		t.Fatalf("unexpected pay channel dict: %#v", got.Dict.PayChannelArr)
	}
	if len(got.Dict.ReconcileStatusArr) != 6 || got.Dict.ReconcileStatusArr[5].Value != ReconcileFailed {
		t.Fatalf("unexpected reconcile status dict: %#v", got.Dict.ReconcileStatusArr)
	}
	if len(got.Dict.BillTypeArr) != 1 || got.Dict.BillTypeArr[0].Value != BillTypePay {
		t.Fatalf("unexpected bill type dict: %#v", got.Dict.BillTypeArr)
	}
}

func TestListDefaultsPageAndMapsLabels(t *testing.T) {
	started := time.Date(2026, 5, 5, 17, 10, 0, 0, time.Local)
	finished := time.Date(2026, 5, 5, 17, 10, 6, 0, time.Local)
	created := time.Date(2026, 5, 5, 17, 0, 0, 0, time.Local)
	repo := &fakeRepository{listRows: []ListRow{{
		ID: 4, ReconcileDate: date(2026, 5, 5), Channel: enum.PayChannelAlipay, ChannelID: 1, BillType: BillTypePay,
		Status: ReconcileFailed, LocalCount: 3, LocalAmount: 1000,
		StartedAt: &started, FinishedAt: &finished, CreatedAt: created,
	}}, total: 1}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), ListQuery{})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if got.Page.CurrentPage != 1 || got.Page.PageSize != 20 || got.Page.TotalPage != 1 || got.Page.Total != 1 {
		t.Fatalf("unexpected page: %#v", got.Page)
	}
	item := got.List[0]
	if item.ChannelText != "支付宝" || item.BillTypeText != "支付" || item.StatusText != "失败" {
		t.Fatalf("unexpected labels: %#v", item)
	}
	if item.ReconcileDate != "2026-05-05" || item.StartedAt == nil || *item.StartedAt != "2026-05-05 17:10:00" || item.FinishedAt == nil || *item.FinishedAt != "2026-05-05 17:10:06" || item.CreatedAt != "2026-05-05 17:00:00" {
		t.Fatalf("unexpected times: %#v", item)
	}
}

func TestDetailReturnsTaskAndFileURLs(t *testing.T) {
	created := time.Date(2026, 5, 5, 17, 0, 0, 0, time.Local)
	repo := &fakeRepository{detail: &Task{
		ID: 4, ReconcileDate: date(2026, 5, 5), Channel: enum.PayChannelAlipay, ChannelID: 1, BillType: BillTypePay,
		Status: ReconcileFailed, PlatformFileURL: "https://cos.example/platform.csv", LocalFileURL: "https://cos.example/local.csv", DiffFileURL: "https://cos.example/diff.csv",
		ErrorMsg: "响应异常", CreatedAt: created,
	}}
	service := NewService(repo)

	got, appErr := service.Detail(context.Background(), 4)
	if appErr != nil {
		t.Fatalf("expected detail to succeed, got %v", appErr)
	}
	if got.Task.ID != 4 || got.Task.ChannelID != 1 || got.Task.PlatformFileURL == "" || got.Task.LocalFileURL == "" || got.Task.DiffFileURL == "" || got.Task.ErrorMsg != "响应异常" {
		t.Fatalf("unexpected detail: %#v", got.Task)
	}
}

func TestDetailMissingRowReturnsNotFound(t *testing.T) {
	service := NewService(&fakeRepository{})

	_, appErr := service.Detail(context.Background(), 404)
	if appErr == nil || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("expected not found, got %#v", appErr)
	}
}

func TestRetryOnlyAllowsFailedTasksAndClearsExecutionFields(t *testing.T) {
	repo := &fakeRepository{detail: &Task{ID: 4, Status: ReconcileFailed}}
	service := NewService(repo)

	appErr := service.Retry(context.Background(), 4)
	if appErr != nil {
		t.Fatalf("expected retry to succeed, got %v", appErr)
	}
	if repo.updatedID != 4 {
		t.Fatalf("expected update id 4, got %d", repo.updatedID)
	}
	if repo.updates["status"] != ReconcilePending || repo.updates["platform_count"] != 0 || repo.updates["local_count"] != 0 || repo.updates["diff_count"] != 0 || repo.updates["platform_file_url"] != "" || repo.updates["local_file_url"] != "" || repo.updates["diff_file_url"] != "" || repo.updates["error_msg"] != "" {
		t.Fatalf("retry fields not reset correctly: %#v", repo.updates)
	}
	if repo.updates["started_at"] != nil || repo.updates["finished_at"] != nil {
		t.Fatalf("retry must clear timestamps: %#v", repo.updates)
	}
}

func TestRetryRejectsNonFailedTask(t *testing.T) {
	service := NewService(&fakeRepository{detail: &Task{ID: 4, Status: ReconcileSuccess}})

	appErr := service.Retry(context.Background(), 4)
	if appErr == nil || appErr.Message != "当前状态不支持重试" {
		t.Fatalf("expected status rejection, got %#v", appErr)
	}
}

func TestFileRejectsInvalidTypeAndEmptyURL(t *testing.T) {
	service := NewService(&fakeRepository{detail: &Task{ID: 4, Status: ReconcileFailed}})

	_, appErr := service.File(context.Background(), 4, "bad")
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected invalid type error, got %#v", appErr)
	}

	_, appErr = service.File(context.Background(), 4, "platform")
	if appErr == nil || appErr.Message != "当前文件不存在" {
		t.Fatalf("expected empty file error, got %#v", appErr)
	}
}

func TestFileReturnsURLAndFilename(t *testing.T) {
	service := NewService(&fakeRepository{detail: &Task{ID: 4, PlatformFileURL: "https://cos.example/reconcile_reports/2026-05-05/platform_bill_4.csv"}})

	got, appErr := service.File(context.Background(), 4, "platform")
	if appErr != nil {
		t.Fatalf("expected file to succeed, got %v", appErr)
	}
	if got.URL != "https://cos.example/reconcile_reports/2026-05-05/platform_bill_4.csv" || got.Filename != "platform_bill_4.csv" {
		t.Fatalf("unexpected file response: %#v", got)
	}
}

func TestCreateDailyTasksDefaultsToYesterdayAndCreatesPendingTasks(t *testing.T) {
	now := time.Date(2026, 5, 7, 10, 0, 0, 0, time.Local)
	repo := &fakeRepository{channels: []ChannelSummary{
		{ID: 1, Name: "支付宝官方沙盒", Channel: enum.PayChannelAlipay},
		{ID: 2, Name: "微信未启用样例", Channel: enum.PayChannelWechat},
	}}
	service := NewService(repo)

	got, err := service.CreateDailyTasks(context.Background(), CreateDailyTasksInput{Now: now})
	if err != nil {
		t.Fatalf("CreateDailyTasks returned error: %v", err)
	}
	if got.Date != "2026-05-06" || got.Scanned != 2 || got.Created != 2 || got.Existing != 0 || got.Skipped != 0 {
		t.Fatalf("unexpected result: %#v", got)
	}
	if len(repo.createdTasks) != 2 {
		t.Fatalf("expected two tasks, got %#v", repo.createdTasks)
	}
	first := repo.createdTasks[0]
	if first.ReconcileDate.Format(dateLayout) != "2026-05-06" || first.Channel != enum.PayChannelAlipay || first.ChannelID != 1 || first.BillType != BillTypePay || first.Status != ReconcilePending || first.IsDel != enum.CommonNo {
		t.Fatalf("unexpected first created task: %#v", first)
	}
	if first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps on created task: %#v", first)
	}
}

func TestCreateDailyTasksIsIdempotentForExistingChannelDateBillType(t *testing.T) {
	reconcileDate := date(2026, 5, 6)
	repo := &fakeRepository{
		channels: []ChannelSummary{
			{ID: 1, Name: "支付宝官方沙盒", Channel: enum.PayChannelAlipay},
			{ID: 2, Name: "微信未启用样例", Channel: enum.PayChannelWechat},
		},
		existingTasks: map[string]*Task{
			reconcileKey(1, reconcileDate, BillTypePay): {ID: 9, ChannelID: 1, ReconcileDate: reconcileDate, BillType: BillTypePay},
		},
	}
	service := NewService(repo)

	got, err := service.CreateDailyTasks(context.Background(), CreateDailyTasksInput{Date: "2026-05-06", Now: time.Date(2026, 5, 7, 10, 0, 0, 0, time.Local)})
	if err != nil {
		t.Fatalf("CreateDailyTasks returned error: %v", err)
	}
	if got.Date != "2026-05-06" || got.Scanned != 2 || got.Created != 1 || got.Existing != 1 || got.Skipped != 0 {
		t.Fatalf("unexpected result: %#v", got)
	}
	if len(repo.createdTasks) != 1 || repo.createdTasks[0].ChannelID != 2 {
		t.Fatalf("expected only channel 2 to be created, got %#v", repo.createdTasks)
	}
}

func TestCreateDailyTasksLimitCapsActiveChannels(t *testing.T) {
	repo := &fakeRepository{channels: []ChannelSummary{
		{ID: 1, Name: "支付宝官方沙盒", Channel: enum.PayChannelAlipay},
		{ID: 2, Name: "微信未启用样例", Channel: enum.PayChannelWechat},
		{ID: 3, Name: "支付宝备用", Channel: enum.PayChannelAlipay},
	}}
	service := NewService(repo)

	got, err := service.CreateDailyTasks(context.Background(), CreateDailyTasksInput{Date: "2026-05-06", Limit: 2, Now: time.Date(2026, 5, 7, 10, 0, 0, 0, time.Local)})
	if err != nil {
		t.Fatalf("CreateDailyTasks returned error: %v", err)
	}
	if got.Scanned != 2 || got.Created != 2 || got.Skipped != 1 {
		t.Fatalf("unexpected result: %#v", got)
	}
	if len(repo.createdTasks) != 2 {
		t.Fatalf("expected two created tasks, got %#v", repo.createdTasks)
	}
}

func TestCreateDailyTasksRejectsInvalidDate(t *testing.T) {
	service := NewService(&fakeRepository{})

	_, err := service.CreateDailyTasks(context.Background(), CreateDailyTasksInput{Date: "2026/05/06"})
	if err == nil {
		t.Fatalf("expected invalid date error")
	}
}

func TestExecutePendingTasksProcessesPendingTasksAndCountsFailures(t *testing.T) {
	repo := &fakeRepository{pendingTasks: []Task{
		{ID: 9, ReconcileDate: date(2026, 5, 6), Channel: enum.PayChannelAlipay, ChannelID: 1, BillType: BillTypePay, Status: ReconcilePending},
	}}
	service := NewService(repo)
	service.reportBaseDir = t.TempDir()
	service.reportPathPrefix = "runtime/reconcile_reports"

	got, err := service.ExecutePendingTasks(context.Background(), ExecutePendingTasksInput{Limit: 20, Now: time.Date(2026, 5, 7, 10, 0, 0, 0, time.Local)})
	if err != nil {
		t.Fatalf("ExecutePendingTasks returned error: %v", err)
	}
	if got.Scanned != 1 || got.Failed != 1 || got.Success != 0 || got.Diff != 0 || got.Skipped != 0 {
		t.Fatalf("unexpected result: %#v", got)
	}
	if !sameStatuses(repo.markedStatuses, []int{ReconcileDownload, ReconcileComparing, ReconcileFailed}) {
		t.Fatalf("unexpected status transitions: %#v", repo.markedStatuses)
	}
}

func TestExecuteTaskSkipsNonPendingOrFailedTask(t *testing.T) {
	repo := &fakeRepository{detail: &Task{ID: 9, ReconcileDate: date(2026, 5, 6), Channel: enum.PayChannelAlipay, ChannelID: 1, BillType: BillTypePay, Status: ReconcileSuccess}}
	service := NewService(repo)

	got, err := service.ExecuteTask(context.Background(), 9)
	if err != nil {
		t.Fatalf("ExecuteTask returned error: %v", err)
	}
	if got.TaskID != 9 || got.Status != ReconcileSuccess {
		t.Fatalf("unexpected result: %#v", got)
	}
	if len(repo.markedStatuses) != 0 {
		t.Fatalf("non-pending task must not mutate status: %#v", repo.markedStatuses)
	}
}

func TestExecuteTaskMarksUnsupportedChannelFailed(t *testing.T) {
	repo := &fakeRepository{detail: &Task{ID: 10, ReconcileDate: date(2026, 5, 6), Channel: enum.PayChannelWechat, ChannelID: 2, BillType: BillTypePay, Status: ReconcilePending}}
	service := NewService(repo)

	got, err := service.ExecuteTask(context.Background(), 10)
	if err != nil {
		t.Fatalf("ExecuteTask returned error: %v", err)
	}
	if got.Status != ReconcileFailed {
		t.Fatalf("expected failed status, got %#v", got)
	}
	if !sameStatuses(repo.markedStatuses, []int{ReconcileDownload, ReconcileFailed}) {
		t.Fatalf("unexpected transitions: %#v", repo.markedStatuses)
	}
	errorMsg := fmt.Sprint(repo.markedFields[len(repo.markedFields)-1]["error_msg"])
	if !strings.Contains(errorMsg, "不支持") {
		t.Fatalf("expected clear unsupported error, got %q", errorMsg)
	}
}

func TestExecuteTaskWritesLocalBillAndFailsWhenPlatformDownloadNotImplemented(t *testing.T) {
	reconcileDate := date(2026, 5, 6)
	repo := &fakeRepository{
		detail: &Task{ID: 9, ReconcileDate: reconcileDate, Channel: enum.PayChannelAlipay, ChannelID: 1, BillType: BillTypePay, Status: ReconcilePending},
		billRows: []BillTransactionRow{
			{TransactionNo: "T1", OrderNo: "R1", TradeNo: "A1", Amount: 1000, Status: enum.PayTxnSuccess, PaidAt: time.Date(2026, 5, 6, 9, 0, 0, 0, time.Local)},
			{TransactionNo: "T2", OrderNo: "R2", TradeNo: "A2", Amount: 2500, Status: enum.PayTxnSuccess, PaidAt: time.Date(2026, 5, 6, 10, 0, 0, 0, time.Local)},
		},
	}
	service := NewService(repo)
	service.reportBaseDir = t.TempDir()
	service.reportPathPrefix = "runtime/reconcile_reports"

	got, err := service.ExecuteTask(context.Background(), 9)
	if err != nil {
		t.Fatalf("ExecuteTask returned error: %v", err)
	}
	if got.TaskID != 9 || got.Status != ReconcileFailed || got.LocalCount != 2 || got.LocalAmount != 3500 || got.PlatformCount != 0 || got.PlatformAmount != 0 {
		t.Fatalf("unexpected result: %#v", got)
	}
	if !sameStatuses(repo.markedStatuses, []int{ReconcileDownload, ReconcileComparing, ReconcileFailed}) {
		t.Fatalf("unexpected transitions: %#v", repo.markedStatuses)
	}
	localURL := fmt.Sprint(repo.markedFields[1]["local_file_url"])
	if localURL != "runtime/reconcile_reports/2026-05-06/9-local.csv" {
		t.Fatalf("unexpected local file url: %q", localURL)
	}
	localFile := filepath.Join(service.reportBaseDir, "2026-05-06", "9-local.csv")
	content, readErr := os.ReadFile(localFile)
	if readErr != nil {
		t.Fatalf("expected local bill file: %v", readErr)
	}
	if !strings.Contains(string(content), "transaction_no,order_no,trade_no,amount,paid_at") || !strings.Contains(string(content), "T2,R2,A2,25.00,2026-05-06 10:00:00") {
		t.Fatalf("unexpected local bill csv:\n%s", string(content))
	}
	finalError := fmt.Sprint(repo.markedFields[len(repo.markedFields)-1]["error_msg"])
	if !strings.Contains(finalError, "平台账单下载未实现") {
		t.Fatalf("expected not implemented error, got %q", finalError)
	}
}

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

func reconcileKey(channelID int64, reconcileDate time.Time, billType int) string {
	return fmt.Sprintf("%s|%d|%d", reconcileDate.Format(dateLayout), channelID, billType)
}

func sameStatuses(got []int, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
