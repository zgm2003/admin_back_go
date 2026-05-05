package paychannel

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/secretbox"
)

const timeLayout = "2006-01-02 15:04:05"

type Service struct {
	repository Repository
	secretbox  secretbox.Box
}

func NewService(repository Repository, box secretbox.Box) *Service {
	return &Service{repository: repository, secretbox: box}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		ChannelArr:      dict.PayChannelOptions(),
		CommonStatusArr: dict.CommonStatusOptions(),
		PayMethodArr:    dict.PayMethodOptions(),
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
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询支付渠道失败", err)
	}
	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, channelItem(row))
	}
	return &ListResponse{
		List: list,
		Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total},
	}, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeCreateInput(input)
	if appErr != nil {
		return 0, appErr
	}
	exists, err := repo.ExistsUnique(ctx, row.Channel, row.MchID, row.AppID, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验支付渠道失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("该渠道+商户号+应用ID 组合已存在")
	}
	if strings.TrimSpace(input.AppPrivateKey) != "" {
		ciphertext, err := s.secretbox.Encrypt(strings.TrimSpace(input.AppPrivateKey))
		if err != nil {
			return 0, apperror.Wrap(apperror.CodeInternal, 500, "加密支付渠道私钥失败", err)
		}
		row.AppPrivateKeyEnc = ciphertext
		row.AppPrivateKeyHint = secretbox.Hint(strings.TrimSpace(input.AppPrivateKey))
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增支付渠道失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的支付渠道ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询支付渠道失败", err)
	}
	if row == nil {
		return apperror.NotFound("支付渠道不存在")
	}
	fields, appErr := normalizeUpdateFields(input)
	if appErr != nil {
		return appErr
	}
	exists, err := repo.ExistsUnique(ctx, input.Channel, strings.TrimSpace(input.MchID), strings.TrimSpace(input.AppID), id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验支付渠道失败", err)
	}
	if exists {
		return apperror.BadRequest("该渠道+商户号+应用ID 组合已存在")
	}
	if input.Channel != row.Channel && len(input.SupportedMethods) == 0 {
		return apperror.BadRequest("请至少选择一种支付方式")
	}
	if strings.TrimSpace(input.AppPrivateKey) != "" {
		ciphertext, err := s.secretbox.Encrypt(strings.TrimSpace(input.AppPrivateKey))
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "加密支付渠道私钥失败", err)
		}
		fields["app_private_key_enc"] = ciphertext
		fields["app_private_key_hint"] = secretbox.Hint(strings.TrimSpace(input.AppPrivateKey))
	}
	if err := repo.Update(ctx, id, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑支付渠道失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的支付渠道ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询支付渠道失败", err)
	}
	if row == nil {
		return apperror.NotFound("支付渠道不存在")
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换支付渠道状态失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id int64) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequest("无效的支付渠道ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询支付渠道失败", err)
	}
	if row == nil {
		return apperror.NotFound("支付渠道不存在")
	}
	referenced, err := repo.Referenced(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验支付渠道引用失败", err)
	}
	if referenced {
		return apperror.BadRequest("支付渠道已有订单或支付流水引用，请禁用而不是删除")
	}
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除支付渠道失败", err)
	}
	return nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("支付渠道仓储未配置")
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
	query.Name = strings.TrimSpace(query.Name)
	return query
}

func normalizeCreateInput(input CreateInput) (Channel, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Name, input.Channel, input.SupportedMethods, input.MchID, input.AppID, input.NotifyURL, input.PublicCertPath, input.PlatformCertPath, input.RootCertPath, input.Sort, input.IsSandbox, input.Status, input.Remark)
	if appErr != nil {
		return Channel{}, appErr
	}
	return Channel{
		Name:             fields.name,
		Channel:          fields.channel,
		MchID:            fields.mchID,
		AppID:            fields.appID,
		NotifyURL:        fields.notifyURL,
		PublicCertPath:   fields.publicCertPath,
		PlatformCertPath: fields.platformCertPath,
		RootCertPath:     fields.rootCertPath,
		ExtraConfig:      fields.extraConfig,
		IsSandbox:        fields.isSandbox,
		Sort:             fields.sort,
		Remark:           fields.remark,
		Status:           fields.status,
		IsDel:            enum.CommonNo,
	}, nil
}

func normalizeUpdateFields(input UpdateInput) (map[string]any, *apperror.Error) {
	fields, appErr := normalizeMutationFields(input.Name, input.Channel, input.SupportedMethods, input.MchID, input.AppID, input.NotifyURL, input.PublicCertPath, input.PlatformCertPath, input.RootCertPath, input.Sort, input.IsSandbox, input.Status, input.Remark)
	if appErr != nil {
		return nil, appErr
	}
	return map[string]any{
		"name":               fields.name,
		"channel":            fields.channel,
		"mch_id":             fields.mchID,
		"app_id":             fields.appID,
		"notify_url":         fields.notifyURL,
		"public_cert_path":   fields.publicCertPath,
		"platform_cert_path": fields.platformCertPath,
		"root_cert_path":     fields.rootCertPath,
		"extra_config":       fields.extraConfig,
		"is_sandbox":         fields.isSandbox,
		"sort":               fields.sort,
		"status":             fields.status,
		"remark":             fields.remark,
	}, nil
}

type normalizedFields struct {
	name             string
	channel          int
	supportedMethods []string
	mchID            string
	appID            string
	notifyURL        string
	publicCertPath   string
	platformCertPath string
	rootCertPath     string
	extraConfig      string
	sort             int
	isSandbox        int
	status           int
	remark           string
}

func normalizeMutationFields(name string, channel int, methods []string, mchID string, appID string, notifyURL string, publicCertPath string, platformCertPath string, rootCertPath string, sortValue int, isSandbox int, status int, remark string) (normalizedFields, *apperror.Error) {
	name = strings.TrimSpace(name)
	mchID = strings.TrimSpace(mchID)
	appID = strings.TrimSpace(appID)
	notifyURL = strings.TrimSpace(notifyURL)
	publicCertPath = strings.TrimSpace(publicCertPath)
	platformCertPath = strings.TrimSpace(platformCertPath)
	rootCertPath = strings.TrimSpace(rootCertPath)
	remark = strings.TrimSpace(remark)

	if name == "" {
		return normalizedFields{}, apperror.BadRequest("渠道名称不能为空")
	}
	if len([]rune(name)) > 50 {
		return normalizedFields{}, apperror.BadRequest("渠道名称不能超过50个字符")
	}
	if !enum.IsPayChannel(channel) {
		return normalizedFields{}, apperror.BadRequest("无效的支付渠道")
	}
	if mchID == "" {
		return normalizedFields{}, apperror.BadRequest("商户号不能为空")
	}
	if len([]rune(mchID)) > 64 {
		return normalizedFields{}, apperror.BadRequest("商户号不能超过64个字符")
	}
	if len([]rune(appID)) > 64 {
		return normalizedFields{}, apperror.BadRequest("应用ID不能超过64个字符")
	}
	if len([]rune(notifyURL)) > 512 || len([]rune(publicCertPath)) > 512 || len([]rune(platformCertPath)) > 512 || len([]rune(rootCertPath)) > 512 {
		return normalizedFields{}, apperror.BadRequest("支付渠道路径字段过长")
	}
	if sortValue < 0 || sortValue > 9999 {
		return normalizedFields{}, apperror.BadRequest("排序必须在 0..9999 之间")
	}
	if isSandbox == 0 {
		isSandbox = enum.CommonNo
	}
	if !enum.IsCommonYesNo(isSandbox) {
		return normalizedFields{}, apperror.BadRequest("无效的沙箱状态")
	}
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return normalizedFields{}, apperror.BadRequest("无效的状态")
	}
	if len([]rune(remark)) > 255 {
		return normalizedFields{}, apperror.BadRequest("备注不能超过255个字符")
	}

	submittedCount := uniqueNonEmptyCount(methods)
	normalizedMethods := enum.NormalizePaySupportedMethods(channel, methods)
	if len(normalizedMethods) == 0 {
		if submittedCount > 0 {
			return normalizedFields{}, apperror.BadRequest("所选支付方式包含当前渠道不支持的选项")
		}
		return normalizedFields{}, apperror.BadRequest("请至少选择一种支付方式")
	}
	if !enum.PaySupportedMethodsValid(channel, methods) {
		return normalizedFields{}, apperror.BadRequest("所选支付方式包含当前渠道不支持的选项")
	}
	extraConfig, err := encodeExtraConfig(normalizedMethods)
	if err != nil {
		return normalizedFields{}, apperror.BadRequest("支付方式配置错误")
	}
	return normalizedFields{
		name: name, channel: channel, supportedMethods: normalizedMethods, mchID: mchID, appID: appID, notifyURL: notifyURL,
		publicCertPath: publicCertPath, platformCertPath: platformCertPath, rootCertPath: rootCertPath,
		extraConfig: extraConfig, sort: sortValue, isSandbox: isSandbox, status: status, remark: remark,
	}, nil
}

func uniqueNonEmptyCount(values []string) int {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}
	return len(seen)
}

func encodeExtraConfig(methods []string) (string, error) {
	payload := struct {
		SupportedMethods []string `json:"supported_methods"`
	}{SupportedMethods: methods}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeSupportedMethods(row Channel) []string {
	var parsed struct {
		SupportedMethods []string `json:"supported_methods"`
	}
	if strings.TrimSpace(row.ExtraConfig) == "" {
		return enum.PayDefaultSupportedMethods(row.Channel)
	}
	if err := json.Unmarshal([]byte(row.ExtraConfig), &parsed); err != nil {
		return enum.PayDefaultSupportedMethods(row.Channel)
	}
	methods := enum.NormalizePaySupportedMethods(row.Channel, parsed.SupportedMethods)
	if len(methods) == 0 {
		return enum.PayDefaultSupportedMethods(row.Channel)
	}
	return methods
}

func channelItem(row Channel) ListItem {
	methods := decodeSupportedMethods(row)
	return ListItem{
		ID: row.ID, Name: row.Name, Channel: row.Channel, ChannelName: enum.PayChannelLabels[row.Channel],
		SupportedMethods: methods, SupportedMethodsText: methodText(methods), MchID: row.MchID, AppID: row.AppID,
		NotifyURL: row.NotifyURL, AppPrivateKeyHint: row.AppPrivateKeyHint, PublicCertPath: row.PublicCertPath,
		PlatformCertPath: row.PlatformCertPath, RootCertPath: row.RootCertPath, Sort: row.Sort, IsSandbox: row.IsSandbox,
		IsSandboxText: yesNoText(row.IsSandbox), Status: row.Status, StatusName: statusText(row.Status), Remark: row.Remark,
		CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt),
	}
}

func methodText(methods []string) string {
	labels := make([]string, 0, len(methods))
	for _, method := range methods {
		label := enum.PayMethodLabels[method]
		if label == "" {
			label = method
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, " / ")
}

func yesNoText(value int) string {
	if value == enum.CommonYes {
		return "是"
	}
	if value == enum.CommonNo {
		return "否"
	}
	return ""
}

func statusText(value int) string {
	for _, item := range dict.CommonStatusOptions() {
		if item.Value == value {
			return item.Label
		}
	}
	return ""
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}
