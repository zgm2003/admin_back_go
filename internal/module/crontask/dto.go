package crontask

import (
	"context"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type ListQuery struct {
	CurrentPage    int
	PageSize       int
	Title          string
	Name           string
	Status         *int
	RegistryStatus string
}

type LogsQuery struct {
	TaskID      int64
	CurrentPage int
	PageSize    int
	Status      *int
	StartDate   string
	EndDate     string
}

type SaveInput struct {
	Name         string
	Title        string
	Description  string
	Cron         string
	CronReadable string
	Handler      string
	Status       int
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	CronPresetArr          []dict.Option[string] `json:"cron_preset_arr"`
	CronTaskStatusArr      []dict.Option[int]    `json:"cron_task_status_arr"`
	CronTaskRegistryStatus []dict.Option[string] `json:"cron_task_registry_status_arr"`
	CronTaskLogStatusArr   []dict.Option[int]    `json:"cron_task_log_status_arr"`
}

type ListResponse struct {
	List []ListItem `json:"list"`
	Page Page       `json:"page"`
}

type ListItem struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Title               string `json:"title"`
	Description         string `json:"description"`
	Cron                string `json:"cron"`
	CronReadable        string `json:"cron_readable"`
	Handler             string `json:"handler"`
	Status              int    `json:"status"`
	StatusName          string `json:"status_name"`
	NextRunTime         string `json:"next_run_time"`
	RegistryStatus      string `json:"registry_status"`
	RegistryStatusText  string `json:"registry_status_text"`
	RegistryTaskType    string `json:"registry_task_type"`
	RegistryDescription string `json:"registry_description"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type LogsResponse struct {
	List []LogItem `json:"list"`
	Page Page      `json:"page"`
}

type LogItem struct {
	ID         int64   `json:"id"`
	TaskID     int64   `json:"task_id"`
	TaskName   string  `json:"task_name"`
	StartTime  *string `json:"start_time"`
	EndTime    *string `json:"end_time"`
	DurationMS *int64  `json:"duration_ms"`
	Status     int     `json:"status"`
	StatusName string  `json:"status_name"`
	Result     *string `json:"result"`
	ErrorMsg   *string `json:"error_msg"`
	CreatedAt  string  `json:"created_at"`
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input SaveInput) (*ListItem, *apperror.Error)
	Update(ctx context.Context, id int64, input SaveInput) *apperror.Error
	ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error
	Delete(ctx context.Context, ids []int64) *apperror.Error
	Logs(ctx context.Context, query LogsQuery) (*LogsResponse, *apperror.Error)
}

const timeLayout = "2006-01-02 15:04:05"

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}
