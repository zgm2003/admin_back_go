package aiknowledge

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
)

const timeLayout = "2006-01-02 15:04:05"

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{CommonStatusArr: dict.CommonStatusOptions(), AIKnowledgeVisibilityArr: dict.AIKnowledgeVisibilityOptions(), AIKnowledgeIndexStatusArr: dict.AIKnowledgeIndexStatusOptions(), AIKnowledgeSourceTypeArr: dict.AIKnowledgeSourceTypeOptions()}}, nil
}
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeListQuery(query)
	rows, total, err := repo.List(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(500, 500, "查询知识库失败", err)
	}
	list := make([]KnowledgeBaseItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, knowledgeBaseItem(row))
	}
	return &ListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}
func (s *Service) Detail(ctx context.Context, id int64) (*KnowledgeBaseItem, *apperror.Error) {
	kb, appErr := s.requireKB(ctx, id)
	if appErr != nil {
		return nil, appErr
	}
	item := knowledgeBaseItem(*kb)
	return &item, nil
}
func (s *Service) Create(ctx context.Context, ownerUserID int64, input KnowledgeBaseMutationInput) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeKBInput(ownerUserID, input, true, nil)
	if appErr != nil {
		return 0, appErr
	}
	id, err := repo.Create(ctx, row)
	if err != nil {
		return 0, apperror.Wrap(500, 500, "新增知识库失败", err)
	}
	return id, nil
}
func (s *Service) Update(ctx context.Context, id int64, input KnowledgeBaseMutationInput) *apperror.Error {
	kb, appErr := s.requireKB(ctx, id)
	if appErr != nil {
		return appErr
	}
	repo, _ := s.requireRepository()
	row, appErr := normalizeKBInput(kb.OwnerUserID, input, false, kb)
	if appErr != nil {
		return appErr
	}
	if err := repo.Update(ctx, id, kbFields(row)); err != nil {
		return apperror.Wrap(500, 500, "编辑知识库失败", err)
	}
	return nil
}
func (s *Service) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("无效的状态")
	}
	if _, appErr := s.requireKB(ctx, id); appErr != nil {
		return appErr
	}
	repo, _ := s.requireRepository()
	if err := repo.ChangeStatus(ctx, id, status); err != nil {
		return apperror.Wrap(500, 500, "切换知识库状态失败", err)
	}
	return nil
}
func (s *Service) Delete(ctx context.Context, ids []int64) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return 0, apperror.BadRequest("知识库ID不能为空")
	}
	count, err := repo.Delete(ctx, ids)
	if err != nil {
		return 0, apperror.Wrap(500, 500, "删除知识库失败", err)
	}
	return count, nil
}
func (s *Service) Documents(ctx context.Context, query DocumentListQuery) (*DocumentListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if _, appErr := s.requireKB(ctx, query.KnowledgeBaseID); appErr != nil {
		return nil, appErr
	}
	query = normalizeDocumentListQuery(query)
	rows, total, err := repo.ListDocuments(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(500, 500, "查询知识库文档失败", err)
	}
	list := make([]DocumentItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, documentItem(row, false))
	}
	return &DocumentListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}
func (s *Service) DocumentDetail(ctx context.Context, id int64, knowledgeBaseID int64) (*DocumentItem, *apperror.Error) {
	doc, appErr := s.requireDocument(ctx, id, knowledgeBaseID)
	if appErr != nil {
		return nil, appErr
	}
	item := documentItem(*doc, true)
	return &item, nil
}
func (s *Service) CreateDocument(ctx context.Context, ownerUserID int64, input DocumentMutationInput) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	kb, appErr := s.requireKB(ctx, input.KnowledgeBaseID)
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeDocumentInput(input, true)
	if appErr != nil {
		return 0, appErr
	}
	var id int64
	err := repo.WithTx(ctx, func(tx Repository) error {
		var err error
		id, err = tx.CreateDocument(ctx, row)
		if err != nil {
			return err
		}
		return reindexDocument(ctx, tx, kb, id, row.Content)
	})
	if err != nil {
		return 0, apperror.Wrap(500, 500, "新增知识库文档失败", err)
	}
	return id, nil
}
func (s *Service) UpdateDocument(ctx context.Context, id int64, input DocumentMutationInput) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	kb, appErr := s.requireKB(ctx, input.KnowledgeBaseID)
	if appErr != nil {
		return appErr
	}
	old, appErr := s.requireDocument(ctx, id, input.KnowledgeBaseID)
	if appErr != nil {
		return appErr
	}
	row, appErr := normalizeDocumentInput(input, false)
	if appErr != nil {
		return appErr
	}
	content := row.Content
	if strings.TrimSpace(content) == "" {
		content = old.Content
	}
	err := repo.WithTx(ctx, func(tx Repository) error {
		if err := tx.UpdateDocument(ctx, id, documentFields(row)); err != nil {
			return err
		}
		return reindexDocument(ctx, tx, kb, id, content)
	})
	if err != nil {
		return apperror.Wrap(500, 500, "编辑知识库文档失败", err)
	}
	return nil
}
func (s *Service) DeleteDocument(ctx context.Context, id int64, knowledgeBaseID int64) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	if _, appErr := s.requireDocument(ctx, id, knowledgeBaseID); appErr != nil {
		return 0, appErr
	}
	if err := repo.DeleteDocument(ctx, id, knowledgeBaseID); err != nil {
		return 0, apperror.Wrap(500, 500, "删除知识库文档失败", err)
	}
	return 1, nil
}
func (s *Service) ReindexDocument(ctx context.Context, id int64, knowledgeBaseID int64) (int, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	kb, appErr := s.requireKB(ctx, knowledgeBaseID)
	if appErr != nil {
		return 0, appErr
	}
	doc, appErr := s.requireDocument(ctx, id, knowledgeBaseID)
	if appErr != nil {
		return 0, appErr
	}
	var count int
	err := repo.WithTx(ctx, func(tx Repository) error {
		var err error
		count, err = reindexDocumentCount(ctx, tx, kb, id, doc.Content)
		return err
	})
	if err != nil {
		return 0, apperror.Wrap(500, 500, "重建知识库索引失败", err)
	}
	return count, nil
}
func (s *Service) Chunks(ctx context.Context, query ChunkListQuery) (*ChunkListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if _, appErr := s.requireKB(ctx, query.KnowledgeBaseID); appErr != nil {
		return nil, appErr
	}
	query = normalizeChunkListQuery(query)
	rows, total, err := repo.ListChunks(ctx, query)
	if err != nil {
		return nil, apperror.Wrap(500, 500, "查询知识库切片失败", err)
	}
	list := make([]ChunkItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, chunkItem(row))
	}
	return &ChunkListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}
func (s *Service) RetrievalTest(ctx context.Context, input RetrievalInput) (*RetrievalResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	kb, appErr := s.requireKB(ctx, input.KnowledgeBaseID)
	if appErr != nil {
		return nil, appErr
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, apperror.BadRequest("查询内容不能为空")
	}
	topK := input.TopK
	if topK <= 0 {
		topK = kb.TopK
	}
	if topK <= 0 {
		topK = 5
	}
	candidates, err := repo.CandidateChunks(ctx, kb.ID, QueryTerms(query), 300)
	if err != nil {
		return nil, apperror.Wrap(500, 500, "查询知识库候选切片失败", err)
	}
	chunks := RankChunks(candidates, query, topK, kb.ScoreThreshold)
	return &RetrievalResponse{Chunks: chunks, ContextPrompt: BuildContextPrompt(chunks)}, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.Internal("知识库仓储未配置")
	}
	return s.repository, nil
}
func (s *Service) requireKB(ctx context.Context, id int64) (*KnowledgeBase, *apperror.Error) {
	if id <= 0 {
		return nil, apperror.BadRequest("无效的知识库ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.Wrap(500, 500, "查询知识库失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("知识库不存在")
	}
	return row, nil
}
func (s *Service) requireDocument(ctx context.Context, id, kbID int64) (*Document, *apperror.Error) {
	if id <= 0 {
		return nil, apperror.BadRequest("无效的文档ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.GetDocument(ctx, id, kbID)
	if err != nil {
		return nil, apperror.Wrap(500, 500, "查询知识库文档失败", err)
	}
	if row == nil {
		return nil, apperror.NotFound("文档不存在")
	}
	return row, nil
}

func normalizeKBInput(owner int64, input KnowledgeBaseMutationInput, creating bool, current *KnowledgeBase) (KnowledgeBase, *apperror.Error) {
	name := strings.TrimSpace(input.Name)
	if name == "" && creating {
		return KnowledgeBase{}, apperror.BadRequest("知识库名称不能为空")
	}
	if len([]rune(name)) > 80 {
		return KnowledgeBase{}, apperror.BadRequest("知识库名称不能超过80个字符")
	}
	visibility := strings.TrimSpace(input.Visibility)
	if visibility == "" {
		visibility = enum.AIKnowledgeVisibilityPrivate
	}
	if current != nil && input.Visibility == "" {
		visibility = current.Visibility
	}
	if !enum.IsAIKnowledgeVisibility(visibility) {
		return KnowledgeBase{}, apperror.BadRequest("无效的知识库可见性")
	}
	chunkSize := input.ChunkSize
	if chunkSize == 0 {
		chunkSize = 800
	}
	if current != nil && input.ChunkSize == 0 {
		chunkSize = current.ChunkSize
	}
	overlap := input.ChunkOverlap
	if overlap == 0 {
		overlap = 120
	}
	if current != nil && input.ChunkOverlap == 0 {
		overlap = current.ChunkOverlap
	}
	if chunkSize < 100 || chunkSize > 4000 {
		return KnowledgeBase{}, apperror.BadRequest("切片长度必须在100到4000之间")
	}
	if overlap < 0 || overlap >= chunkSize {
		return KnowledgeBase{}, apperror.BadRequest("切片重叠必须小于切片长度")
	}
	topK := input.TopK
	if topK == 0 {
		topK = 5
	}
	if current != nil && input.TopK == 0 {
		topK = current.TopK
	}
	if topK < 1 || topK > 20 {
		return KnowledgeBase{}, apperror.BadRequest("召回数量必须在1到20之间")
	}
	status := input.Status
	if status == 0 {
		status = enum.CommonYes
	}
	if current != nil && input.Status == 0 {
		status = current.Status
	}
	if !enum.IsCommonStatus(status) {
		return KnowledgeBase{}, apperror.BadRequest("无效的状态")
	}
	perm := nullableJSON(input.PermissionJSON)
	return KnowledgeBase{Name: name, Description: input.Description, OwnerUserID: owner, Visibility: visibility, PermissionJSON: perm, ChunkSize: chunkSize, ChunkOverlap: overlap, TopK: topK, ScoreThreshold: input.ScoreThreshold, Status: status, IsDel: enum.CommonNo}, nil
}
func normalizeDocumentInput(input DocumentMutationInput, creating bool) (Document, *apperror.Error) {
	title := strings.TrimSpace(input.Title)
	if title == "" && creating {
		return Document{}, apperror.BadRequest("文档标题不能为空")
	}
	if len([]rune(title)) > 120 {
		return Document{}, apperror.BadRequest("文档标题不能超过120个字符")
	}
	source := strings.TrimSpace(input.SourceType)
	if source == "" {
		source = enum.AIKnowledgeSourceManual
	}
	if !enum.IsAIKnowledgeSourceType(source) {
		return Document{}, apperror.BadRequest("无效的文档来源类型")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" && creating {
		return Document{}, apperror.BadRequest("文档内容不能为空")
	}
	status := input.Status
	if status == 0 {
		status = enum.CommonYes
	}
	if !enum.IsCommonStatus(status) {
		return Document{}, apperror.BadRequest("无效的状态")
	}
	return Document{KnowledgeBaseID: input.KnowledgeBaseID, Title: title, SourceType: source, Content: content, ChunkCount: 0, IndexStatus: enum.AIKnowledgeIndexFailed, Status: status, IsDel: enum.CommonNo}, nil
}
func reindexDocument(ctx context.Context, repo Repository, kb *KnowledgeBase, id int64, content string) error {
	returnValue, err := reindexDocumentCount(ctx, repo, kb, id, content)
	_ = returnValue
	return err
}
func reindexDocumentCount(ctx context.Context, repo Repository, kb *KnowledgeBase, id int64, content string) (int, error) {
	chunks := ChunkText(content, kb.ChunkSize, kb.ChunkOverlap)
	count, err := repo.ReplaceDocumentChunks(ctx, kb.ID, id, chunks)
	if err != nil {
		return 0, err
	}
	status := enum.AIKnowledgeIndexFailed
	if count > 0 {
		status = enum.AIKnowledgeIndexIndexed
	}
	if err := repo.UpdateDocumentChunkStatus(ctx, id, count, status); err != nil {
		return 0, err
	}
	return count, nil
}
func normalizeListQuery(q ListQuery) ListQuery {
	if q.CurrentPage <= 0 {
		q.CurrentPage = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	if q.PageSize > enum.PageSizeMax {
		q.PageSize = enum.PageSizeMax
	}
	q.Name = strings.TrimSpace(q.Name)
	q.Visibility = strings.TrimSpace(q.Visibility)
	return q
}
func normalizeDocumentListQuery(q DocumentListQuery) DocumentListQuery {
	if q.CurrentPage <= 0 {
		q.CurrentPage = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	if q.PageSize > enum.PageSizeMax {
		q.PageSize = enum.PageSizeMax
	}
	q.Title = strings.TrimSpace(q.Title)
	return q
}
func normalizeChunkListQuery(q ChunkListQuery) ChunkListQuery {
	if q.CurrentPage <= 0 {
		q.CurrentPage = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	if q.PageSize > enum.PageSizeMax {
		q.PageSize = enum.PageSizeMax
	}
	return q
}
func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}
func nullableJSON(v JSONObject) *string {
	if len(v) == 0 {
		return nil
	}
	b, _ := json.Marshal(v)
	s := string(b)
	return &s
}
func decodeObject(raw *string) JSONObject {
	if raw == nil || *raw == "" {
		return JSONObject{}
	}
	var out JSONObject
	if json.Unmarshal([]byte(*raw), &out) != nil {
		return JSONObject{}
	}
	return out
}
func kbFields(row KnowledgeBase) map[string]any {
	return map[string]any{"name": row.Name, "description": row.Description, "visibility": row.Visibility, "permission_json": row.PermissionJSON, "chunk_size": row.ChunkSize, "chunk_overlap": row.ChunkOverlap, "top_k": row.TopK, "score_threshold": row.ScoreThreshold, "status": row.Status}
}
func documentFields(row Document) map[string]any {
	fields := map[string]any{"source_type": row.SourceType, "status": row.Status}
	if row.Title != "" {
		fields["title"] = row.Title
	}
	if row.Content != "" {
		fields["content"] = row.Content
	}
	return fields
}
func knowledgeBaseItem(row KnowledgeBase) KnowledgeBaseItem {
	return KnowledgeBaseItem{ID: row.ID, Name: row.Name, Description: row.Description, OwnerUserID: row.OwnerUserID, Visibility: row.Visibility, VisibilityName: enum.AIKnowledgeVisibilityLabels[row.Visibility], PermissionJSON: decodeObject(row.PermissionJSON), ChunkSize: row.ChunkSize, ChunkOverlap: row.ChunkOverlap, TopK: row.TopK, ScoreThreshold: row.ScoreThreshold, Status: row.Status, StatusName: statusName(row.Status), CreatedAt: row.CreatedAt.Format(timeLayout), UpdatedAt: row.UpdatedAt.Format(timeLayout)}
}
func documentItem(row Document, includeContent bool) DocumentItem {
	item := DocumentItem{ID: row.ID, KnowledgeBaseID: row.KnowledgeBaseID, Title: row.Title, SourceType: row.SourceType, SourceTypeName: enum.AIKnowledgeSourceTypeLabels[row.SourceType], ChunkCount: row.ChunkCount, IndexStatus: row.IndexStatus, IndexStatusName: enum.AIKnowledgeIndexStatusLabels[row.IndexStatus], Status: row.Status, StatusName: statusName(row.Status), CreatedAt: row.CreatedAt.Format(timeLayout), UpdatedAt: row.UpdatedAt.Format(timeLayout)}
	if includeContent {
		item.Content = row.Content
	}
	return item
}
func chunkItem(row Chunk) ChunkItem {
	return ChunkItem{ID: row.ID, KnowledgeBaseID: row.KnowledgeBaseID, DocumentID: row.DocumentID, ChunkNo: row.ChunkNo, Content: row.Content, TokenEstimate: row.TokenEstimate, MetadataJSON: decodeObject(row.MetadataJSON), Status: row.Status, CreatedAt: row.CreatedAt.Format(timeLayout)}
}
func statusName(v int) string {
	if v == enum.CommonYes {
		return "启用"
	}
	if v == enum.CommonNo {
		return "禁用"
	}
	return ""
}
func normalizeIDs(ids []int64) []int64 {
	seen := map[int64]struct{}{}
	out := []int64{}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
