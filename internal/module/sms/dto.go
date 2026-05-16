package sms

import (
	"time"

	"admin_back_go/internal/dict"
)

const (
	DefaultRegion   = "ap-guangzhou"
	DefaultEndpoint = "sms.tencentcloudapi.com"
)

type PageInitResponse struct {
	Dict PageInitDict `json:"dict"`
}

type PageInitDict struct {
	CommonStatusArr   []dict.Option[int]    `json:"common_status_arr"`
	SmsSceneArr       []dict.Option[string] `json:"sms_scene_arr"`
	SmsLogSceneArr    []dict.Option[string] `json:"sms_log_scene_arr"`
	SmsLogStatusArr   []dict.Option[int]    `json:"sms_log_status_arr"`
	SmsRegionArr      []dict.Option[string] `json:"sms_region_arr"`
	DefaultRegion     string                `json:"default_region"`
	DefaultEndpoint   string                `json:"default_endpoint"`
	DefaultTTLMinutes int                   `json:"default_ttl_minutes"`
}

type ConfigResponse struct {
	ID                   *uint64 `json:"id"`
	Configured           bool    `json:"configured"`
	SecretIDHint         string  `json:"secret_id_hint"`
	SecretKeyHint        string  `json:"secret_key_hint"`
	SmsSdkAppID          string  `json:"sms_sdk_app_id"`
	SignName             string  `json:"sign_name"`
	Region               string  `json:"region"`
	Endpoint             string  `json:"endpoint"`
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
	SmsSdkAppID          string
	SignName             string
	Region               string
	Endpoint             string
	Status               int
	VerifyCodeTTLMinutes int
}

type TestInput struct {
	ToPhone       string
	TemplateScene string
}

type TemplateDTO struct {
	ID                uint64            `json:"id"`
	Scene             string            `json:"scene"`
	Name              string            `json:"name"`
	TencentTemplateID string            `json:"tencent_template_id"`
	Variables         []string          `json:"variables"`
	SampleVariables   map[string]string `json:"sample_variables"`
	Status            int               `json:"status"`
	CreatedAt         string            `json:"created_at"`
	UpdatedAt         string            `json:"updated_at"`
}

type SaveTemplateInput struct {
	Scene             string
	Name              string
	TencentTemplateID string
	Variables         []string
	SampleVariables   map[string]string
	Status            int
}

type TemplateUpdate struct {
	Scene               string
	Name                string
	TencentTemplateID   string
	VariablesJSON       string
	SampleVariablesJSON string
	Status              int
}

type LogQuery struct {
	CurrentPage    int
	PageSize       int
	Scene          string
	Status         *int
	ToPhone        string
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
	ToPhone          string          `json:"to_phone"`
	Status           int             `json:"status"`
	TencentRequestID string          `json:"tencent_request_id"`
	TencentSerialNo  string          `json:"tencent_serial_no"`
	TencentFee       uint64          `json:"tencent_fee"`
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
	TencentTemplateID string   `json:"tencent_template_id"`
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
	SerialNo     string
	Fee          uint64
	ErrorCode    string
	ErrorMessage string
	DurationMS   uint64
	SentAt       *time.Time
}

type SendInput struct {
	SecretID         string
	SecretKey        string
	Region           string
	Endpoint         string
	SmsSdkAppID      string
	SignName         string
	TemplateID       string
	PhoneNumber      string
	TemplateParamSet []string
}

type SendResult struct {
	RequestID string
	SerialNo  string
	Fee       uint64
}
