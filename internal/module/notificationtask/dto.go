package notificationtask

import (
	"context"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
)

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	NotificationTypeArr       []dict.Option[int]    `json:"notification_type_arr"`
	NotificationLevelArr      []dict.Option[int]    `json:"notification_level_arr"`
	NotificationTargetTypeArr []dict.Option[int]    `json:"notification_target_type_arr"`
	NotificationTaskStatusArr []dict.Option[int]    `json:"notification_task_status_arr"`
	PlatformArr               []dict.Option[string] `json:"platformArr"`
}

type StatusCountQuery struct {
	Title string
}

type StatusCountItem struct {
	Label string `json:"label"`
	Value int    `json:"value"`
	Num   int64  `json:"num"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Status      *int
	Title       string
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
	Title          string  `json:"title"`
	Content        string  `json:"content"`
	Type           int     `json:"type"`
	TypeText       string  `json:"type_text"`
	Level          int     `json:"level"`
	LevelText      string  `json:"level_text"`
	Link           string  `json:"link"`
	Platform       string  `json:"platform"`
	PlatformText   string  `json:"platform_text"`
	TargetType     int     `json:"target_type"`
	TargetTypeText string  `json:"target_type_text"`
	Status         int     `json:"status"`
	StatusText     string  `json:"status_text"`
	TotalCount     int     `json:"total_count"`
	SentCount      int     `json:"sent_count"`
	SendAt         *string `json:"send_at"`
	ErrorMsg       *string `json:"error_msg"`
	CreatedAt      string  `json:"created_at"`
}

type CreateInput struct {
	Title      string
	Content    string
	Type       int
	Level      int
	Link       string
	Platform   string
	TargetType int
	TargetIDs  []int64
	SendAt     string
	CreatedBy  int64
}

type CreateResponse struct {
	ID     int64 `json:"id"`
	Queued bool  `json:"queued"`
}

type SendTaskInput struct {
	TaskID int64
}

type DispatchDueInput struct {
	Now   time.Time
	Limit int
}

type DispatchDueResult struct {
	Claimed int `json:"claimed"`
	Queued  int `json:"queued"`
}

type SendTaskResult struct {
	TaskID int64 `json:"task_id"`
	Sent   int   `json:"sent"`
	Noop   bool  `json:"noop"`
}

type HTTPService interface {
	Init(ctx context.Context) (*InitResponse, *apperror.Error)
	StatusCount(ctx context.Context, query StatusCountQuery) ([]StatusCountItem, *apperror.Error)
	List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error)
	Create(ctx context.Context, input CreateInput) (*CreateResponse, *apperror.Error)
	Cancel(ctx context.Context, id int64) *apperror.Error
	Delete(ctx context.Context, id int64) *apperror.Error
}
