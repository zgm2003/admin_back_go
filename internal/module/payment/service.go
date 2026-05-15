package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	gateway "admin_back_go/internal/platform/payment"
	"admin_back_go/internal/platform/secretbox"
)

const timeLayout = "2006-01-02 15:04:05"

const (
	environmentSandbox    = "sandbox"
	environmentProduction = "production"
	providerAlipay        = enum.PaymentProviderAlipay
	certTypeApp           = "app_cert"
	certTypeAlipay        = "alipay_cert"
	certTypeAlipayRoot    = "alipay_root_cert"
)

var configCodePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

type secretCodec interface {
	Encrypt(plain string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

type certResolver interface {
	Resolve(path string) (string, error)
}

type certificateStore interface {
	Save(ctx context.Context, file gateway.CertificateFile) (*gateway.CertificateSaveResult, error)
}

type noopCertResolver struct{}

func (noopCertResolver) Resolve(path string) (string, error) { return strings.TrimSpace(path), nil }

type Dependencies struct {
	Repository   Repository
	Gateway      gateway.Gateway
	Secretbox    secretCodec
	CertResolver certResolver
	CertStore    certificateStore
	Now          func() time.Time
}

type Service struct {
	repository   Repository
	gateway      gateway.Gateway
	secretbox    secretCodec
	certResolver certResolver
	certStore    certificateStore
	now          func() time.Time
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	box := deps.Secretbox
	if box == nil {
		box = secretbox.New(nil)
	}
	resolver := deps.CertResolver
	if resolver == nil {
		resolver = noopCertResolver{}
	}
	return &Service{
		repository:   deps.Repository,
		gateway:      deps.Gateway,
		secretbox:    box,
		certResolver: resolver,
		certStore:    deps.CertStore,
		now:          now,
	}
}

func (s *Service) ConfigInit(ctx context.Context) (*ConfigInitResponse, *apperror.Error) {
	_ = ctx
	return &ConfigInitResponse{Dict: ConfigInitDict{
		ProviderArr:        paymentProviderOptions(),
		EnvironmentArr:     paymentEnvironmentOptions(),
		CommonStatusArr:    dict.CommonStatusOptions(),
		EnabledMethodArr:   dict.PaymentMethodOptions(),
		CertificateTypeArr: paymentCertificateTypeOptions(),
	}}, nil
}

func (s *Service) ListConfigs(ctx context.Context, query ConfigListQuery) (*ConfigListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query.Name = strings.TrimSpace(query.Name)
	query.Provider = strings.TrimSpace(query.Provider)
	query.Environment = strings.TrimSpace(query.Environment)
	rows, total, err := repo.ListConfigs(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询支付配置失败", err)
	}
	list := make([]ConfigListItem, 0, len(rows))
	for _, row := range rows {
		item, appErr := configListItem(row)
		if appErr != nil {
			return nil, appErr
		}
		list = append(list, item)
	}
	_, size, _ := normalizePage(query.CurrentPage, query.PageSize)
	return &ConfigListResponse{List: list, Page: Page{PageSize: size, CurrentPage: currentPage(query.CurrentPage), TotalPage: totalPage(total, size), Total: total}}, nil
}

func (s *Service) CreateConfig(ctx context.Context, input ConfigMutationInput) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	cfg, _, appErr := s.normalizeMutation(input, nil, true)
	if appErr != nil {
		return 0, appErr
	}
	if cfg.Status == enum.CommonYes {
		if _, appErr := s.testConfigRow(ctx, cfg); appErr != nil {
			return 0, appErr
		}
	}
	id, err := repo.CreateConfig(ctx, cfg)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "新增支付配置失败", err)
	}
	return id, nil
}

func (s *Service) UpdateConfig(ctx context.Context, id int64, input ConfigMutationInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的支付配置ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	existing, err := repo.GetConfig(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询支付配置失败", err)
	}
	if existing == nil {
		return apperror.NotFound("支付配置不存在")
	}
	cfg, keepPrivateKey, appErr := s.normalizeMutation(input, existing, false)
	if appErr != nil {
		return appErr
	}
	cfg.ID = id
	if cfg.Status == enum.CommonYes {
		if _, appErr := s.testConfigRow(ctx, cfg); appErr != nil {
			return appErr
		}
	}
	if err := repo.UpdateConfig(ctx, cfg, keepPrivateKey); err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "编辑支付配置失败", err)
	}
	return nil
}

func (s *Service) ChangeConfigStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 || !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的支付配置状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if status == enum.CommonYes {
		cfg, err := repo.GetConfig(ctx, id)
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询支付配置失败", err)
		}
		if cfg == nil {
			return apperror.NotFound("支付配置不存在")
		}
		if _, appErr := s.testConfigRow(ctx, *cfg); appErr != nil {
			return appErr
		}
	}
	if err := repo.ChangeConfigStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "切换支付配置状态失败", err)
	}
	return nil
}

func (s *Service) DeleteConfig(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的支付配置ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.DeleteConfig(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "删除支付配置失败", err)
	}
	return nil
}

func (s *Service) UploadCertificate(ctx context.Context, input CertificateUploadInput) (*CertificateUploadResponse, *apperror.Error) {
	if s == nil || s.certStore == nil {
		return nil, apperror.Internal("支付证书存储未配置")
	}
	result, err := s.certStore.Save(ctx, gateway.CertificateFile{
		ConfigCode: input.ConfigCode,
		CertType:   input.CertType,
		FileName:   input.FileName,
		Size:       input.Size,
		Reader:     input.Reader,
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeBadRequest, http.StatusBadRequest, "上传支付宝证书失败", err)
	}
	return &CertificateUploadResponse{Path: result.Path, FileName: result.FileName, SHA256: result.SHA256, Size: result.Size}, nil
}

func (s *Service) TestConfig(ctx context.Context, id int64) (*ConfigTestResponse, *apperror.Error) {
	if id <= 0 {
		return nil, apperror.BadRequest("无效的支付配置ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	cfg, err := repo.GetConfig(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询支付配置失败", err)
	}
	if cfg == nil {
		return nil, apperror.NotFound("支付配置不存在")
	}
	return s.testConfigRow(ctx, *cfg)
}

func (s *Service) normalizeMutation(input ConfigMutationInput, existing *Config, create bool) (Config, bool, *apperror.Error) {
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = providerAlipay
	}
	if provider != providerAlipay {
		return Config{}, false, apperror.BadRequest("当前仅支持支付宝支付配置")
	}
	code := strings.TrimSpace(input.Code)
	if code == "" || !configCodePattern.MatchString(code) {
		return Config{}, false, apperror.BadRequest("支付配置编码只能包含小写字母、数字、下划线和中划线")
	}
	if existing != nil && code != existing.Code {
		return Config{}, false, apperror.BadRequest("支付配置编码创建后不能修改")
	}
	if existing != nil && provider != existing.Provider {
		return Config{}, false, apperror.BadRequest("支付配置供应商创建后不能修改")
	}
	name := strings.TrimSpace(input.Name)
	appID := strings.TrimSpace(input.AppID)
	if name == "" || appID == "" {
		return Config{}, false, apperror.BadRequest("支付配置名称和支付宝AppID不能为空")
	}
	environment := strings.TrimSpace(input.Environment)
	if !isPaymentEnvironment(environment) {
		return Config{}, false, apperror.BadRequest("无效的支付宝环境")
	}
	methods, appErr := normalizeEnabledMethods(input.EnabledMethods)
	if appErr != nil {
		return Config{}, false, appErr
	}
	if !enum.IsCommonStatus(input.Status) {
		return Config{}, false, apperror.BadRequest("无效的支付配置状态")
	}
	notifyURL := strings.TrimSpace(input.NotifyURL)
	if !isHTTPURL(notifyURL) {
		return Config{}, false, apperror.BadRequest("支付宝异步通知地址必须是 http 或 https URL")
	}
	key := strings.TrimSpace(input.AppPrivateKey)
	keepPrivateKey := !create && key == ""
	keyEnc := ""
	keyHint := ""
	if keepPrivateKey {
		keyEnc = existing.PrivateKeyEnc
		keyHint = existing.PrivateKeyHint
	} else {
		if key == "" {
			return Config{}, false, apperror.BadRequest("新增支付配置必须填写应用私钥")
		}
		enc, err := s.secretbox.Encrypt(key)
		if err != nil {
			return Config{}, false, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "加密支付宝应用私钥失败", err)
		}
		keyEnc = enc
		keyHint = secretbox.Hint(key)
	}
	methodsJSON, err := json.Marshal(methods)
	if err != nil {
		return Config{}, false, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "编码支付方式失败", err)
	}
	return Config{
		Provider:           provider,
		Code:               code,
		Name:               name,
		AppID:              appID,
		PrivateKeyEnc:      keyEnc,
		PrivateKeyHint:     keyHint,
		AppCertPath:        strings.TrimSpace(input.AppCertPath),
		PlatformCertPath:   strings.TrimSpace(input.PlatformCertPath),
		RootCertPath:       strings.TrimSpace(input.RootCertPath),
		NotifyURL:          notifyURL,
		Environment:        environment,
		EnabledMethodsJSON: string(methodsJSON),
		Status:             input.Status,
		Remark:             strings.TrimSpace(input.Remark),
		IsDel:              enum.CommonNo,
	}, keepPrivateKey, nil
}

func (s *Service) testConfigRow(ctx context.Context, cfg Config) (*ConfigTestResponse, *apperror.Error) {
	if s == nil || s.gateway == nil {
		return nil, apperror.Internal("支付宝网关未配置")
	}
	if strings.TrimSpace(cfg.Provider) != providerAlipay {
		return nil, apperror.BadRequest("当前仅支持支付宝支付配置")
	}
	if !isPaymentEnvironment(strings.TrimSpace(cfg.Environment)) {
		return nil, apperror.BadRequest("无效的支付宝环境")
	}
	if _, appErr := decodeEnabledMethods(cfg.EnabledMethodsJSON); appErr != nil {
		return nil, appErr
	}
	privateKey, err := s.secretbox.Decrypt(cfg.PrivateKeyEnc)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "解密支付宝应用私钥失败", err)
	}
	if strings.TrimSpace(privateKey) == "" {
		return nil, apperror.BadRequest("支付宝应用私钥未配置")
	}
	appCertPath, err := s.certResolver.Resolve(cfg.AppCertPath)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeBadRequest, http.StatusBadRequest, "应用公钥证书不可用", err)
	}
	alipayCertPath, err := s.certResolver.Resolve(cfg.PlatformCertPath)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeBadRequest, http.StatusBadRequest, "支付宝公钥证书不可用", err)
	}
	rootCertPath, err := s.certResolver.Resolve(cfg.RootCertPath)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeBadRequest, http.StatusBadRequest, "支付宝根证书不可用", err)
	}
	platformCfg := gateway.ChannelConfig{
		Provider:         cfg.Provider,
		AppID:            cfg.AppID,
		PrivateKey:       privateKey,
		AppCertPath:      appCertPath,
		PlatformCertPath: alipayCertPath,
		RootCertPath:     rootCertPath,
		NotifyURL:        cfg.NotifyURL,
		IsSandbox:        cfg.Environment == environmentSandbox,
	}
	if err := s.gateway.TestConfig(ctx, platformCfg); err != nil {
		return nil, apperror.Wrap(apperror.CodeBadRequest, http.StatusBadRequest, "支付宝配置测试失败", err)
	}
	return &ConfigTestResponse{OK: true, Checks: []string{"private_key_decrypted", "cert_paths_resolved", "alipay_client_built"}, Message: "支付宝配置本地校验通过"}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("支付配置仓库未配置")
	}
	return s.repository, nil
}

func configListItem(row Config) (ConfigListItem, *apperror.Error) {
	methods, appErr := decodeEnabledMethods(row.EnabledMethodsJSON)
	if appErr != nil {
		return ConfigListItem{}, appErr
	}
	return ConfigListItem{
		ID:                 row.ID,
		Provider:           row.Provider,
		ProviderText:       providerText(row.Provider),
		Code:               row.Code,
		Name:               row.Name,
		AppID:              row.AppID,
		PrivateKeyHint:     row.PrivateKeyHint,
		AppCertPath:        row.AppCertPath,
		PlatformCertPath:   row.PlatformCertPath,
		RootCertPath:       row.RootCertPath,
		NotifyURL:          row.NotifyURL,
		Environment:        row.Environment,
		EnvironmentText:    environmentText(row.Environment),
		EnabledMethods:     methods,
		EnabledMethodsText: methodText(methods),
		Status:             row.Status,
		StatusText:         commonStatusText(row.Status),
		Remark:             row.Remark,
		CreatedAt:          formatTime(row.CreatedAt),
		UpdatedAt:          formatTime(row.UpdatedAt),
	}, nil
}

func normalizeEnabledMethods(values []string) ([]string, *apperror.Error) {
	seen := map[string]bool{}
	methods := make([]string, 0, len(values))
	for _, value := range values {
		method := strings.TrimSpace(value)
		if !enum.IsPaymentMethod(method) {
			return nil, apperror.BadRequest("无效的支付方式")
		}
		if seen[method] {
			continue
		}
		seen[method] = true
		methods = append(methods, method)
	}
	if len(methods) == 0 {
		return nil, apperror.BadRequest("至少选择一个支付方式")
	}
	sort.Slice(methods, func(i, j int) bool {
		return methodOrder(methods[i]) < methodOrder(methods[j])
	})
	return methods, nil
}

func decodeEnabledMethods(raw string) ([]string, *apperror.Error) {
	var methods []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &methods); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "解析支付方式失败", err)
	}
	return normalizeEnabledMethods(methods)
}

func methodOrder(method string) int {
	for idx, value := range enum.PaymentMethods {
		if value == method {
			return idx
		}
	}
	return len(enum.PaymentMethods)
}

func isPaymentEnvironment(value string) bool {
	return value == environmentSandbox || value == environmentProduction
}

func paymentProviderOptions() []dict.Option[string] {
	return []dict.Option[string]{
		{Label: "支付宝", Value: providerAlipay},
	}
}

func isHTTPURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func paymentEnvironmentOptions() []dict.Option[string] {
	return []dict.Option[string]{
		{Label: "沙箱环境", Value: environmentSandbox},
		{Label: "正式环境", Value: environmentProduction},
	}
}

func paymentCertificateTypeOptions() []dict.Option[string] {
	return []dict.Option[string]{
		{Label: "应用公钥证书", Value: certTypeApp},
		{Label: "支付宝公钥证书", Value: certTypeAlipay},
		{Label: "支付宝根证书", Value: certTypeAlipayRoot},
	}
}

func providerText(value string) string {
	switch value {
	case providerAlipay:
		return "支付宝"
	default:
		return value
	}
}

func environmentText(value string) string {
	switch value {
	case environmentSandbox:
		return "沙箱环境"
	case environmentProduction:
		return "正式环境"
	default:
		return value
	}
}

func commonStatusText(status int) string {
	if status == enum.CommonYes {
		return "启用"
	}
	if status == enum.CommonNo {
		return "禁用"
	}
	return "未知"
}

func methodText(methods []string) string {
	parts := make([]string, 0, len(methods))
	for _, method := range methods {
		label := enum.PaymentMethodLabels[method]
		if label == "" {
			label = method
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "、")
}

func currentPage(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func totalPage(total int64, size int) int {
	if total <= 0 || size <= 0 {
		return 0
	}
	return int((total + int64(size) - 1) / int64(size))
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}

func mustConfigJSON(methods []string) string {
	data, err := json.Marshal(methods)
	if err != nil {
		return fmt.Sprintf("[%q]", enum.PaymentMethodWeb)
	}
	return string(data)
}
