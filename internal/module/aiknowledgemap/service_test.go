package aiknowledgemap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/platform/secretbox"
)

type fakeKnowledgeMapRepository struct {
	maps              []MapWithEngine
	total             int64
	rawMaps           map[uint64]KnowledgeMap
	mapsByID          map[uint64]MapWithEngine
	connections       []Provider
	activeProviders   map[uint64]Provider
	existsCode        bool
	createdMap        *KnowledgeMap
	mapUpdates        []map[string]any
	mapStatusID       uint64
	mapStatus         int
	deletedMapID      uint64
	documents         []Document
	docByID           map[uint64]DocumentWithMap
	createdDocument   *Document
	documentUpdates   []map[string]any
	documentStatusID  uint64
	documentStatus    int
	deletedDocumentID uint64
	syncableDocuments []Document
}

func (f *fakeKnowledgeMapRepository) List(ctx context.Context, query ListQuery) ([]MapWithEngine, int64, error) {
	return f.maps, f.total, nil
}
func (f *fakeKnowledgeMapRepository) Get(ctx context.Context, id uint64) (*MapWithEngine, error) {
	row, ok := f.mapsByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}
func (f *fakeKnowledgeMapRepository) GetRaw(ctx context.Context, id uint64) (*KnowledgeMap, error) {
	row, ok := f.rawMaps[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}
func (f *fakeKnowledgeMapRepository) ListActiveProviders(ctx context.Context) ([]Provider, error) {
	return f.connections, nil
}
func (f *fakeKnowledgeMapRepository) GetActiveProvider(ctx context.Context, id uint64) (*Provider, error) {
	row, ok := f.activeProviders[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}
func (f *fakeKnowledgeMapRepository) ExistsByCode(ctx context.Context, code string, excludeID uint64) (bool, error) {
	return f.existsCode, nil
}
func (f *fakeKnowledgeMapRepository) Create(ctx context.Context, row KnowledgeMap) (uint64, error) {
	f.createdMap = &row
	return 11, nil
}
func (f *fakeKnowledgeMapRepository) Update(ctx context.Context, id uint64, fields map[string]any) error {
	f.mapUpdates = append(f.mapUpdates, fields)
	return nil
}
func (f *fakeKnowledgeMapRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	f.mapStatusID = id
	f.mapStatus = status
	return nil
}
func (f *fakeKnowledgeMapRepository) Delete(ctx context.Context, id uint64) error {
	f.deletedMapID = id
	return nil
}
func (f *fakeKnowledgeMapRepository) ListDocuments(ctx context.Context, mapID uint64) ([]Document, error) {
	return f.documents, nil
}
func (f *fakeKnowledgeMapRepository) CreateDocument(ctx context.Context, row Document) (uint64, error) {
	f.createdDocument = &row
	return 22, nil
}
func (f *fakeKnowledgeMapRepository) GetDocument(ctx context.Context, id uint64) (*DocumentWithMap, error) {
	row, ok := f.docByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}
func (f *fakeKnowledgeMapRepository) UpdateDocument(ctx context.Context, id uint64, fields map[string]any) error {
	f.documentUpdates = append(f.documentUpdates, fields)
	return nil
}
func (f *fakeKnowledgeMapRepository) ChangeDocumentStatus(ctx context.Context, id uint64, status int) error {
	f.documentStatusID = id
	f.documentStatus = status
	return nil
}
func (f *fakeKnowledgeMapRepository) DeleteDocument(ctx context.Context, id uint64) error {
	f.deletedDocumentID = id
	return nil
}
func (f *fakeKnowledgeMapRepository) ListSyncableDocuments(ctx context.Context, mapID uint64) ([]Document, error) {
	return f.syncableDocuments, nil
}

type fakeKnowledgeEngineFactory struct {
	engine *fakeKnowledgeEngine
	input  EngineConfig
	err    error
}

func (f *fakeKnowledgeEngineFactory) NewEngine(ctx context.Context, input EngineConfig) (platformai.Engine, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return f.engine, nil
}

type fakeKnowledgeEngine struct {
	syncInputs   []platformai.KnowledgeSyncInput
	statusInputs []platformai.KnowledgeStatusInput
	syncResult   *platformai.KnowledgeSyncResult
	statusResult *platformai.KnowledgeStatusResult
	err          error
}

func (f *fakeKnowledgeEngine) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	return &platformai.TestConnectionResult{OK: true}, nil
}
func (f *fakeKnowledgeEngine) StreamChat(ctx context.Context, input platformai.ChatInput, sink platformai.EventSink) (*platformai.ChatResult, error) {
	return nil, nil
}
func (f *fakeKnowledgeEngine) StopChat(ctx context.Context, input platformai.StopChatInput) error {
	return nil
}
func (f *fakeKnowledgeEngine) SyncKnowledge(ctx context.Context, input platformai.KnowledgeSyncInput) (*platformai.KnowledgeSyncResult, error) {
	f.syncInputs = append(f.syncInputs, input)
	if f.err != nil {
		return nil, f.err
	}
	if f.syncResult != nil {
		return f.syncResult, nil
	}
	return &platformai.KnowledgeSyncResult{EngineDatasetID: input.DatasetID, EngineDocumentID: "doc-1", EngineBatch: "batch-1", IndexingStatus: "indexing"}, nil
}
func (f *fakeKnowledgeEngine) KnowledgeStatus(ctx context.Context, input platformai.KnowledgeStatusInput) (*platformai.KnowledgeStatusResult, error) {
	f.statusInputs = append(f.statusInputs, input)
	if f.err != nil {
		return nil, f.err
	}
	if f.statusResult != nil {
		return f.statusResult, nil
	}
	return &platformai.KnowledgeStatusResult{IndexingStatus: "completed"}, nil
}

func TestCreateTextDocumentSyncsEngineAndStoresResult(t *testing.T) {
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("plain-engine-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeKnowledgeMapRepository{
		rawMaps:         map[uint64]KnowledgeMap{7: {ID: 7, ProviderID: 3, EngineDatasetID: "dataset-1", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		activeProviders: map[uint64]Provider{3: {ID: 3, EngineType: "dify", BaseURL: "https://api.dify.test/v1", APIKeyEnc: cipher, Status: enum.CommonYes, IsDel: enum.CommonNo}},
	}
	engine := &fakeKnowledgeEngine{}
	factory := &fakeKnowledgeEngineFactory{engine: engine}
	service := NewService(repo, box, factory)

	id, appErr := service.CreateDocument(context.Background(), 7, DocumentInput{Name: "FAQ", SourceType: "text", Content: "hello", Status: enum.CommonYes})
	if appErr != nil {
		t.Fatalf("expected create document to succeed, got %v", appErr)
	}
	if id != 22 {
		t.Fatalf("expected created id 22, got %d", id)
	}
	if factory.input.APIKey != "plain-engine-key" || factory.input.BaseURL != "https://api.dify.test/v1" || factory.input.EngineType != platformai.EngineTypeDify {
		t.Fatalf("unexpected engine config: %#v", factory.input)
	}
	if len(engine.syncInputs) != 1 || engine.syncInputs[0].DatasetID != "dataset-1" || engine.syncInputs[0].Document.Name != "FAQ" || engine.syncInputs[0].Document.Text != "hello" {
		t.Fatalf("unexpected sync input: %#v", engine.syncInputs)
	}
	if repo.createdDocument == nil || repo.createdDocument.IndexingStatus != "pending" {
		t.Fatalf("document should be created pending before sync, got %#v", repo.createdDocument)
	}
	if len(repo.documentUpdates) != 1 || repo.documentUpdates[0]["engine_document_id"] != "doc-1" || repo.documentUpdates[0]["engine_batch"] != "batch-1" || repo.documentUpdates[0]["indexing_status"] != "indexing" {
		t.Fatalf("sync result must be stored, got %#v", repo.documentUpdates)
	}
}

func TestSyncRejectsDisabledMap(t *testing.T) {
	repo := &fakeKnowledgeMapRepository{rawMaps: map[uint64]KnowledgeMap{7: {ID: 7, Status: enum.CommonNo, IsDel: enum.CommonNo}}}
	service := NewService(repo, secretbox.New("vault-key"), &fakeKnowledgeEngineFactory{engine: &fakeKnowledgeEngine{}})

	appErr := service.Sync(context.Background(), 7)
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "AI知识库已禁用" {
		t.Fatalf("expected disabled map error, got %#v", appErr)
	}
}

func TestCreateTextDocumentStoresErrorWhenEngineFails(t *testing.T) {
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("plain-engine-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeKnowledgeMapRepository{
		rawMaps:         map[uint64]KnowledgeMap{7: {ID: 7, ProviderID: 3, EngineDatasetID: "dataset-1", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		activeProviders: map[uint64]Provider{3: {ID: 3, EngineType: "dify", BaseURL: "https://api.dify.test/v1", APIKeyEnc: cipher, Status: enum.CommonYes, IsDel: enum.CommonNo}},
	}
	service := NewService(repo, box, &fakeKnowledgeEngineFactory{engine: &fakeKnowledgeEngine{err: errors.New("dify failed")}})

	_, appErr := service.CreateDocument(context.Background(), 7, DocumentInput{Name: "FAQ", SourceType: "text", Content: "hello", Status: enum.CommonYes})
	if appErr == nil || appErr.Code == apperror.CodeOK {
		t.Fatalf("expected upstream error, got %#v", appErr)
	}
	if len(repo.documentUpdates) != 1 || !strings.Contains(repo.documentUpdates[0]["error_message"].(string), "dify failed") {
		t.Fatalf("upstream error must be stored, got %#v", repo.documentUpdates)
	}
}

func TestListDoesNotLeakEngineSecret(t *testing.T) {
	now := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	repo := &fakeKnowledgeMapRepository{maps: []MapWithEngine{{KnowledgeMap: KnowledgeMap{ID: 1, ProviderID: 3, Name: "客服库", Code: "support", EngineDatasetID: "dataset-1", Visibility: "private", MetaJSON: `{"owner":"ops"}`, Status: enum.CommonYes, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now}, ProviderName: "Dify", EngineType: "dify", EngineAPIKeyEnc: "cipher-secret"}}, total: 1}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"api_key", "api_key_enc", "cipher-secret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("list response leaked %q in %s", forbidden, body)
		}
	}
	if len(got.List) != 1 || string(got.List[0].MetaJSON) != `{"owner":"ops"}` {
		t.Fatalf("unexpected list response: %#v", got.List)
	}
}
