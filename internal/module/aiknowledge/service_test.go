package aiknowledge

import (
	"context"
	"testing"
)

func TestServiceCreatesDocumentAndReindexesChunks(t *testing.T) {
	repo := &fakeRepo{kb: &KnowledgeBase{ID: 1, Name: "KB", ChunkSize: 4, ChunkOverlap: 1, TopK: 5, ScoreThreshold: 0, Status: 1, IsDel: 2}}
	service := NewService(repo)
	id, appErr := service.CreateDocument(context.Background(), 7, DocumentMutationInput{KnowledgeBaseID: 1, Title: "Doc", SourceType: "manual", Content: "abcdefghij", Status: 1})
	if appErr != nil {
		t.Fatalf("CreateDocument error: %v", appErr)
	}
	if id != 55 || !repo.txCalled {
		t.Fatalf("document not created in tx: id=%d tx=%v", id, repo.txCalled)
	}
	if len(repo.chunks) != 3 || repo.chunkCount != 3 || repo.indexStatus != 1 {
		t.Fatalf("reindex mismatch chunks=%#v count=%d status=%d", repo.chunks, repo.chunkCount, repo.indexStatus)
	}
}

func TestServiceRejectsInvalidKnowledgeSource(t *testing.T) {
	repo := &fakeRepo{kb: &KnowledgeBase{ID: 1, ChunkSize: 800, ChunkOverlap: 120, Status: 1, IsDel: 2}}
	service := NewService(repo)
	_, appErr := service.CreateDocument(context.Background(), 7, DocumentMutationInput{KnowledgeBaseID: 1, Title: "Doc", SourceType: "file", Content: "content"})
	if appErr == nil {
		t.Fatalf("expected file source rejection")
	}
}

func TestServiceRetrievalRanksChunks(t *testing.T) {
	repo := &fakeRepo{kb: &KnowledgeBase{ID: 1, ChunkSize: 800, ChunkOverlap: 120, TopK: 5, ScoreThreshold: 0, Status: 1, IsDel: 2}, candidates: []RetrievalChunk{{KnowledgeBaseID: 1, DocumentID: 2, DocumentTitle: "Doc", ChunkNo: 1, Content: "知识库 权限 权限"}}}
	service := NewService(repo)
	res, appErr := service.RetrievalTest(context.Background(), RetrievalInput{KnowledgeBaseID: 1, Query: "知识库 权限", TopK: 5})
	if appErr != nil {
		t.Fatalf("RetrievalTest error: %v", appErr)
	}
	if len(res.Chunks) != 1 || res.Chunks[0].Score <= 2 || res.ContextPrompt == "" {
		t.Fatalf("unexpected retrieval: %#v", res)
	}
}

type fakeRepo struct {
	kb          *KnowledgeBase
	txCalled    bool
	chunks      []ChunkPayload
	chunkCount  int
	indexStatus int
	candidates  []RetrievalChunk
}

func (f *fakeRepo) Init(ctx context.Context) (*InitResponse, error) { return nil, nil }
func (f *fakeRepo) List(ctx context.Context, query ListQuery) ([]KnowledgeBase, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepo) Get(ctx context.Context, id int64) (*KnowledgeBase, error) {
	if f.kb != nil && f.kb.ID == id {
		return f.kb, nil
	}
	return nil, nil
}
func (f *fakeRepo) Create(ctx context.Context, row KnowledgeBase) (int64, error)      { return 0, nil }
func (f *fakeRepo) Update(ctx context.Context, id int64, fields map[string]any) error { return nil }
func (f *fakeRepo) ChangeStatus(ctx context.Context, id int64, status int) error      { return nil }
func (f *fakeRepo) Delete(ctx context.Context, ids []int64) (int64, error)            { return 0, nil }
func (f *fakeRepo) ListDocuments(ctx context.Context, query DocumentListQuery) ([]Document, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepo) GetDocument(ctx context.Context, id int64, knowledgeBaseID int64) (*Document, error) {
	return nil, nil
}
func (f *fakeRepo) CreateDocument(ctx context.Context, row Document) (int64, error) { return 55, nil }
func (f *fakeRepo) UpdateDocument(ctx context.Context, id int64, fields map[string]any) error {
	return nil
}
func (f *fakeRepo) DeleteDocument(ctx context.Context, id int64, knowledgeBaseID int64) error {
	return nil
}
func (f *fakeRepo) ListChunks(ctx context.Context, query ChunkListQuery) ([]Chunk, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepo) ReplaceDocumentChunks(ctx context.Context, knowledgeBaseID int64, documentID int64, chunks []ChunkPayload) (int, error) {
	f.chunks = chunks
	return len(chunks), nil
}
func (f *fakeRepo) UpdateDocumentChunkStatus(ctx context.Context, id int64, chunkCount int, indexStatus int) error {
	f.chunkCount = chunkCount
	f.indexStatus = indexStatus
	return nil
}
func (f *fakeRepo) CandidateChunks(ctx context.Context, knowledgeBaseID int64, terms []string, limit int) ([]RetrievalChunk, error) {
	return f.candidates, nil
}
func (f *fakeRepo) WithTx(ctx context.Context, fn func(Repository) error) error {
	f.txCalled = true
	return fn(f)
}

var _ Repository = (*fakeRepo)(nil)
