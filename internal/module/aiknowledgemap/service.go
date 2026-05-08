package aiknowledgemap

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/platform/secretbox"
)

const (
	timeLayout = "2006-01-02 15:04:05"

	VisibilityPrivate = "private"
	VisibilityPublic  = "public"

	SourceTypeText = "text"
	SourceTypeFile = "file"

	IndexingPending = "pending"
	IndexingFailed  = "failed"
)

var visibilityLabels = map[string]string{
	VisibilityPrivate: "私有",
	VisibilityPublic:  "公开",
}

var sourceTypeLabels = map[string]string{
	SourceTypeText: "文本",
	SourceTypeFile: "文件",
}

var indexingStatusLabels = map[string]string{
	"pending":   "待同步",
	"indexing":  "索引中",
	"completed": "已完成",
	"failed":    "失败",
}

type Service struct {
	repository Repository
	secretbox  secretbox.Box
	factory    EngineFactory
}

func NewService(repository Repository, box secretbox.Box, factory EngineFactory) *Service {
	return &Service{repository: repository, secretbox: box, factory: factory}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	connections, err := repo.ListActiveConnections(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商选项失败", err)
	}
	options := make([]EngineOption, 0, len(connections))
	for _, row := range connections {
		options = append(options, EngineOption{Label: row.Name, Value: row.ID, EngineType: row.EngineType})
	}
	return &InitResponse{Dict: InitDict{
		VisibilityArr:           stringOptions([]string{VisibilityPrivate, VisibilityPublic}, visibilityLabels),
		SourceTypeArr:           stringOptions([]string{SourceTypeText, SourceTypeFile}, sourceTypeLabels),
		IndexingStatusArr:       stringOptions([]string{"pending", "indexing", "completed", "failed"}, indexingStatusLabels),
		CommonStatusArr:         dict.CommonStatusOptions(),
		EngineConnectionOptions: options,
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
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI知识库失败", err)
	}
	list := make([]MapDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, mapDTO(row))
	}
	return &ListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) Detail(ctx context.Context, id uint64) (*DetailResponse, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("无效的AI知识库ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI知识库失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI知识库不存在")
	}
	return &DetailResponse{MapDTO: mapDTO(*row)}, nil
}

func (s *Service) Create(ctx context.Context, input MapInput) (uint64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeCreateInput(input)
	if appErr != nil {
		return 0, appErr
	}
	if appErr := s.ensureActiveConnection(ctx, repo, row.EngineConnectionID); appErr != nil {
		return 0, appErr
	}
	exists, err := repo.ExistsByCode(ctx, row.Code, 0)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "校验AI知识库编码失败", err)
	}
	if exists {
		return 0, apperror.BadRequest("AI知识库编码已存在")
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI知识库失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id uint64, input MapInput) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI知识库ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if appErr := ensureMapExists(ctx, repo, id); appErr != nil {
		return appErr
	}
	fields, appErr := normalizeUpdateFields(input)
	if appErr != nil {
		return appErr
	}
	if appErr := s.ensureActiveConnection(ctx, repo, input.EngineConnectionID); appErr != nil {
		return appErr
	}
	exists, err := repo.ExistsByCode(ctx, strings.TrimSpace(input.Code), id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "校验AI知识库编码失败", err)
	}
	if exists {
		return apperror.BadRequest("AI知识库编码已存在")
	}
	if err := repo.Update(ctx, id, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "编辑AI知识库失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI知识库ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if appErr := ensureMapExists(ctx, repo, id); appErr != nil {
		return appErr
	}
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换AI知识库状态失败", err)
	}
	return nil
}

func (s *Service) Sync(ctx context.Context, id uint64) *apperror.Error {
	repo, row, appErr := s.activeMap(ctx, id)
	if appErr != nil {
		return appErr
	}
	engine, appErr := s.engineForMap(ctx, repo, *row)
	if appErr != nil {
		return appErr
	}
	docs, err := repo.ListSyncableDocuments(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI知识库文档失败", err)
	}
	for _, doc := range docs {
		if appErr := s.syncDocument(ctx, repo, engine, *row, doc); appErr != nil {
			return appErr
		}
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id uint64) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI知识库ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if appErr := ensureMapExists(ctx, repo, id); appErr != nil {
		return appErr
	}
	if err := repo.Delete(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI知识库失败", err)
	}
	return nil
}

func (s *Service) Documents(ctx context.Context, mapID uint64) (*DocumentListResponse, *apperror.Error) {
	if mapID == 0 {
		return nil, apperror.BadRequest("无效的AI知识库ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if appErr := ensureMapExists(ctx, repo, mapID); appErr != nil {
		return nil, appErr
	}
	rows, err := repo.ListDocuments(ctx, mapID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI知识库文档失败", err)
	}
	list := make([]DocumentDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, documentDTO(row))
	}
	return &DocumentListResponse{List: list}, nil
}

func (s *Service) CreateDocument(ctx context.Context, mapID uint64, input DocumentInput) (uint64, *apperror.Error) {
	repo, row, appErr := s.activeMap(ctx, mapID)
	if appErr != nil {
		return 0, appErr
	}
	doc, appErr := normalizeDocumentInput(mapID, input)
	if appErr != nil {
		return 0, appErr
	}
	if doc.SourceType == SourceTypeFile {
		doc.ErrorMessage = "file source_type is not supported in this slice"
		if id, err := repo.CreateDocument(ctx, doc); err != nil {
			return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI知识库文档失败", err)
		} else {
			return id, apperror.BadRequest("文件知识库导入暂未支持")
		}
	}
	id, err := repo.CreateDocument(ctx, doc)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, 500, "新增AI知识库文档失败", err)
	}
	doc.ID = id
	engine, appErr := s.engineForMap(ctx, repo, *row)
	if appErr != nil {
		_ = repo.UpdateDocument(ctx, id, map[string]any{"indexing_status": IndexingFailed, "error_message": appErr.Message})
		return 0, appErr
	}
	if appErr := s.syncDocument(ctx, repo, engine, *row, doc); appErr != nil {
		return 0, appErr
	}
	return id, nil
}

func (s *Service) ChangeDocumentStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI知识库文档ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.ChangeDocumentStatus(ctx, id, status); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "切换AI知识库文档状态失败", err)
	}
	return nil
}

func (s *Service) RefreshDocumentStatus(ctx context.Context, id uint64) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI知识库文档ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	doc, err := repo.GetDocument(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI知识库文档失败", err)
	}
	if doc == nil {
		return apperror.NotFound("AI知识库文档不存在")
	}
	if strings.TrimSpace(doc.EngineDocumentID) == "" {
		return apperror.BadRequest("AI知识库文档尚未同步")
	}
	engine, appErr := s.engineFromJoinedDocument(ctx, *doc)
	if appErr != nil {
		return appErr
	}
	result, err := engine.KnowledgeStatus(ctx, platformai.KnowledgeStatusInput{DatasetID: doc.EngineDatasetID, DocumentID: doc.EngineDocumentID})
	if err != nil {
		msg := err.Error()
		_ = repo.UpdateDocument(ctx, id, map[string]any{"indexing_status": IndexingFailed, "error_message": msg})
		return apperror.Wrap(apperror.CodeInternal, 500, "刷新AI知识库文档状态失败", err)
	}
	fields := map[string]any{"indexing_status": nonEmpty(result.IndexingStatus, doc.IndexingStatus), "error_message": result.ErrorMessage}
	if err := repo.UpdateDocument(ctx, id, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "保存AI知识库文档状态失败", err)
	}
	return nil
}

func (s *Service) DeleteDocument(ctx context.Context, id uint64) *apperror.Error {
	if id == 0 {
		return apperror.BadRequest("无效的AI知识库文档ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.DeleteDocument(ctx, id); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "删除AI知识库文档失败", err)
	}
	return nil
}

func (s *Service) syncDocument(ctx context.Context, repo Repository, engine platformai.Engine, row KnowledgeMap, doc Document) *apperror.Error {
	result, err := engine.SyncKnowledge(ctx, platformai.KnowledgeSyncInput{
		DatasetID: row.EngineDatasetID,
		Document:  platformai.KnowledgeDocument{Name: doc.Name, Text: doc.Content, SourceRef: doc.SourceRef},
	})
	if err != nil {
		msg := err.Error()
		_ = repo.UpdateDocument(ctx, doc.ID, map[string]any{"indexing_status": IndexingFailed, "error_message": msg})
		return apperror.Wrap(apperror.CodeInternal, 500, "同步AI知识库文档失败", err)
	}
	fields := map[string]any{
		"engine_document_id": result.EngineDocumentID,
		"engine_batch":       result.EngineBatch,
		"indexing_status":    nonEmpty(result.IndexingStatus, IndexingPending),
		"error_message":      "",
	}
	if strings.TrimSpace(row.EngineDatasetID) == "" && strings.TrimSpace(result.EngineDatasetID) != "" {
		if err := repo.Update(ctx, row.ID, map[string]any{"engine_dataset_id": result.EngineDatasetID}); err != nil {
			return apperror.Wrap(apperror.CodeInternal, 500, "保存AI知识库引擎ID失败", err)
		}
	}
	if err := repo.UpdateDocument(ctx, doc.ID, fields); err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "保存AI知识库文档同步状态失败", err)
	}
	return nil
}

func (s *Service) activeMap(ctx context.Context, id uint64) (Repository, *KnowledgeMap, *apperror.Error) {
	if id == 0 {
		return nil, nil, apperror.BadRequest("无效的AI知识库ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, nil, appErr
	}
	row, err := repo.GetRaw(ctx, id)
	if err != nil {
		return nil, nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI知识库失败", err)
	}
	if row == nil {
		return nil, nil, apperror.NotFound("AI知识库不存在")
	}
	if row.Status != enum.CommonYes {
		return nil, nil, apperror.BadRequest("AI知识库已禁用")
	}
	return repo, row, nil
}

func (s *Service) engineForMap(ctx context.Context, repo Repository, row KnowledgeMap) (platformai.Engine, *apperror.Error) {
	connection, err := repo.GetActiveConnection(ctx, row.EngineConnectionID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if connection == nil {
		return nil, apperror.BadRequest("AI供应商不存在或已禁用")
	}
	return s.engineFromConnection(ctx, *connection)
}

func (s *Service) engineFromJoinedDocument(ctx context.Context, doc DocumentWithMap) (platformai.Engine, *apperror.Error) {
	return s.engineFromConnection(ctx, EngineConnection{EngineType: doc.EngineType, BaseURL: doc.EngineBaseURL, APIKeyEnc: doc.EngineAPIKeyEnc, Status: enum.CommonYes, IsDel: enum.CommonNo})
}

func (s *Service) engineFromConnection(ctx context.Context, connection EngineConnection) (platformai.Engine, *apperror.Error) {
	apiKey, err := s.secretbox.Decrypt(connection.APIKeyEnc)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "解密AI供应商API Key失败", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, apperror.BadRequest("AI供应商API Key未配置")
	}
	factory := s.factory
	if factory == nil {
		factory = unsupportedFactory{}
	}
	engine, err := factory.NewEngine(ctx, EngineConfig{EngineType: platformai.EngineType(connection.EngineType), BaseURL: connection.BaseURL, APIKey: apiKey})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, 500, "创建AI引擎失败", err)
	}
	return engine, nil
}

func (s *Service) ensureActiveConnection(ctx context.Context, repo Repository, id uint64) *apperror.Error {
	connection, err := repo.GetActiveConnection(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI供应商失败", err)
	}
	if connection == nil {
		return apperror.BadRequest("AI供应商不存在或已禁用")
	}
	return nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("AI知识库仓储未配置")
	}
	return s.repository, nil
}

func ensureMapExists(ctx context.Context, repo Repository, id uint64) *apperror.Error {
	row, err := repo.GetRaw(ctx, id)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, 500, "查询AI知识库失败", err)
	}
	if row == nil {
		return apperror.NotFound("AI知识库不存在")
	}
	return nil
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
	query.Code = strings.TrimSpace(query.Code)
	query.Visibility = strings.TrimSpace(query.Visibility)
	return query
}

func normalizeCreateInput(input MapInput) (KnowledgeMap, *apperror.Error) {
	fields, appErr := normalizeMapFields(input)
	if appErr != nil {
		return KnowledgeMap{}, appErr
	}
	return KnowledgeMap{EngineConnectionID: fields.engineConnectionID, Name: fields.name, Code: fields.code, EngineDatasetID: fields.engineDatasetID, Visibility: fields.visibility, MetaJSON: fields.metaJSON, Status: fields.status, IsDel: enum.CommonNo}, nil
}

func normalizeUpdateFields(input MapInput) (map[string]any, *apperror.Error) {
	fields, appErr := normalizeMapFields(input)
	if appErr != nil {
		return nil, appErr
	}
	return map[string]any{"engine_connection_id": fields.engineConnectionID, "name": fields.name, "code": fields.code, "engine_dataset_id": fields.engineDatasetID, "visibility": fields.visibility, "meta_json": fields.metaJSON, "status": fields.status}, nil
}

type normalizedMapFields struct {
	engineConnectionID uint64
	name               string
	code               string
	engineDatasetID    string
	visibility         string
	metaJSON           string
	status             int
}

func normalizeMapFields(input MapInput) (normalizedMapFields, *apperror.Error) {
	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	visibility := strings.TrimSpace(input.Visibility)
	if input.EngineConnectionID == 0 {
		return normalizedMapFields{}, apperror.BadRequest("AI供应商不能为空")
	}
	if name == "" {
		return normalizedMapFields{}, apperror.BadRequest("AI知识库名称不能为空")
	}
	if len([]rune(name)) > 128 {
		return normalizedMapFields{}, apperror.BadRequest("AI知识库名称不能超过128个字符")
	}
	if code == "" {
		return normalizedMapFields{}, apperror.BadRequest("AI知识库编码不能为空")
	}
	if len([]rune(code)) > 128 {
		return normalizedMapFields{}, apperror.BadRequest("AI知识库编码不能超过128个字符")
	}
	if visibility == "" {
		visibility = VisibilityPrivate
	}
	if !isVisibility(visibility) {
		return normalizedMapFields{}, apperror.BadRequest("无效的知识库可见性")
	}
	status := input.Status
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return normalizedMapFields{}, apperror.BadRequest("无效的状态")
	}
	metaJSON, appErr := normalizeRawJSON(input.MetaJSON)
	if appErr != nil {
		return normalizedMapFields{}, appErr
	}
	return normalizedMapFields{engineConnectionID: input.EngineConnectionID, name: name, code: code, engineDatasetID: strings.TrimSpace(input.EngineDatasetID), visibility: visibility, metaJSON: metaJSON, status: status}, nil
}

func normalizeDocumentInput(mapID uint64, input DocumentInput) (Document, *apperror.Error) {
	name := strings.TrimSpace(input.Name)
	sourceType := strings.TrimSpace(input.SourceType)
	if name == "" {
		return Document{}, apperror.BadRequest("AI知识库文档名称不能为空")
	}
	if len([]rune(name)) > 255 {
		return Document{}, apperror.BadRequest("AI知识库文档名称不能超过255个字符")
	}
	if !isSourceType(sourceType) {
		return Document{}, apperror.BadRequest("无效的文档来源类型")
	}
	content := strings.TrimSpace(input.Content)
	sourceRef := strings.TrimSpace(input.SourceRef)
	if sourceType == SourceTypeText && content == "" {
		return Document{}, apperror.BadRequest("文本内容不能为空")
	}
	if sourceType == SourceTypeFile && sourceRef == "" {
		return Document{}, apperror.BadRequest("文件来源不能为空")
	}
	status := input.Status
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return Document{}, apperror.BadRequest("无效的状态")
	}
	metaJSON, appErr := normalizeRawJSON(input.MetaJSON)
	if appErr != nil {
		return Document{}, appErr
	}
	return Document{KnowledgeMapID: mapID, Name: name, SourceType: sourceType, SourceRef: sourceRef, Content: content, IndexingStatus: IndexingPending, Status: status, IsDel: enum.CommonNo, MetaJSON: metaJSON}, nil
}

func mapDTO(row MapWithEngine) MapDTO {
	return MapDTO{ID: row.ID, EngineConnectionID: row.EngineConnectionID, EngineConnectionName: row.EngineConnectionName, EngineType: row.EngineType, Name: row.Name, Code: row.Code, EngineDatasetID: row.EngineDatasetID, Visibility: row.Visibility, VisibilityName: visibilityLabels[row.Visibility], MetaJSON: rawJSON(row.MetaJSON), Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func documentDTO(row Document) DocumentDTO {
	return DocumentDTO{ID: row.ID, KnowledgeMapID: row.KnowledgeMapID, Name: row.Name, EngineDocumentID: row.EngineDocumentID, EngineBatch: row.EngineBatch, SourceType: row.SourceType, SourceTypeName: sourceTypeLabels[row.SourceType], SourceRef: row.SourceRef, IndexingStatus: row.IndexingStatus, ErrorMessage: row.ErrorMessage, MetaJSON: rawJSON(row.MetaJSON), Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func normalizeRawJSON(value json.RawMessage) (string, *apperror.Error) {
	if len(value) == 0 {
		return "{}", nil
	}
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || trimmed == "null" {
		return "{}", nil
	}
	if !json.Valid([]byte(trimmed)) {
		return "", apperror.BadRequest("扩展配置不是合法JSON")
	}
	return trimmed, nil
}

func rawJSON(value string) json.RawMessage {
	value = strings.TrimSpace(value)
	if value == "" || !json.Valid([]byte(value)) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(value)
}

func stringOptions(values []string, labels map[string]string) []dict.Option[string] {
	options := make([]dict.Option[string], 0, len(values))
	for _, value := range values {
		options = append(options, dict.Option[string]{Label: labels[value], Value: value})
	}
	return options
}

func isVisibility(value string) bool { _, ok := visibilityLabels[value]; return ok }
func isSourceType(value string) bool { _, ok := sourceTypeLabels[value]; return ok }

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

func nonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

type unsupportedFactory struct{}

func (unsupportedFactory) NewEngine(ctx context.Context, input EngineConfig) (platformai.Engine, error) {
	return nil, fmt.Errorf("ai knowledge engine factory not configured")
}
