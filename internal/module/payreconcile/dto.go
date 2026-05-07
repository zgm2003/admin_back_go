package payreconcile

import (
	"context"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

const (
	BillTypePay = 1
)

const (
	ReconcilePending   = 1
	ReconcileDownload  = 2
	ReconcileComparing = 3
	ReconcileSuccess   = 4
	ReconcileDiff      = 5
	ReconcileFailed    = 6
)

var ReconcileStatuses = []int{
	ReconcilePending,
	ReconcileDownload,
	ReconcileComparing,
	ReconcileSuccess,
	ReconcileDiff,
	ReconcileFailed,
}

var ReconcileStatusLabels = map[int]string{
	ReconcilePending:   "待执行",
	ReconcileDownload:  "下载中",
	ReconcileComparing: "对比中",
	ReconcileSuccess:   "成功",
	ReconcileDiff:      "有差异",
	ReconcileFailed:    "失败",
}

var BillTypeLabels = map[int]string{
	BillTypePay: "支付",
}

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	PayChannelArr      []dict.Option[int] `json:"pay_channel_arr"`
	ChannelArr         []dict.Option[int] `json:"channel_arr"`
	ReconcileStatusArr []dict.Option[int] `json:"reconcile_status_arr"`
	BillTypeArr        []dict.Option[int] `json:"bill_type_arr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Channel     *int
	Status      *int
	BillType    *int
	StartDate   string
	EndDate     string
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
	ID             int64   `json:"id"`
	ReconcileDate  string  `json:"reconcile_date"`
	Channel        int     `json:"channel"`
	ChannelText    string  `json:"channel_text"`
	BillType       int     `json:"bill_type"`
	BillTypeText   string  `json:"bill_type_text"`
	Status         int     `json:"status"`
	StatusText     string  `json:"status_text"`
	PlatformCount  int     `json:"platform_count"`
	PlatformAmount int64   `json:"platform_amount"`
	LocalCount     int     `json:"local_count"`
	LocalAmount    int64   `json:"local_amount"`
	DiffCount      int     `json:"diff_count"`
	DiffAmount     int64   `json:"diff_amount"`
	StartedAt      *string `json:"started_at"`
	FinishedAt     *string `json:"finished_at"`
	CreatedAt      string  `json:"created_at"`
}

type DetailResponse struct {
	Task DetailTask `json:"task"`
}

type DetailTask struct {
	ID              int64   `json:"id"`
	ReconcileDate   string  `json:"reconcile_date"`
	Channel         int     `json:"channel"`
	ChannelText     string  `json:"channel_text"`
	ChannelID       int64   `json:"channel_id"`
	BillType        int     `json:"bill_type"`
	BillTypeText    string  `json:"bill_type_text"`
	Status          int     `json:"status"`
	StatusText      string  `json:"status_text"`
	PlatformCount   int     `json:"platform_count"`
	PlatformAmount  int64   `json:"platform_amount"`
	LocalCount      int     `json:"local_count"`
	LocalAmount     int64   `json:"local_amount"`
	DiffCount       int     `json:"diff_count"`
	DiffAmount      int64   `json:"diff_amount"`
	PlatformFileURL string  `json:"platform_file_url"`
	LocalFileURL    string  `json:"local_file_url"`
	DiffFileURL     string  `json:"diff_file_url"`
	StartedAt       *string `json:"started_at"`
	FinishedAt      *string `json:"finished_at"`
	ErrorMsg        string  `json:"error_msg"`
	CreatedAt       string  `json:"created_at"`
}

type FileResponse struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
}

type CreateDailyTasksInput struct {
	Date  string
	Limit int
	Now   time.Time
}

type CreateDailyTasksResult struct {
	Date     string `json:"date"`
	Scanned  int    `json:"scanned"`
	Created  int    `json:"created"`
	Existing int    `json:"existing"`
	Skipped  int    `json:"skipped"`
}

type ExecutePendingTasksInput struct {
	Limit int
	Now   time.Time
}

type ExecutePendingTasksResult struct {
	Scanned int `json:"scanned"`
	Success int `json:"success"`
	Diff    int `json:"diff"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type ExecuteTaskResult struct {
	TaskID         int64 `json:"task_id"`
	Status         int   `json:"status"`
	PlatformCount  int   `json:"platform_count"`
	PlatformAmount int64 `json:"platform_amount"`
	LocalCount     int   `json:"local_count"`
	LocalAmount    int64 `json:"local_amount"`
	DiffCount      int   `json:"diff_count"`
	DiffAmount     int64 `json:"diff_amount"`
}

type BillTransactionRow struct {
	TransactionNo string
	OrderNo       string
	TradeNo       string
	Amount        int64
	Status        int
	PaidAt        time.Time
}

type ChannelSummary struct {
	ID      int64
	Name    string
	Channel int
}

type Task struct {
	ID              int64      `gorm:"column:id;primaryKey"`
	ReconcileDate   time.Time  `gorm:"column:reconcile_date"`
	Channel         int        `gorm:"column:channel"`
	ChannelID       int64      `gorm:"column:channel_id"`
	BillType        int        `gorm:"column:bill_type"`
	Status          int        `gorm:"column:status"`
	PlatformCount   int        `gorm:"column:platform_count"`
	PlatformAmount  int64      `gorm:"column:platform_amount"`
	LocalCount      int        `gorm:"column:local_count"`
	LocalAmount     int64      `gorm:"column:local_amount"`
	DiffCount       int        `gorm:"column:diff_count"`
	DiffAmount      int64      `gorm:"column:diff_amount"`
	PlatformFileURL string     `gorm:"column:platform_file_url"`
	LocalFileURL    string     `gorm:"column:local_file_url"`
	DiffFileURL     string     `gorm:"column:diff_file_url"`
	StartedAt       *time.Time `gorm:"column:started_at"`
	FinishedAt      *time.Time `gorm:"column:finished_at"`
	ErrorMsg        string     `gorm:"column:error_msg"`
	IsDel           int        `gorm:"column:is_del"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (Task) TableName() string { return "pay_reconcile_tasks" }

type ListRow struct {
	ID             int64
	ReconcileDate  time.Time
	Channel        int
	ChannelID      int64
	BillType       int
	Status         int
	PlatformCount  int
	PlatformAmount int64
	LocalCount     int
	LocalAmount    int64
	DiffCount      int
	DiffAmount     int64
	StartedAt      *time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Detail(ctx context.Context, id int64) (*DetailResponse, *apperror.Error)
	Retry(ctx context.Context, id int64) *apperror.Error
	File(ctx context.Context, id int64, fileType string) (*FileResponse, *apperror.Error)
}
