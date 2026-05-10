package aiknowledge

import (
	"context"
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
)

const (
	SourceTypeText     = "text"
	SourceTypeMarkdown = "markdown"
	SourceTypeFile     = "file"

	IndexStatusPending = "pending"
	IndexStatusIndexed = "indexed"
	IndexStatusFailed  = "failed"

	maxDocumentContentBytes = 2 * 1024 * 1024
)

var codePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

var sourceTypeLabels = map[string]string{
	SourceTypeText:     "文本",
	SourceTypeMarkdown: "Markdown",
	SourceTypeFile:     "文件",
}

var indexStatusLabels = map[string]string{
	IndexStatusPending: "待索引",
	IndexStatusIndexed: "已索引",
	IndexStatusFailed:  "索引失败",
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{
		CommonStatusArr: dict.CommonStatusOptions(),
		SourceTypeArr:   stringOptions([]string{SourceTypeText, SourceTypeMarkdown, SourceTypeFile}, sourceTypeLabels),
		IndexStatusArr:  stringOptions([]string{IndexStatusPending, IndexStatusIndexed, IndexStatusFailed}, indexStatusLabels),
	}}, nil
}

func (s *Service) ListBases(ctx context.Context, query BaseListQuery) (*BaseListResponse, *apperror.Error) {
	repo, appErr := s.requireRepo()
	if appErr != nil {
		return nil, appErr
	}
	query = normalizeBaseListQuery(query)
	rows, total, err := repo.ListBases(ctx, query)
	if err != nil {
		return nil, wrapInternal(err)
	}
	list := make([]BaseDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, baseDTO(row))
	}
	return &BaseListResponse{List: list, Page: buildPage(query.CurrentPage, query.PageSize, total)}, nil
}

func (s *Service) GetBase(ctx context.Context, id uint64) (*BaseDetailResponse, *apperror.Error) {
	row, appErr := s.getBase(ctx, id)
	if appErr != nil {
		return nil, appErr
	}
	return &BaseDetailResponse{BaseDTO: baseDTO(*row)}, nil
}

func (s *Service) CreateBase(ctx context.Context, input BaseMutationInput) (uint64, *apperror.Error) {
	repo, appErr := s.requireRepo()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := normalizeBaseInput(input)
	if appErr != nil {
		return 0, appErr
	}
	id, err := repo.CreateBase(ctx, row)
	if err != nil {
		return 0, wrapInternal(err)
	}
	return id, nil
}

func (s *Service) UpdateBase(ctx context.Context, id uint64, input BaseMutationInput) *apperror.Error {
	if _, appErr := s.getBase(ctx, id); appErr != nil {
		return appErr
	}
	row, appErr := normalizeBaseInput(input)
	if appErr != nil {
		return appErr
	}
	repo, _ := s.requireRepo()
	if err := repo.UpdateBase(ctx, id, map[string]any{
		"name":                      row.Name,
		"code":                      row.Code,
		"description":               row.Description,
		"chunk_size_chars":          row.ChunkSizeChars,
		"chunk_overlap_chars":       row.ChunkOverlapChars,
		"default_top_k":             row.DefaultTopK,
		"default_min_score":         row.DefaultMinScore,
		"default_max_context_chars": row.DefaultMaxContextChars,
		"status":                    row.Status,
	}); err != nil {
		return wrapInternal(err)
	}
	return nil
}

func (s *Service) ChangeBaseStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("AI知识库状态错误")
	}
	if _, appErr := s.getBase(ctx, id); appErr != nil {
		return appErr
	}
	repo, _ := s.requireRepo()
	if err := repo.ChangeBaseStatus(ctx, id, status); err != nil {
		return wrapInternal(err)
	}
	return nil
}

func (s *Service) DeleteBase(ctx context.Context, id uint64) *apperror.Error {
	if _, appErr := s.getBase(ctx, id); appErr != nil {
		return appErr
	}
	repo, _ := s.requireRepo()
	if err := repo.DeleteBase(ctx, id); err != nil {
		return wrapInternal(err)
	}
	return nil
}

func (s *Service) ListDocuments(ctx context.Context, baseID uint64, query DocumentListQuery) (*DocumentListResponse, *apperror.Error) {
	if _, appErr := s.getBase(ctx, baseID); appErr != nil {
		return nil, appErr
	}
	repo, _ := s.requireRepo()
	query = normalizeDocumentListQuery(query)
	rows, total, err := repo.ListDocuments(ctx, baseID, query)
	if err != nil {
		return nil, wrapInternal(err)
	}
	list := make([]DocumentDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, documentDTO(row))
	}
	return &DocumentListResponse{List: list, Page: buildPage(query.CurrentPage, query.PageSize, total)}, nil
}

func (s *Service) GetDocument(ctx context.Context, id uint64) (*DocumentDetailResponse, *apperror.Error) {
	row, appErr := s.getDocument(ctx, id)
	if appErr != nil {
		return nil, appErr
	}
	return &DocumentDetailResponse{DocumentDTO: documentDTO(*row), Content: row.Content}, nil
}

func (s *Service) CreateDocument(ctx context.Context, baseID uint64, input DocumentMutationInput) (uint64, *apperror.Error) {
	if _, appErr := s.getBase(ctx, baseID); appErr != nil {
		return 0, appErr
	}
	repo, _ := s.requireRepo()
	row, appErr := normalizeDocumentInput(baseID, input)
	if appErr != nil {
		return 0, appErr
	}
	id, err := repo.CreateDocument(ctx, row)
	if err != nil {
		return 0, wrapInternal(err)
	}
	return id, nil
}

func (s *Service) UpdateDocument(ctx context.Context, id uint64, input DocumentMutationInput) *apperror.Error {
	row, appErr := s.getDocument(ctx, id)
	if appErr != nil {
		return appErr
	}
	normalized, appErr := normalizeDocumentInput(row.KnowledgeBaseID, input)
	if appErr != nil {
		return appErr
	}
	repo, _ := s.requireRepo()
	if err := repo.UpdateDocument(ctx, id, map[string]any{
		"title":         normalized.Title,
		"source_type":   normalized.SourceType,
		"source_ref":    normalized.SourceRef,
		"content":       normalized.Content,
		"index_status":  IndexStatusPending,
		"error_message": "",
		"status":        normalized.Status,
	}); err != nil {
		return wrapInternal(err)
	}
	return nil
}

func (s *Service) ChangeDocumentStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("AI知识库文档状态错误")
	}
	if _, appErr := s.getDocument(ctx, id); appErr != nil {
		return appErr
	}
	repo, _ := s.requireRepo()
	if err := repo.ChangeDocumentStatus(ctx, id, status); err != nil {
		return wrapInternal(err)
	}
	return nil
}

func (s *Service) DeleteDocument(ctx context.Context, id uint64) *apperror.Error {
	if _, appErr := s.getDocument(ctx, id); appErr != nil {
		return appErr
	}
	repo, _ := s.requireRepo()
	if err := repo.DeleteDocument(ctx, id); err != nil {
		return wrapInternal(err)
	}
	return nil
}

func (s *Service) ReindexDocument(ctx context.Context, id uint64) *apperror.Error {
	doc, appErr := s.getDocument(ctx, id)
	if appErr != nil {
		return appErr
	}
	base, appErr := s.getBase(ctx, doc.KnowledgeBaseID)
	if appErr != nil {
		return appErr
	}
	chunks, err := ChunkText(doc.Content, ChunkOptions{SizeChars: base.ChunkSizeChars, OverlapChars: base.ChunkOverlapChars})
	if err != nil {
		return apperror.BadRequest("AI知识库文档分块失败")
	}
	repo, _ := s.requireRepo()
	if err := repo.ReplaceChunks(ctx, *doc, chunks, nowUTC()); err != nil {
		return wrapInternal(err)
	}
	return nil
}

func (s *Service) ListChunks(ctx context.Context, documentID uint64) (*ChunkListResponse, *apperror.Error) {
	if _, appErr := s.getDocument(ctx, documentID); appErr != nil {
		return nil, appErr
	}
	repo, _ := s.requireRepo()
	rows, err := repo.ListChunks(ctx, documentID)
	if err != nil {
		return nil, wrapInternal(err)
	}
	list := make([]ChunkDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, chunkDTO(row))
	}
	return &ChunkListResponse{List: list}, nil
}

func (s *Service) RetrievalTest(ctx context.Context, baseID uint64, input RetrievalTestInput) (*RetrievalResult, *apperror.Error) {
	base, appErr := s.getBase(ctx, baseID)
	if appErr != nil {
		return nil, appErr
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, apperror.BadRequest("检索问题不能为空")
	}
	options, appErr := retrievalOptionsFromBase(*base, input)
	if appErr != nil {
		return nil, appErr
	}
	repo, _ := s.requireRepo()
	candidates, err := repo.ListCandidates(ctx, []uint64{baseID}, candidateLimit(options.TopK))
	if err != nil {
		return nil, wrapInternal(err)
	}
	result := SelectHits(query, candidates, options)
	return &result, nil
}

func (s *Service) AgentKnowledgeBases(ctx context.Context, agentID uint64) (*AgentKnowledgeBindingsResponse, *apperror.Error) {
	repo, appErr := s.requireRepo()
	if appErr != nil {
		return nil, appErr
	}
	options, err := repo.ListActiveBaseOptions(ctx)
	if err != nil {
		return nil, wrapInternal(err)
	}
	bindings, err := repo.ListAgentKnowledgeBindings(ctx, agentID)
	if err != nil {
		return nil, wrapInternal(err)
	}
	return &AgentKnowledgeBindingsResponse{AgentID: agentID, BaseOptions: baseOptionsDTO(options), Bindings: bindingRowsDTO(bindings)}, nil
}

func (s *Service) UpdateAgentKnowledgeBases(ctx context.Context, agentID uint64, input UpdateAgentKnowledgeBindingsInput) *apperror.Error {
	repo, appErr := s.requireRepo()
	if appErr != nil {
		return appErr
	}
	options, err := repo.ListActiveBaseOptions(ctx)
	if err != nil {
		return wrapInternal(err)
	}
	rows, appErr := normalizeBindings(input.Bindings, options)
	if appErr != nil {
		return appErr
	}
	if err := repo.ReplaceAgentKnowledgeBindings(ctx, agentID, rows); err != nil {
		return wrapInternal(err)
	}
	return nil
}

func (s *Service) RetrieveForRun(ctx context.Context, input KnowledgeRuntimeInput) (*KnowledgeContextResult, *apperror.Error) {
	repo, appErr := s.requireRepo()
	if appErr != nil {
		return nil, appErr
	}
	query := strings.TrimSpace(input.Query)
	if query == "" || input.AgentID == 0 {
		return &KnowledgeContextResult{Status: RetrievalStatusSkipped}, nil
	}
	bindings, err := repo.ListRuntimeBindings(ctx, input.AgentID)
	if err != nil {
		return nil, wrapInternal(err)
	}
	if len(bindings) == 0 {
		return &KnowledgeContextResult{Status: RetrievalStatusSkipped}, nil
	}
	baseIDs := make([]uint64, 0, len(bindings))
	for _, binding := range bindings {
		baseIDs = append(baseIDs, binding.KnowledgeBaseID)
	}
	startedAt := input.StartedAt
	if startedAt.IsZero() {
		startedAt = nowUTC()
	}
	retrievalID, err := repo.CreateRetrieval(ctx, CreateRetrievalInput{RunID: input.RunID, Query: query, Status: RetrievalStatusSuccess, StartedAt: startedAt})
	if err != nil {
		return nil, wrapInternal(err)
	}
	candidates, err := repo.ListCandidates(ctx, baseIDs, candidateLimit(maxBindingTopK(bindings)))
	if err != nil {
		_ = repo.FinishRetrieval(ctx, FinishRetrievalInput{ID: retrievalID, Status: RetrievalStatusFailed, ErrorMessage: err.Error()})
		return nil, wrapInternal(err)
	}
	result := selectRuntimeHits(query, candidates, bindings)
	if err := repo.InsertRetrievalHits(ctx, retrievalID, result.Hits); err != nil {
		return nil, wrapInternal(err)
	}
	duration := uint(time.Since(startedAt).Milliseconds())
	if err := repo.FinishRetrieval(ctx, FinishRetrievalInput{ID: retrievalID, Status: result.Status, TotalHits: result.TotalHits, SelectedHits: result.SelectedHits, DurationMS: duration}); err != nil {
		return nil, wrapInternal(err)
	}
	return &KnowledgeContextResult{RetrievalID: retrievalID, Status: result.Status, Context: BuildKnowledgeContext(result.Selected)}, nil
}

func (s *Service) getBase(ctx context.Context, id uint64) (*KnowledgeBase, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("AI知识库ID不能为空")
	}
	repo, appErr := s.requireRepo()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.GetBase(ctx, id)
	if err != nil {
		return nil, wrapInternal(err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI知识库不存在")
	}
	return row, nil
}

func (s *Service) getDocument(ctx context.Context, id uint64) (*KnowledgeDocument, *apperror.Error) {
	if id == 0 {
		return nil, apperror.BadRequest("AI知识库文档ID不能为空")
	}
	repo, appErr := s.requireRepo()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.GetDocument(ctx, id)
	if err != nil {
		return nil, wrapInternal(err)
	}
	if row == nil {
		return nil, apperror.NotFound("AI知识库文档不存在")
	}
	return row, nil
}

func (s *Service) requireRepo() (Repository, *apperror.Error) {
	if s == nil || s.repo == nil {
		return nil, apperror.Internal("AI知识库服务未配置")
	}
	return s.repo, nil
}

func normalizeBaseInput(input BaseMutationInput) (KnowledgeBase, *apperror.Error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return KnowledgeBase{}, apperror.BadRequest("AI知识库名称不能为空")
	}
	if utf8.RuneCountInString(name) > 128 {
		return KnowledgeBase{}, apperror.BadRequest("AI知识库名称不能超过128个字符")
	}
	code := strings.TrimSpace(input.Code)
	if code == "" {
		return KnowledgeBase{}, apperror.BadRequest("AI知识库编码不能为空")
	}
	if utf8.RuneCountInString(code) > 128 || !codePattern.MatchString(code) {
		return KnowledgeBase{}, apperror.BadRequest("AI知识库编码只能包含小写字母、数字、下划线和中划线")
	}
	if input.ChunkSizeChars < 300 || input.ChunkSizeChars > 8000 {
		return KnowledgeBase{}, apperror.BadRequest("分块大小必须在300到8000之间")
	}
	if input.ChunkOverlapChars >= input.ChunkSizeChars {
		return KnowledgeBase{}, apperror.BadRequest("分块重叠必须小于分块大小")
	}
	if input.ChunkOverlapChars > 1000 {
		return KnowledgeBase{}, apperror.BadRequest("分块重叠不能超过1000")
	}
	if input.DefaultTopK < 1 || input.DefaultTopK > 20 {
		return KnowledgeBase{}, apperror.BadRequest("默认召回数量必须在1到20之间")
	}
	if input.DefaultMinScore < 0 || input.DefaultMinScore > 100 {
		return KnowledgeBase{}, apperror.BadRequest("默认最低分必须在0到100之间")
	}
	if input.DefaultMaxContextChars < 1000 || input.DefaultMaxContextChars > 30000 {
		return KnowledgeBase{}, apperror.BadRequest("默认上下文字符数必须在1000到30000之间")
	}
	if !enum.IsCommonStatus(input.Status) {
		return KnowledgeBase{}, apperror.BadRequest("AI知识库状态错误")
	}
	return KnowledgeBase{Name: name, Code: code, Description: strings.TrimSpace(input.Description), ChunkSizeChars: input.ChunkSizeChars, ChunkOverlapChars: input.ChunkOverlapChars, DefaultTopK: input.DefaultTopK, DefaultMinScore: input.DefaultMinScore, DefaultMaxContextChars: input.DefaultMaxContextChars, Status: input.Status, IsDel: enum.CommonNo}, nil
}

func normalizeDocumentInput(baseID uint64, input DocumentMutationInput) (KnowledgeDocument, *apperror.Error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return KnowledgeDocument{}, apperror.BadRequest("AI知识库文档标题不能为空")
	}
	if utf8.RuneCountInString(title) > 191 {
		return KnowledgeDocument{}, apperror.BadRequest("AI知识库文档标题不能超过191个字符")
	}
	sourceType := strings.TrimSpace(input.SourceType)
	if sourceType == "" {
		sourceType = SourceTypeText
	}
	if _, ok := sourceTypeLabels[sourceType]; !ok {
		return KnowledgeDocument{}, apperror.BadRequest("AI知识库文档来源类型错误")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return KnowledgeDocument{}, apperror.BadRequest("AI知识库文档内容不能为空")
	}
	if len([]byte(content)) > maxDocumentContentBytes {
		return KnowledgeDocument{}, apperror.BadRequest("AI知识库文档内容不能超过2MB")
	}
	if !enum.IsCommonStatus(input.Status) {
		return KnowledgeDocument{}, apperror.BadRequest("AI知识库文档状态错误")
	}
	return KnowledgeDocument{KnowledgeBaseID: baseID, Title: title, SourceType: sourceType, SourceRef: strings.TrimSpace(input.SourceRef), Content: content, IndexStatus: IndexStatusPending, ErrorMessage: "", Status: input.Status, IsDel: enum.CommonNo}, nil
}

func normalizeBindings(inputs []AgentKnowledgeBindingInput, options []KnowledgeBaseOptionRow) ([]AgentKnowledgeBindingInput, *apperror.Error) {
	defaults := make(map[uint64]KnowledgeBaseOptionRow, len(options))
	for _, option := range options {
		defaults[option.ID] = option
	}
	byBase := make(map[uint64]AgentKnowledgeBindingInput, len(inputs))
	for _, input := range inputs {
		if input.KnowledgeBaseID == 0 {
			return nil, apperror.BadRequest("知识库ID不能为空")
		}
		option, ok := defaults[input.KnowledgeBaseID]
		if !ok {
			return nil, apperror.BadRequest("AI知识库不存在或未启用")
		}
		row := input
		if row.TopK == 0 {
			row.TopK = option.DefaultTopK
		}
		if row.MinScore == nil {
			row.MinScore = float64Ptr(option.DefaultMinScore)
		}
		if row.MaxContextChars == 0 {
			row.MaxContextChars = option.DefaultMaxContextChars
		}
		if row.Status == 0 {
			row.Status = enum.CommonYes
		}
		if appErr := validateBindingOptions(row.TopK, bindingMinScore(row), row.MaxContextChars, row.Status); appErr != nil {
			return nil, appErr
		}
		byBase[row.KnowledgeBaseID] = row
	}
	ids := make([]uint64, 0, len(byBase))
	for id := range byBase {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	rows := make([]AgentKnowledgeBindingInput, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, byBase[id])
	}
	return rows, nil
}

func validateBindingOptions(topK uint, minScore float64, maxContextChars uint, status int) *apperror.Error {
	if topK < 1 || topK > 20 {
		return apperror.BadRequest("知识库召回数量必须在1到20之间")
	}
	if minScore < 0 || minScore > 100 {
		return apperror.BadRequest("知识库最低分必须在0到100之间")
	}
	if maxContextChars < 1000 || maxContextChars > 30000 {
		return apperror.BadRequest("知识库上下文字符数必须在1000到30000之间")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequest("知识库绑定状态错误")
	}
	return nil
}

func retrievalOptionsFromBase(base KnowledgeBase, input RetrievalTestInput) (RetrievalOptions, *apperror.Error) {
	topK := input.TopK
	if topK == 0 {
		topK = base.DefaultTopK
	}
	minScore := base.DefaultMinScore
	if input.MinScore != nil {
		minScore = *input.MinScore
	}
	maxContext := input.MaxContextChars
	if maxContext == 0 {
		maxContext = base.DefaultMaxContextChars
	}
	if appErr := validateBindingOptions(topK, minScore, maxContext, enum.CommonYes); appErr != nil {
		return RetrievalOptions{}, appErr
	}
	return RetrievalOptions{TopK: topK, MinScore: minScore, MaxContextChars: maxContext}, nil
}

func selectRuntimeHits(query string, candidates []RetrievalCandidate, bindings []RuntimeBindingRow) RetrievalResult {
	byBase := make(map[uint64][]RetrievalCandidate)
	for _, candidate := range candidates {
		byBase[candidate.KnowledgeBaseID] = append(byBase[candidate.KnowledgeBaseID], candidate)
	}
	var merged RetrievalResult
	merged.Query = strings.TrimSpace(query)
	merged.Status = RetrievalStatusSuccess
	for _, binding := range bindings {
		partial := SelectHits(query, byBase[binding.KnowledgeBaseID], RetrievalOptions{TopK: binding.TopK, MinScore: binding.MinScore, MaxContextChars: binding.MaxContextChars})
		merged.TotalHits += partial.TotalHits
		merged.Hits = append(merged.Hits, partial.Hits...)
		merged.Selected = append(merged.Selected, partial.Selected...)
	}
	sort.SliceStable(merged.Hits, func(i, j int) bool {
		if merged.Hits[i].Score == merged.Hits[j].Score {
			return merged.Hits[i].ChunkID < merged.Hits[j].ChunkID
		}
		return merged.Hits[i].Score > merged.Hits[j].Score
	})
	var selected uint
	for i := range merged.Hits {
		merged.Hits[i].RankNo = uint(i + 1)
		if merged.Hits[i].Status == HitStatusSelected {
			selected++
		}
	}
	merged.Selected = selectedHitsFromHits(merged.Hits)
	merged.SelectedHits = uint(len(merged.Selected))
	return merged
}

func selectedHitsFromHits(hits []RetrievalHit) []SelectedHit {
	selected := make([]SelectedHit, 0)
	for _, hit := range hits {
		if hit.Status != HitStatusSelected {
			continue
		}
		selected = append(selected, SelectedHit{Ref: "K" + strconvUint(uint(len(selected)+1)), KnowledgeBaseID: hit.KnowledgeBaseID, KnowledgeBaseName: hit.KnowledgeBaseName, DocumentID: hit.DocumentID, DocumentTitle: hit.DocumentTitle, ChunkID: hit.ChunkID, ChunkIndex: hit.ChunkIndex, Score: hit.Score, RankNo: hit.RankNo, Content: hit.Content})
	}
	return selected
}

func bindingMinScore(input AgentKnowledgeBindingInput) float64 {
	if input.MinScore == nil {
		return 0
	}
	return *input.MinScore
}

func float64Ptr(value float64) *float64 {
	return &value
}

func normalizeBaseListQuery(query BaseListQuery) BaseListQuery {
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
	return query
}

func normalizeDocumentListQuery(query DocumentListQuery) DocumentListQuery {
	if query.CurrentPage <= 0 {
		query.CurrentPage = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.Title = strings.TrimSpace(query.Title)
	return query
}

func buildPage(currentPage, pageSize int, total int64) Page {
	if pageSize <= 0 {
		pageSize = 20
	}
	return Page{PageSize: pageSize, CurrentPage: currentPage, Total: total, TotalPage: int(math.Ceil(float64(total) / float64(pageSize)))}
}

func baseDTO(row KnowledgeBase) BaseDTO {
	return BaseDTO{ID: row.ID, Name: row.Name, Code: row.Code, Description: row.Description, ChunkSizeChars: row.ChunkSizeChars, ChunkOverlapChars: row.ChunkOverlapChars, DefaultTopK: row.DefaultTopK, DefaultMinScore: row.DefaultMinScore, DefaultMaxContextChars: row.DefaultMaxContextChars, Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func documentDTO(row KnowledgeDocument) DocumentDTO {
	lastIndexedAt := ""
	if row.LastIndexedAt != nil {
		lastIndexedAt = formatTime(*row.LastIndexedAt)
	}
	return DocumentDTO{ID: row.ID, KnowledgeBaseID: row.KnowledgeBaseID, Title: row.Title, SourceType: row.SourceType, SourceTypeName: sourceTypeLabels[row.SourceType], SourceRef: row.SourceRef, IndexStatus: row.IndexStatus, IndexStatusName: indexStatusLabels[row.IndexStatus], ErrorMessage: row.ErrorMessage, LastIndexedAt: lastIndexedAt, Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func chunkDTO(row KnowledgeChunk) ChunkDTO {
	return ChunkDTO{ID: row.ID, KnowledgeBaseID: row.KnowledgeBaseID, DocumentID: row.DocumentID, ChunkIndex: row.ChunkIndex, Title: row.Title, Content: row.Content, ContentChars: row.ContentChars, Status: row.Status, StatusName: statusText(row.Status), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func baseOptionsDTO(rows []KnowledgeBaseOptionRow) []KnowledgeBaseOption {
	items := make([]KnowledgeBaseOption, 0, len(rows))
	for _, row := range rows {
		items = append(items, KnowledgeBaseOption{Label: row.Name, Value: row.ID, Description: row.Description, DefaultTopK: row.DefaultTopK, DefaultMinScore: row.DefaultMinScore, DefaultMaxContextChars: row.DefaultMaxContextChars})
	}
	return items
}

func bindingRowsDTO(rows []AgentKnowledgeBindingRow) []AgentKnowledgeBindingItem {
	items := make([]AgentKnowledgeBindingItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, AgentKnowledgeBindingItem{ID: row.ID, KnowledgeBaseID: row.KnowledgeBaseID, KnowledgeBaseName: row.KnowledgeBaseName, TopK: row.TopK, MinScore: row.MinScore, MaxContextChars: row.MaxContextChars, Status: row.Status, StatusName: statusText(row.Status)})
	}
	return items
}

func stringOptions(values []string, labels map[string]string) []dict.Option[string] {
	items := make([]dict.Option[string], 0, len(values))
	for _, value := range values {
		items = append(items, dict.Option[string]{Label: labels[value], Value: value})
	}
	return items
}

func statusText(status int) string {
	for _, item := range dict.CommonStatusOptions() {
		if item.Value == status {
			return item.Label
		}
	}
	return "未知"
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func nowUTC() time.Time { return time.Now().UTC() }

func candidateLimit(topK uint) int {
	if topK == 0 {
		return 100
	}
	limit := int(topK) * 20
	if limit < 100 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func maxBindingTopK(bindings []RuntimeBindingRow) uint {
	var max uint
	for _, binding := range bindings {
		if binding.TopK > max {
			max = binding.TopK
		}
	}
	return max
}

func strconvUint(v uint) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func wrapInternal(err error) *apperror.Error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrRepositoryNotConfigured) {
		return apperror.Internal("AI知识库仓储未配置")
	}
	return apperror.Internal("AI知识库服务异常")
}
