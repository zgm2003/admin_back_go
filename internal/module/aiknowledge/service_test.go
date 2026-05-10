package aiknowledge

import (
	"context"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/enum"
)

type fakeKnowledgeRepository struct {
	createdBase       *KnowledgeBase
	bases             map[uint64]KnowledgeBase
	documents         map[uint64]KnowledgeDocument
	replacedDocument  *KnowledgeDocument
	replacedChunks    []TextChunk
	replacedIndexedAt time.Time
	candidates        []RetrievalCandidate
	baseOptions       []KnowledgeBaseOptionRow
	replacedAgentID   uint64
	replacedBindings  []AgentKnowledgeBindingInput
	runtimeBindings   []RuntimeBindingRow
	listCandidatesIDs []uint64
	createRetrievals  []CreateRetrievalInput
	finishedRetrieval []FinishRetrievalInput
	insertedHits      []ScoredHit
}

func (f *fakeKnowledgeRepository) ListBases(ctx context.Context, query BaseListQuery) ([]KnowledgeBase, int64, error) {
	return nil, 0, nil
}
func (f *fakeKnowledgeRepository) GetBase(ctx context.Context, id uint64) (*KnowledgeBase, error) {
	if row, ok := f.bases[id]; ok {
		return &row, nil
	}
	return nil, nil
}
func (f *fakeKnowledgeRepository) CreateBase(ctx context.Context, row KnowledgeBase) (uint64, error) {
	f.createdBase = &row
	return 11, nil
}
func (f *fakeKnowledgeRepository) UpdateBase(ctx context.Context, id uint64, fields map[string]any) error {
	return nil
}
func (f *fakeKnowledgeRepository) ChangeBaseStatus(ctx context.Context, id uint64, status int) error {
	return nil
}
func (f *fakeKnowledgeRepository) DeleteBase(ctx context.Context, id uint64) error { return nil }
func (f *fakeKnowledgeRepository) ListDocuments(ctx context.Context, baseID uint64, query DocumentListQuery) ([]KnowledgeDocument, int64, error) {
	return nil, 0, nil
}
func (f *fakeKnowledgeRepository) GetDocument(ctx context.Context, id uint64) (*KnowledgeDocument, error) {
	if row, ok := f.documents[id]; ok {
		return &row, nil
	}
	return nil, nil
}
func (f *fakeKnowledgeRepository) CreateDocument(ctx context.Context, row KnowledgeDocument) (uint64, error) {
	return 0, nil
}
func (f *fakeKnowledgeRepository) UpdateDocument(ctx context.Context, id uint64, fields map[string]any) error {
	return nil
}
func (f *fakeKnowledgeRepository) ChangeDocumentStatus(ctx context.Context, id uint64, status int) error {
	return nil
}
func (f *fakeKnowledgeRepository) DeleteDocument(ctx context.Context, id uint64) error { return nil }
func (f *fakeKnowledgeRepository) ReplaceChunks(ctx context.Context, document KnowledgeDocument, chunks []TextChunk, indexedAt time.Time) error {
	f.replacedDocument = &document
	f.replacedChunks = append([]TextChunk(nil), chunks...)
	f.replacedIndexedAt = indexedAt
	return nil
}
func (f *fakeKnowledgeRepository) ListChunks(ctx context.Context, documentID uint64) ([]KnowledgeChunk, error) {
	return nil, nil
}
func (f *fakeKnowledgeRepository) ListActiveBaseOptions(ctx context.Context) ([]KnowledgeBaseOptionRow, error) {
	return f.baseOptions, nil
}
func (f *fakeKnowledgeRepository) ListAgentKnowledgeBindings(ctx context.Context, agentID uint64) ([]AgentKnowledgeBindingRow, error) {
	return nil, nil
}
func (f *fakeKnowledgeRepository) ReplaceAgentKnowledgeBindings(ctx context.Context, agentID uint64, rows []AgentKnowledgeBindingInput) error {
	f.replacedAgentID = agentID
	f.replacedBindings = append([]AgentKnowledgeBindingInput(nil), rows...)
	return nil
}
func (f *fakeKnowledgeRepository) ListRuntimeBindings(ctx context.Context, agentID uint64) ([]RuntimeBindingRow, error) {
	return f.runtimeBindings, nil
}
func (f *fakeKnowledgeRepository) ListCandidates(ctx context.Context, baseIDs []uint64, limit int) ([]RetrievalCandidate, error) {
	f.listCandidatesIDs = append([]uint64(nil), baseIDs...)
	return f.candidates, nil
}
func (f *fakeKnowledgeRepository) CreateRetrieval(ctx context.Context, input CreateRetrievalInput) (uint64, error) {
	f.createRetrievals = append(f.createRetrievals, input)
	return 100, nil
}
func (f *fakeKnowledgeRepository) FinishRetrieval(ctx context.Context, input FinishRetrievalInput) error {
	f.finishedRetrieval = append(f.finishedRetrieval, input)
	return nil
}
func (f *fakeKnowledgeRepository) InsertRetrievalHits(ctx context.Context, retrievalID uint64, hits []ScoredHit) error {
	f.insertedHits = append([]ScoredHit(nil), hits...)
	return nil
}

func TestCreateKnowledgeBaseStoresAllFields(t *testing.T) {
	repo := &fakeKnowledgeRepository{}
	id, appErr := NewService(repo).CreateBase(context.Background(), BaseMutationInput{
		Name:                   "项目知识库",
		Code:                   "admin_go-project",
		Description:            "架构说明",
		ChunkSizeChars:         1200,
		ChunkOverlapChars:      120,
		DefaultTopK:            6,
		DefaultMinScore:        0.25,
		DefaultMaxContextChars: 7000,
		Status:                 enum.CommonYes,
	})
	if appErr != nil {
		t.Fatalf("CreateBase returned error: %v", appErr)
	}
	if id != 11 || repo.createdBase == nil {
		t.Fatalf("base not created: id=%d row=%#v", id, repo.createdBase)
	}
	got := repo.createdBase
	if got.Name != "项目知识库" || got.Code != "admin_go-project" || got.Description != "架构说明" {
		t.Fatalf("base text fields mismatch: %#v", got)
	}
	if got.ChunkSizeChars != 1200 || got.ChunkOverlapChars != 120 || got.DefaultTopK != 6 || got.DefaultMinScore != 0.25 || got.DefaultMaxContextChars != 7000 {
		t.Fatalf("base rag fields mismatch: %#v", got)
	}
	if got.Status != enum.CommonYes || got.IsDel != enum.CommonNo {
		t.Fatalf("base status/is_del mismatch: %#v", got)
	}
}

func TestReindexDocumentReplacesChunksAndMarksIndexed(t *testing.T) {
	content := ""
	for i := 0; i < 900; i++ {
		content += "a"
	}
	repo := &fakeKnowledgeRepository{
		bases:     map[uint64]KnowledgeBase{3: {ID: 3, ChunkSizeChars: 400, ChunkOverlapChars: 50, Status: enum.CommonYes, IsDel: enum.CommonNo}},
		documents: map[uint64]KnowledgeDocument{9: {ID: 9, KnowledgeBaseID: 3, Title: "Go 后端架构", Content: content, Status: enum.CommonYes, IsDel: enum.CommonNo}},
	}
	appErr := NewService(repo).ReindexDocument(context.Background(), 9)
	if appErr != nil {
		t.Fatalf("ReindexDocument returned error: %v", appErr)
	}
	if repo.replacedDocument == nil || repo.replacedDocument.ID != 9 || repo.replacedDocument.KnowledgeBaseID != 3 {
		t.Fatalf("document not passed to ReplaceChunks: %#v", repo.replacedDocument)
	}
	if len(repo.replacedChunks) != 3 {
		t.Fatalf("chunk count=%d want 3: %#v", len(repo.replacedChunks), repo.replacedChunks)
	}
	if repo.replacedChunks[0].Index != 1 || repo.replacedChunks[0].Chars != 400 || repo.replacedChunks[1].Chars != 400 || repo.replacedChunks[2].Chars != 200 {
		t.Fatalf("unexpected chunk metadata: %#v", repo.replacedChunks)
	}
	if repo.replacedIndexedAt.IsZero() {
		t.Fatal("indexedAt must be set so repository can mark document indexed")
	}
}

func TestRetrievalTestReturnsSelectedHits(t *testing.T) {
	repo := &fakeKnowledgeRepository{
		bases: map[uint64]KnowledgeBase{1: {ID: 1, Name: "架构库", DefaultTopK: 2, DefaultMinScore: 0.1, DefaultMaxContextChars: 1000, Status: enum.CommonYes, IsDel: enum.CommonNo}},
		candidates: []RetrievalCandidate{
			{KnowledgeBaseID: 1, KnowledgeBaseName: "架构库", DocumentID: 2, DocumentTitle: "Go 后端架构", ChunkID: 3, ChunkIndex: 1, Title: "Go 后端架构", Content: "Gin modular monolith route handler service repository model", ContentChars: 60},
			{KnowledgeBaseID: 1, KnowledgeBaseName: "架构库", DocumentID: 4, DocumentTitle: "无关", ChunkID: 5, ChunkIndex: 1, Title: "无关", Content: "天气很好", ContentChars: 4},
		},
	}
	res, appErr := NewService(repo).RetrievalTest(context.Background(), 1, RetrievalTestInput{Query: "Gin route"})
	if appErr != nil {
		t.Fatalf("RetrievalTest returned error: %v", appErr)
	}
	if !reflect.DeepEqual(repo.listCandidatesIDs, []uint64{1}) {
		t.Fatalf("candidate base ids mismatch: %#v", repo.listCandidatesIDs)
	}
	if res.TotalHits != 2 || res.SelectedHits != 1 || len(res.Hits) != 2 || len(res.Selected) != 1 {
		t.Fatalf("unexpected retrieval result: %#v", res)
	}
	if res.Selected[0].KnowledgeBaseID != 1 || res.Selected[0].ChunkID != 3 || res.Hits[0].Status != HitStatusSelected || res.Hits[1].Status != HitStatusSkipped {
		t.Fatalf("selected hit mismatch: %#v", res)
	}
}

func TestUpdateAgentKnowledgeBasesNormalizesBindings(t *testing.T) {
	repo := &fakeKnowledgeRepository{baseOptions: []KnowledgeBaseOptionRow{
		{ID: 1, Name: "架构库", DefaultTopK: 5, DefaultMinScore: 0.1, DefaultMaxContextChars: 6000},
		{ID: 2, Name: "产品库", DefaultTopK: 4, DefaultMinScore: 0.2, DefaultMaxContextChars: 5000},
	}}
	appErr := NewService(repo).UpdateAgentKnowledgeBases(context.Background(), 7, UpdateAgentKnowledgeBindingsInput{Bindings: []AgentKnowledgeBindingInput{
		{KnowledgeBaseID: 2, TopK: 3, MinScore: float64Ptr(0.3), MaxContextChars: 4000, Status: enum.CommonYes},
		{KnowledgeBaseID: 1, Status: enum.CommonYes},
		{KnowledgeBaseID: 2, TopK: 8, MinScore: float64Ptr(0.4), MaxContextChars: 9000, Status: enum.CommonNo},
	}})
	if appErr != nil {
		t.Fatalf("UpdateAgentKnowledgeBases returned error: %v", appErr)
	}
	if repo.replacedAgentID != 7 {
		t.Fatalf("agent id mismatch: %d", repo.replacedAgentID)
	}
	want := []AgentKnowledgeBindingInput{
		{KnowledgeBaseID: 1, TopK: 5, MinScore: float64Ptr(0.1), MaxContextChars: 6000, Status: enum.CommonYes},
		{KnowledgeBaseID: 2, TopK: 8, MinScore: float64Ptr(0.4), MaxContextChars: 9000, Status: enum.CommonNo},
	}
	if !reflect.DeepEqual(repo.replacedBindings, want) {
		t.Fatalf("bindings mismatch:\n got=%#v\nwant=%#v", repo.replacedBindings, want)
	}
}

func TestUpdateAgentKnowledgeBasesKeepsExplicitZeroMinScore(t *testing.T) {
	repo := &fakeKnowledgeRepository{baseOptions: []KnowledgeBaseOptionRow{
		{ID: 1, Name: "架构库", DefaultTopK: 5, DefaultMinScore: 0.8, DefaultMaxContextChars: 6000},
	}}
	appErr := NewService(repo).UpdateAgentKnowledgeBases(context.Background(), 7, UpdateAgentKnowledgeBindingsInput{Bindings: []AgentKnowledgeBindingInput{
		{KnowledgeBaseID: 1, TopK: 5, MinScore: float64Ptr(0), MaxContextChars: 6000, Status: enum.CommonYes},
	}})
	if appErr != nil {
		t.Fatalf("UpdateAgentKnowledgeBases returned error: %v", appErr)
	}
	if len(repo.replacedBindings) != 1 || repo.replacedBindings[0].MinScore == nil || *repo.replacedBindings[0].MinScore != 0 {
		t.Fatalf("explicit zero min_score must be preserved: %#v", repo.replacedBindings)
	}
}

func TestRuntimeRetrieveSkipsWhenAgentHasNoBindings(t *testing.T) {
	repo := &fakeKnowledgeRepository{}
	res, appErr := NewService(repo).RetrieveForRun(context.Background(), KnowledgeRuntimeInput{RunID: 10, AgentID: 99, Query: "Go 架构"})
	if appErr != nil {
		t.Fatalf("RetrieveForRun returned error: %v", appErr)
	}
	if res == nil || res.Status != RetrievalStatusSkipped || res.Context != "" {
		t.Fatalf("runtime skip result mismatch: %#v", res)
	}
	if len(repo.createRetrievals) != 0 || len(repo.finishedRetrieval) != 0 || len(repo.insertedHits) != 0 || len(repo.listCandidatesIDs) != 0 {
		t.Fatalf("runtime without bindings must not touch retrieval candidates/logs: %#v", repo)
	}
}
