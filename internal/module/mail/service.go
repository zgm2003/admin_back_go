package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/secretbox"
)

const (
	timeLayout        = "2006-01-02 15:04:05"
	defaultAppName    = "admin_go"
	defaultPage       = 1
	defaultPageSize   = 20
	maxTemplateVarLen = 64
)

var simpleEmailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

type Sender interface {
	Send(ctx context.Context, input SendInput) (SendResult, error)
}

type SenderFunc func(ctx context.Context, input SendInput) (SendResult, error)

func (f SenderFunc) Send(ctx context.Context, input SendInput) (SendResult, error) {
	return f(ctx, input)
}

type codedError interface {
	ErrorCode() string
}

type Service struct {
	repository Repository
	secretBox  secretbox.Box
	sender     Sender
}

func NewService(repository Repository, secretBox secretbox.Box, sender Sender) *Service {
	return &Service{repository: repository, secretBox: secretBox, sender: sender}
}

func (s *Service) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	return &PageInitResponse{Dict: PageInitDict{
		CommonStatusArr:  dict.CommonStatusOptions(),
		MailSceneArr:     dict.MailSceneOptions(),
		MailLogSceneArr:  dict.MailLogSceneOptions(),
		MailLogStatusArr: dict.MailLogStatusOptions(),
		DefaultRegion:    DefaultRegion,
		DefaultEndpoint:  DefaultEndpoint,
	}}, nil
}

func (s *Service) Config(ctx context.Context) (*ConfigResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.DefaultConfig(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件配置失败", err)
	}
	if row == nil {
		return defaultConfigResponse(), nil
	}
	return configResponseFromRow(*row), nil
}

func (s *Service) SaveConfig(ctx context.Context, input SaveConfigInput) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	input, appErr = normalizeConfigInput(input)
	if appErr != nil {
		return appErr
	}
	existing, err := repo.DefaultConfig(ctx)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件配置失败", err)
	}
	row, appErr := s.configRowFromInput(existing, input)
	if appErr != nil {
		return appErr
	}
	if err := repo.SaveDefaultConfig(ctx, row); err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "保存邮件配置失败", err)
	}
	return nil
}

func (s *Service) DeleteConfig(ctx context.Context) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.SoftDeleteDefaultConfig(ctx); err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "删除邮件配置失败", err)
	}
	return nil
}

func (s *Service) TestSend(ctx context.Context, input TestInput) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	input.ToEmail = strings.TrimSpace(input.ToEmail)
	input.TemplateScene = strings.TrimSpace(input.TemplateScene)
	if !isEmail(input.ToEmail) {
		return apperror.BadRequest("测试收件邮箱格式不正确")
	}
	if !enum.IsMailTemplateScene(input.TemplateScene) {
		return apperror.BadRequest("无效的邮件模板场景")
	}
	sample, appErr := s.sampleTemplateData(ctx, input.TemplateScene)
	if appErr != nil {
		return appErr
	}
	sentAt := time.Now()
	appErr = s.send(ctx, input.TemplateScene, enum.MailSceneTest, input.ToEmail, sample)
	message := ""
	if appErr != nil {
		message = appErr.Message
	}
	if err := repo.UpdateConfigTestResult(ctx, &sentAt, message); err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "更新邮件测试结果失败", err)
	}
	return appErr
}

func (s *Service) Templates(ctx context.Context) ([]TemplateDTO, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	rows, err := repo.ListTemplates(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件模板失败", err)
	}
	items := make([]TemplateDTO, 0, len(rows))
	for _, row := range rows {
		item, err := templateDTOFromRow(row)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "解析邮件模板失败", err)
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) CreateTemplate(ctx context.Context, input SaveTemplateInput) (uint64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := templateRowFromInput(input)
	if appErr != nil {
		return 0, appErr
	}
	id, err := repo.SaveTemplate(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "保存邮件模板失败", err)
	}
	return id, nil
}

func (s *Service) UpdateTemplate(ctx context.Context, id uint64, input SaveTemplateInput) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的邮件模板ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if appErr := ensureTemplateExists(ctx, repo, id); appErr != nil {
		return appErr
	}
	row, appErr := templateRowFromInput(input)
	if appErr != nil {
		return appErr
	}
	update := TemplateUpdate{
		Scene: row.Scene, Name: row.Name, Subject: row.Subject, TencentTemplateID: row.TencentTemplateID,
		VariablesJSON: row.VariablesJSON, SampleVariablesJSON: row.SampleVariablesJSON, Status: row.Status,
	}
	if err := repo.UpdateTemplate(ctx, id, update); err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "更新邮件模板失败", err)
	}
	return nil
}

func (s *Service) ChangeTemplateStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的邮件模板ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.TemplateByID(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件模板失败", err)
	}
	if row == nil {
		return apperror.NotFound("邮件模板不存在")
	}
	if err := repo.UpdateTemplate(ctx, id, TemplateUpdate{
		Scene: row.Scene, Name: row.Name, Subject: row.Subject, TencentTemplateID: row.TencentTemplateID,
		VariablesJSON: row.VariablesJSON, SampleVariablesJSON: row.SampleVariablesJSON, Status: status,
	}); err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "修改邮件模板状态失败", err)
	}
	return nil
}

func (s *Service) DeleteTemplate(ctx context.Context, id uint64) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的邮件模板ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if appErr := ensureTemplateExists(ctx, repo, id); appErr != nil {
		return appErr
	}
	if err := repo.SoftDeleteTemplate(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "删除邮件模板失败", err)
	}
	return nil
}

func (s *Service) Logs(ctx context.Context, query LogQuery) (*LogListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query, appErr = normalizeLogQuery(query)
	if appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.ListLogs(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件日志失败", err)
	}
	list := make([]LogDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, logDTOFromRow(row))
	}
	return &LogListResponse{List: list, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize, Total: total, TotalPage: totalPage(total, query.PageSize)}}, nil
}

func (s *Service) Log(ctx context.Context, id uint64) (*LogDTO, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的邮件日志ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.LogByID(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件日志失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("邮件日志不存在")
	}
	result := logDTOFromRow(*row)
	return &result, nil
}

func (s *Service) DeleteLogs(ctx context.Context, ids []uint64) *apperror.Error {
	ids = normalizeUint64IDs(ids)
	if len(ids) == 0 {
		return apperror.BadRequest("请选择要删除的邮件日志")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.SoftDeleteLogs(ctx, ids); err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "删除邮件日志失败", err)
	}
	return nil
}

func (s *Service) SendVerifyCode(ctx context.Context, scene string, toEmail string, code string, ttl time.Duration) *apperror.Error {
	scene = strings.TrimSpace(scene)
	toEmail = strings.TrimSpace(toEmail)
	code = strings.TrimSpace(code)
	if !enum.IsMailTemplateScene(scene) {
		return apperror.BadRequest("无效的邮件验证码场景")
	}
	if !isEmail(toEmail) {
		return apperror.BadRequest("邮箱格式不正确")
	}
	if code == "" {
		return apperror.BadRequest("验证码不能为空")
	}
	data := map[string]string{
		"code":        code,
		"ttl_minutes": ttlMinutes(ttl),
		"app_name":    defaultAppName,
	}
	return s.send(ctx, scene, scene, toEmail, data)
}

func (s *Service) send(ctx context.Context, templateScene string, logScene string, toEmail string, data map[string]string) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	sender, appErr := s.requireSender()
	if appErr != nil {
		return appErr
	}
	cfg, appErr := s.enabledConfig(ctx, repo)
	if appErr != nil {
		return appErr
	}
	tmpl, appErr := enabledTemplate(ctx, repo, templateScene)
	if appErr != nil {
		return appErr
	}
	variables, err := decodeVariables(tmpl.VariablesJSON)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "解析邮件模板变量失败", err)
	}
	if appErr := ensureTemplateDataCoversVariables(variables, data); appErr != nil {
		return appErr
	}
	secretID, secretKey, appErr := s.decryptCredentials(*cfg)
	if appErr != nil {
		return appErr
	}
	templateID := tmpl.ID
	logID, err := repo.CreateLog(ctx, Log{
		Scene: logScene, TemplateID: &templateID, ToEmail: toEmail, Subject: tmpl.Subject, Status: enum.MailLogStatusPending,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "写入邮件日志失败", err)
	}
	started := time.Now()
	result, err := sender.Send(ctx, SendInput{
		SecretID: secretID, SecretKey: secretKey, Region: cfg.Region, Endpoint: cfg.Endpoint,
		FromEmail: cfg.FromEmail, FromName: cfg.FromName, ReplyTo: cfg.ReplyTo,
		ToEmail: toEmail, Subject: tmpl.Subject, TemplateID: tmpl.TencentTemplateID, TemplateData: data,
	})
	duration := uint64(time.Since(started).Milliseconds())
	if err != nil {
		finish := LogFinish{Status: enum.MailLogStatusFailed, ErrorCode: senderErrorCode(err), ErrorMessage: err.Error(), DurationMS: duration}
		_ = repo.FinishLog(ctx, logID, finish)
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "邮件发送失败", err)
	}
	sentAt := time.Now()
	finish := LogFinish{Status: enum.MailLogStatusSuccess, RequestID: result.RequestID, MessageID: result.MessageID, DurationMS: duration, SentAt: &sentAt}
	if err := repo.FinishLog(ctx, logID, finish); err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "更新邮件日志失败", err)
	}
	return nil
}

func (s *Service) sampleTemplateData(ctx context.Context, scene string) (map[string]string, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	tmpl, appErr := enabledTemplate(ctx, repo, scene)
	if appErr != nil {
		return nil, appErr
	}
	sample, err := decodeSampleVariables(tmpl.SampleVariablesJSON)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "解析邮件测试变量失败", err)
	}
	return sample, nil
}

func (s *Service) enabledConfig(ctx context.Context, repo Repository) (*Config, *apperror.Error) {
	cfg, err := repo.DefaultConfig(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件配置失败", err)
	}
	if cfg == nil {
		return nil, apperror.Internal("邮件服务未配置")
	}
	if cfg.Status != enum.CommonYes {
		return nil, apperror.BadRequest("邮件服务已禁用")
	}
	return cfg, nil
}

func enabledTemplate(ctx context.Context, repo Repository, scene string) (*Template, *apperror.Error) {
	tmpl, err := repo.TemplateByScene(ctx, scene)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件模板失败", err)
	}
	if tmpl == nil {
		return nil, apperror.BadRequest("邮件模板未配置")
	}
	if tmpl.Status != enum.CommonYes {
		return nil, apperror.BadRequest("邮件模板已禁用")
	}
	return tmpl, nil
}

func (s *Service) decryptCredentials(cfg Config) (string, string, *apperror.Error) {
	secretID, err := s.secretBox.Decrypt(cfg.SecretIDEnc)
	if err != nil {
		return "", "", apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "解密 Tencent SecretId 失败", err)
	}
	secretKey, err := s.secretBox.Decrypt(cfg.SecretKeyEnc)
	if err != nil {
		return "", "", apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "解密 Tencent SecretKey 失败", err)
	}
	if secretID == "" || secretKey == "" {
		return "", "", apperror.Internal("腾讯云邮件密钥未配置")
	}
	return secretID, secretKey, nil
}

func (s *Service) configRowFromInput(existing *Config, input SaveConfigInput) (Config, *apperror.Error) {
	secretIDEnc, secretIDHint, appErr := s.secretValue(existing, input.SecretID, true)
	if appErr != nil {
		return Config{}, appErr
	}
	secretKeyEnc, secretKeyHint, appErr := s.secretValue(existing, input.SecretKey, false)
	if appErr != nil {
		return Config{}, appErr
	}
	return Config{
		ConfigKey: defaultConfigKey, SecretIDEnc: secretIDEnc, SecretIDHint: secretIDHint,
		SecretKeyEnc: secretKeyEnc, SecretKeyHint: secretKeyHint, Region: input.Region,
		Endpoint: input.Endpoint, FromEmail: input.FromEmail, FromName: input.FromName,
		ReplyTo: input.ReplyTo, Status: input.Status, IsDel: enum.CommonNo,
	}, nil
}

func (s *Service) secretValue(existing *Config, plain string, secretID bool) (string, string, *apperror.Error) {
	if plain != "" {
		enc, err := s.secretBox.Encrypt(plain)
		if err != nil {
			return "", "", apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "加密邮件密钥失败", err)
		}
		return enc, secretbox.Hint(plain), nil
	}
	if existing == nil {
		return "", "", apperror.BadRequest("首次配置必须填写腾讯云 SecretId 和 SecretKey")
	}
	if secretID {
		return existing.SecretIDEnc, existing.SecretIDHint, nil
	}
	return existing.SecretKeyEnc, existing.SecretKeyHint, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "邮件仓储未配置", ErrRepositoryNotConfigured)
	}
	return s.repository, nil
}

func (s *Service) requireSender() (Sender, *apperror.Error) {
	if s == nil || s.sender == nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "邮件发送器未配置", ErrSenderNotConfigured)
	}
	return s.sender, nil
}

func normalizeConfigInput(input SaveConfigInput) (SaveConfigInput, *apperror.Error) {
	input.SecretID = strings.TrimSpace(input.SecretID)
	input.SecretKey = strings.TrimSpace(input.SecretKey)
	input.Region = strings.TrimSpace(input.Region)
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.FromEmail = strings.TrimSpace(input.FromEmail)
	input.FromName = strings.TrimSpace(input.FromName)
	input.ReplyTo = strings.TrimSpace(input.ReplyTo)
	if input.Region == "" {
		input.Region = DefaultRegion
	}
	if input.Endpoint == "" {
		input.Endpoint = DefaultEndpoint
	}
	if !isEmail(input.FromEmail) {
		return input, apperror.BadRequest("发件邮箱格式不正确")
	}
	if input.ReplyTo != "" && !isEmail(input.ReplyTo) {
		return input, apperror.BadRequest("回复邮箱格式不正确")
	}
	if !enum.IsCommonStatus(input.Status) {
		return input, apperror.BadRequest("无效的状态")
	}
	return input, nil
}

func templateRowFromInput(input SaveTemplateInput) (Template, *apperror.Error) {
	input.Scene = strings.TrimSpace(input.Scene)
	input.Name = strings.TrimSpace(input.Name)
	input.Subject = strings.TrimSpace(input.Subject)
	if !enum.IsMailTemplateScene(input.Scene) {
		return Template{}, apperror.BadRequest("无效的邮件模板场景")
	}
	if input.Name == "" {
		return Template{}, apperror.BadRequest("模板名称不能为空")
	}
	if input.Subject == "" {
		return Template{}, apperror.BadRequest("邮件主题不能为空")
	}
	if input.TencentTemplateID == 0 {
		return Template{}, apperror.BadRequest("腾讯云模板ID不能为空")
	}
	if !enum.IsCommonStatus(input.Status) {
		return Template{}, apperror.BadRequest("无效的状态")
	}
	variablesJSON, appErr := encodeVariables(input.Variables)
	if appErr != nil {
		return Template{}, appErr
	}
	sampleJSON, appErr := encodeSampleVariables(input.SampleVariables, input.Variables)
	if appErr != nil {
		return Template{}, appErr
	}
	return Template{
		Scene: input.Scene, Name: input.Name, Subject: input.Subject, TencentTemplateID: input.TencentTemplateID,
		VariablesJSON: variablesJSON, SampleVariablesJSON: sampleJSON, Status: input.Status, IsDel: enum.CommonNo,
	}, nil
}

func encodeVariables(values []string) (string, *apperror.Error) {
	normalized, appErr := normalizeVariables(values)
	if appErr != nil {
		return "", appErr
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return "", apperror.BadRequest("模板变量格式错误")
	}
	return string(body), nil
}

func decodeVariables(raw string) ([]string, error) {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return normalizeVariablesAsError(values)
}

func encodeSampleVariables(values map[string]string, variables []string) (string, *apperror.Error) {
	normalizedVars, appErr := normalizeVariables(variables)
	if appErr != nil {
		return "", appErr
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			return "", apperror.BadRequest("测试变量名不能为空")
		}
		normalized[key] = strings.TrimSpace(value)
	}
	for _, key := range normalizedVars {
		if _, ok := normalized[key]; !ok {
			return "", apperror.BadRequest("测试变量缺少 " + key)
		}
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return "", apperror.BadRequest("测试变量格式错误")
	}
	return string(body), nil
}

func decodeSampleVariables(raw string) (map[string]string, error) {
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	if values == nil {
		return map[string]string{}, nil
	}
	return values, nil
}

func normalizeVariables(values []string) ([]string, *apperror.Error) {
	result, err := normalizeVariablesAsError(values)
	if err != nil {
		return nil, apperror.BadRequest(err.Error())
	}
	return result, nil
}

func normalizeVariablesAsError(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("模板变量不能为空")
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("模板变量名不能为空")
		}
		if len([]rune(value)) > maxTemplateVarLen {
			return nil, fmt.Errorf("模板变量名过长")
		}
		if _, ok := seen[value]; ok {
			return nil, fmt.Errorf("模板变量名重复: %s", value)
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func ensureTemplateDataCoversVariables(variables []string, data map[string]string) *apperror.Error {
	for _, key := range variables {
		if _, ok := data[key]; !ok {
			return apperror.Internal("邮件模板变量缺少 " + key)
		}
	}
	return nil
}

func normalizeLogQuery(query LogQuery) (LogQuery, *apperror.Error) {
	if query.CurrentPage <= 0 {
		query.CurrentPage = defaultPage
	}
	if query.PageSize <= 0 {
		query.PageSize = defaultPageSize
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.Scene = strings.TrimSpace(query.Scene)
	query.ToEmail = strings.TrimSpace(query.ToEmail)
	if query.Scene != "" && !enum.IsMailLogScene(query.Scene) {
		return query, apperror.BadRequest("无效的邮件日志场景")
	}
	if query.Status != nil && !enum.IsMailLogStatus(*query.Status) {
		return query, apperror.BadRequest("无效的邮件日志状态")
	}
	return query, nil
}

func ensureTemplateExists(ctx context.Context, repo Repository, id uint64) *apperror.Error {
	row, err := repo.TemplateByID(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件模板失败", err)
	}
	if row == nil {
		return apperror.NotFound("邮件模板不存在")
	}
	return nil
}

func templateDTOFromRow(row Template) (TemplateDTO, error) {
	variables, err := decodeVariables(row.VariablesJSON)
	if err != nil {
		return TemplateDTO{}, err
	}
	sample, err := decodeSampleVariables(row.SampleVariablesJSON)
	if err != nil {
		return TemplateDTO{}, err
	}
	return TemplateDTO{
		ID: row.ID, Scene: row.Scene, Name: row.Name, Subject: row.Subject, TencentTemplateID: row.TencentTemplateID,
		Variables: variables, SampleVariables: sample, Status: row.Status, CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}, nil
}

func logDTOFromRow(row Log) LogDTO {
	return LogDTO{
		ID: row.ID, Scene: row.Scene, TemplateID: row.TemplateID, ToEmail: row.ToEmail, Subject: row.Subject,
		Status: row.Status, TencentRequestID: row.TencentRequestID, TencentMessageID: row.TencentMessageID,
		ErrorCode: row.ErrorCode, ErrorMessage: row.ErrorMessage, DurationMS: row.DurationMS,
		SentAt: formatOptionalTime(row.SentAt), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func configResponseFromRow(row Config) *ConfigResponse {
	id := row.ID
	return &ConfigResponse{
		ID: &id, Configured: true, SecretIDHint: row.SecretIDHint, SecretKeyHint: row.SecretKeyHint,
		Region: row.Region, Endpoint: row.Endpoint, FromEmail: row.FromEmail, FromName: row.FromName,
		ReplyTo: row.ReplyTo, Status: row.Status, LastTestAt: formatOptionalTime(row.LastTestAt),
		LastTestError: row.LastTestError, CreatedAt: optionalTime(row.CreatedAt), UpdatedAt: optionalTime(row.UpdatedAt),
	}
}

func defaultConfigResponse() *ConfigResponse {
	return &ConfigResponse{Configured: false, Region: DefaultRegion, Endpoint: DefaultEndpoint, Status: enum.CommonNo}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}

func optionalTime(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	formatted := value.Format(timeLayout)
	return &formatted
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := value.Format(timeLayout)
	return &formatted
}

func parseTime(value string) (*time.Time, *apperror.Error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation(timeLayout, value, time.Local)
	if err != nil {
		return nil, apperror.BadRequest("时间格式必须为 YYYY-MM-DD HH:mm:ss")
	}
	return &parsed, nil
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func ttlMinutes(ttl time.Duration) string {
	minutes := int(math.Ceil(ttl.Minutes()))
	if minutes <= 0 {
		minutes = 1
	}
	return fmt.Sprintf("%d", minutes)
}

func isEmail(value string) bool {
	return simpleEmailPattern.MatchString(strings.TrimSpace(value))
}

func senderErrorCode(err error) string {
	var coded codedError
	if err != nil && errors.As(err, &coded) {
		return coded.ErrorCode()
	}
	return ""
}
