package mail

import (
	"time"

	"admin_back_go/internal/dict"
)

const (
	DefaultRegion   = "ap-guangzhou"
	DefaultEndpoint = "ses.tencentcloudapi.com"
)

type PageInitResponse struct {
	Dict PageInitDict `json:"dict"`
}

type PageInitDict struct {
	CommonStatusArr   []dict.Option[int]    `json:"common_status_arr"`
	MailSceneArr      []dict.Option[string] `json:"mail_scene_arr"`
	MailLogSceneArr   []dict.Option[string] `json:"mail_log_scene_arr"`
	MailLogStatusArr  []dict.Option[int]    `json:"mail_log_status_arr"`
	MailRegionArr     []dict.Option[string] `json:"mail_region_arr"`
	DefaultRegion     string                `json:"default_region"`
	DefaultEndpoint   string                `json:"default_endpoint"`
	DefaultTTLMinutes int                   `json:"default_ttl_minutes"`
}

type ConfigResponse struct {
	ID                   *uint64 `json:"id"`
	Configured           bool    `json:"configured"`
	SecretIDHint         string  `json:"secret_id_hint"`
	SecretKeyHint        string  `json:"secret_key_hint"`
	Region               string  `json:"region"`
	Endpoint             string  `json:"endpoint"`
	FromEmail            string  `json:"from_email"`
	FromName             string  `json:"from_name"`
	ReplyTo              string  `json:"reply_to"`
	Status               int     `json:"status"`
	VerifyCodeTTLMinutes int     `json:"verify_code_ttl_minutes"`
	LastTestAt           *string `json:"last_test_at"`
	LastTestError        string  `json:"last_test_error"`
	CreatedAt            *string `json:"created_at"`
	UpdatedAt            *string `json:"updated_at"`
}

type SaveConfigInput struct {
	SecretID             string
	SecretKey            string
	Region               string
	Endpoint             string
	FromEmail            string
	FromName             string
	ReplyTo              string
	Status               int
	VerifyCodeTTLMinutes int
}

type TestInput struct {
	ToEmail       string
	TemplateScene string
}

type TemplateDTO struct {
	ID                uint64            `json:"id"`
	Scene             string            `json:"scene"`
	Name              string            `json:"name"`
	Subject           string            `json:"subject"`
	TencentTemplateID uint64            `json:"tencent_template_id"`
	Variables         []string          `json:"variables"`
	SampleVariables   map[string]string `json:"sample_variables"`
	Status            int               `json:"status"`
	CreatedAt         string            `json:"created_at"`
	UpdatedAt         string            `json:"updated_at"`
}

type SaveTemplateInput struct {
	Scene             string
	Name              string
	Subject           string
	TencentTemplateID uint64
	Variables         []string
	SampleVariables   map[string]string
	Status            int
}

type TemplateUpdate struct {
	Scene               string
	Name                string
	Subject             string
	TencentTemplateID   uint64
	VariablesJSON       string
	SampleVariablesJSON string
	Status              int
}

type LogQuery struct {
	CurrentPage    int
	PageSize       int
	Scene          string
	Status         *int
	ToEmail        string
	CreatedAtStart *time.Time
	CreatedAtEnd   *time.Time
}

type LogListResponse struct {
	List []LogDTO `json:"list"`
	Page Page     `json:"page"`
}

type LogDTO struct {
	ID               uint64          `json:"id"`
	Scene            string          `json:"scene"`
	TemplateID       *uint64         `json:"template_id"`
	ToEmail          string          `json:"to_email"`
	Subject          string          `json:"subject"`
	Status           int             `json:"status"`
	TencentRequestID string          `json:"tencent_request_id"`
	TencentMessageID string          `json:"tencent_message_id"`
	ErrorCode        string          `json:"error_code"`
	ErrorMessage     string          `json:"error_message"`
	DurationMS       uint64          `json:"duration_ms"`
	SentAt           *string         `json:"sent_at"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
	Template         *LogTemplateDTO `json:"template,omitempty"`
}

type LogTemplateDTO struct {
	ID                uint64   `json:"id"`
	Scene             string   `json:"scene"`
	Name              string   `json:"name"`
	TencentTemplateID uint64   `json:"tencent_template_id"`
	Variables         []string `json:"variables"`
	Status            int      `json:"status"`
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type LogFinish struct {
	Status       int
	RequestID    string
	MessageID    string
	ErrorCode    string
	ErrorMessage string
	DurationMS   uint64
	SentAt       *time.Time
}

type SendInput struct {
	SecretID     string
	SecretKey    string
	Region       string
	Endpoint     string
	FromEmail    string
	FromName     string
	ReplyTo      string
	ToEmail      string
	Subject      string
	TemplateID   uint64
	TemplateData map[string]string
}

type SendResult struct {
	RequestID string
	MessageID string
}
