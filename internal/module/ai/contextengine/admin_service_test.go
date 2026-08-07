package contextengine

import (
	"context"
	"errors"
	"io"
	"testing"

	"admin_back_go/internal/infra/storage"
	"admin_back_go/internal/module/ai/officialmodel"
	aiprovider "admin_back_go/internal/module/ai/provider"
)

func TestProfileCreateValidatesModelKindsAndExplicitPolicy(t *testing.T) {
	repository := &fakeAdminRepository{models: map[uint64]ProviderModelCapability{
		11: {ID: 11, Kind: aiprovider.ModelKindEmbedding, Enabled: true, ProviderEnabled: true},
		22: {ID: 22, Kind: aiprovider.ModelKindRerank, Enabled: true, ProviderEnabled: true},
		33: {ID: 33, Kind: aiprovider.ModelKindChat, Enabled: true, ProviderEnabled: true, OfficialModelID: "gpt-memory"},
	}}
	service := NewAdminService(repository, nil, nil, WithOfficialModelResolver(officialmodel.ResolverFunc(func(context.Context, string) (officialmodel.ResolvedModel, error) {
		return officialmodel.ResolvedModel{Model: officialmodel.Model{ContextWindowTokens: 128000, MaxOutputTokens: 8192, TokenCounterID: "utf8_bytes_v1"}}, nil
	})))
	profile, appErr := service.CreateProfile(context.Background(), 7, CreateProfileInput{
		Name: "default", EmbeddingProviderModelID: 11, EmbeddingDimensions: 1536,
		EmbeddingMaxInputTokens: 8192, EmbeddingTokenCounterID: "utf8_bytes_v1",
		DenseDistance: "cosine", DenseMinScore: "0.200000", RerankerProviderModelID: uint64Pointer(22),
		RerankerMinScore: stringPointer("0.300000"), MemoryProviderModelID: uint64Pointer(33),
	})
	if appErr != nil || profile == nil || repository.createdProfile == nil {
		t.Fatalf("profile=%#v error=%#v", profile, appErr)
	}
	if repository.createdProfile.SparseEncoder != SparseEncoderUnicodeLexicalV1 || repository.createdProfile.TargetIndexGeneration == nil || *repository.createdProfile.TargetIndexGeneration != 1 {
		t.Fatalf("created profile=%#v", repository.createdProfile)
	}

	repository.models[11] = ProviderModelCapability{ID: 11, Kind: aiprovider.ModelKindChat, Enabled: true, ProviderEnabled: true}
	if _, appErr := service.CreateProfile(context.Background(), 7, CreateProfileInput{
		Name: "bad", EmbeddingProviderModelID: 11, EmbeddingDimensions: 1536,
		EmbeddingMaxInputTokens: 8192, EmbeddingTokenCounterID: "utf8_bytes_v1",
		DenseDistance: "cosine", DenseMinScore: "0.200000",
	}); appErr == nil {
		t.Fatal("wrong embedding model kind was accepted")
	}
}

func TestProfileCreateEnqueuesCommittedProfileRebuild(t *testing.T) {
	repository := &fakeAdminRepository{models: map[uint64]ProviderModelCapability{
		11: {ID: 11, Kind: aiprovider.ModelKindEmbedding, Enabled: true, ProviderEnabled: true},
	}}
	enqueuer := &recordingProfileRebuildEnqueuer{}
	service := NewAdminService(repository, nil, nil, WithProfileRebuildEnqueuer(enqueuer))
	profile, appErr := service.CreateProfile(context.Background(), 7, CreateProfileInput{
		Name: "default", EmbeddingProviderModelID: 11, EmbeddingDimensions: 1536,
		EmbeddingMaxInputTokens: 8191, EmbeddingTokenCounterID: "utf8_bytes_v1",
		DenseDistance: "cosine", DenseMinScore: "0.200000",
	})
	if appErr != nil || profile == nil {
		t.Fatalf("profile=%#v error=%#v", profile, appErr)
	}
	if len(enqueuer.profiles) != 1 || enqueuer.profiles[0].ID != profile.ID {
		t.Fatalf("profiles=%+v", enqueuer.profiles)
	}
}

func TestSpaceProfileChangeRequiresUnreferencedSpace(t *testing.T) {
	repository := &fakeAdminRepository{
		profiles:        map[uint64]ContextProfile{1: readyProfile(1), 2: readyProfile(2)},
		spaces:          map[uint64]ContextSpace{9: {ID: 9, Platform: "admin", ProfileID: 1, Name: "docs", Status: SpaceEnabled}},
		spaceReferenced: true,
	}
	service := NewAdminService(repository, nil, nil)
	if _, appErr := service.UpdateSpace(context.Background(), "admin", 9, UpdateSpaceInput{Name: "docs", Status: SpaceEnabled, ProfileID: 2}); appErr == nil {
		t.Fatal("referenced space changed profile")
	}
	repository.spaceReferenced = false
	space, appErr := service.UpdateSpace(context.Background(), "admin", 9, UpdateSpaceInput{Name: "docs", Status: SpaceEnabled, ProfileID: 2})
	if appErr != nil || space.ProfileID != 2 {
		t.Fatalf("space=%#v error=%#v", space, appErr)
	}
}

func TestProfileAssignmentRequiresReadyAndRejectsAgentConflict(t *testing.T) {
	repository := &fakeAdminRepository{profiles: map[uint64]ContextProfile{1: readyProfile(1)}}
	service := NewAdminService(repository, nil, nil)
	if err := service.RequireAssignable(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	repository.agentConflict = true
	if err := service.RequireAgentProfileChangeAllowed(context.Background(), 9, uint64Pointer(1)); err == nil {
		t.Fatal("agent profile conflict was accepted")
	}
	if err := service.RequireAgentProfileChangeAllowed(context.Background(), 9, nil); err != nil {
		t.Fatalf("clearing an agent profile must remain available with historical context: %v", err)
	}
	profile := repository.profiles[1]
	profile.Status = ProfileRetired
	repository.profiles[1] = profile
	if err := service.RequireAssignable(context.Background(), 1); err == nil {
		t.Fatal("retired profile was assignable")
	}
}

func TestDocumentCreateChecksOwnershipObjectAndReturnsCommittedOnEnqueueFailure(t *testing.T) {
	repository := &fakeAdminRepository{
		profiles: map[uint64]ContextProfile{1: readyProfile(1)},
		spaces:   map[uint64]ContextSpace{9: {ID: 9, Platform: "admin", ProfileID: 1, Name: "docs", Status: SpaceEnabled}},
	}
	objects := &fakeConditionalReader{metadata: storage.ConditionalObjectMetadata{ETag: `"verified"`, Size: 4, MIMEType: "text/plain"}}
	service := NewAdminService(repository, objects, failingVersionEnqueuer{})
	document, appErr := service.CreateDocument(context.Background(), "admin", 7, CreateDocumentInput{
		SpaceID: uint64Pointer(9), Title: "notes", SourceStorageProvider: "cos",
		SourceObjectKey: "ai_context_documents/notes.txt", SourceETag: `"verified"`, SourceSize: 4, SourceFilename: "notes.txt",
	})
	if appErr != nil || document == nil || document.Version.State != DocumentVersionQueued {
		t.Fatalf("document=%#v error=%#v", document, appErr)
	}
	if repository.createdDocument == nil || repository.createdDocument.Version.SourceMIMEType != "text/plain" {
		t.Fatalf("persisted=%#v", repository.createdDocument)
	}

	conversationID := uint64(10)
	invalid := CreateDocumentInput{SpaceID: uint64Pointer(9), ConversationID: &conversationID}
	if _, appErr := service.CreateDocument(context.Background(), "admin", 7, invalid); appErr == nil {
		t.Fatal("document with two owners was accepted")
	}
}

func TestDocumentReindexCreatesNewImmutableVersion(t *testing.T) {
	repository := &fakeAdminRepository{documents: map[uint64]DocumentAdminDTO{5: {
		ID: 5, SpaceID: uint64Pointer(9), ProfileID: 1, Version: DocumentVersionDTO{ID: 6, DocumentID: 5, ProfileID: 1,
			SourceStorageProvider: "cos", SourceObjectKey: "ai_context_documents/a.txt", SourceETag: `"v1"`, SourceSize: 4,
			SourceMIMEType: "text/plain", SourceFilename: "a.txt", State: DocumentVersionReady},
	}}}
	service := NewAdminService(repository, nil, nil)
	result, appErr := service.ReindexDocument(context.Background(), "admin", 5)
	if appErr != nil || result.Version.ID == 6 || result.Version.State != DocumentVersionQueued {
		t.Fatalf("result=%#v error=%#v", result, appErr)
	}
}

func TestAdminContractMissingDocumentIsNotAnEmptyVersionList(t *testing.T) {
	service := NewAdminService(&fakeAdminRepository{}, nil, nil)
	result, appErr := service.ListDocumentVersions(context.Background(), "admin", 99)
	if appErr == nil || result != nil {
		t.Fatalf("result=%+v error=%+v", result, appErr)
	}
}

func TestContextPageInitPartitionsProviderModelOptions(t *testing.T) {
	repository := &fakeAdminRepository{modelOptions: []ProviderModelOption{
		{ID: 11, ProviderName: "Alpha", ModelID: "embed-v1", ModelKind: aiprovider.ModelKindEmbedding, DisplayName: "Embedding One"},
		{ID: 12, ProviderName: "Alpha", ModelID: "rerank-v1", ModelKind: aiprovider.ModelKindRerank},
		{ID: 13, ProviderName: "Beta", ModelID: "chat-v1", ModelKind: aiprovider.ModelKindChat, DisplayName: "Memory Chat"},
	}}

	result, appErr := NewAdminService(repository, nil, nil).PageInit(context.Background())
	if appErr != nil {
		t.Fatalf("PageInit error = %#v", appErr)
	}
	if len(result.EmbeddingModelOptions) != 1 || result.EmbeddingModelOptions[0] != (ProviderModelOptionDTO{
		Value: 11, Label: "Alpha / Embedding One", ProviderName: "Alpha", ModelID: "embed-v1",
	}) {
		t.Fatalf("embedding options = %#v", result.EmbeddingModelOptions)
	}
	if len(result.RerankerModelOptions) != 1 || result.RerankerModelOptions[0].Label != "Alpha / rerank-v1" {
		t.Fatalf("reranker options = %#v", result.RerankerModelOptions)
	}
	if len(result.MemoryModelOptions) != 1 || result.MemoryModelOptions[0].Value != 13 {
		t.Fatalf("memory options = %#v", result.MemoryModelOptions)
	}
}

func TestContextPageInitReturnsEmptyArraysWithoutFabricatedOptions(t *testing.T) {
	result, appErr := NewAdminService(&fakeAdminRepository{}, nil, nil).PageInit(context.Background())
	if appErr != nil {
		t.Fatalf("PageInit error = %#v", appErr)
	}
	if result.EmbeddingModelOptions == nil || result.RerankerModelOptions == nil || result.MemoryModelOptions == nil {
		t.Fatalf("empty options must be arrays: %#v", result)
	}
	if len(result.EmbeddingModelOptions)+len(result.RerankerModelOptions)+len(result.MemoryModelOptions) != 0 {
		t.Fatalf("PageInit fabricated options: %#v", result)
	}
}

type fakeAdminRepository struct {
	models          map[uint64]ProviderModelCapability
	modelOptions    []ProviderModelOption
	profiles        map[uint64]ContextProfile
	spaces          map[uint64]ContextSpace
	documents       map[uint64]DocumentAdminDTO
	createdProfile  *ContextProfile
	createdDocument *DocumentAdminDTO
	spaceReferenced bool
	agentConflict   bool
}

func (repository *fakeAdminRepository) ListProviderModelOptions(context.Context) ([]ProviderModelOption, error) {
	return repository.modelOptions, nil
}

func (repository *fakeAdminRepository) FindProviderModelCapability(_ context.Context, id uint64) (*ProviderModelCapability, error) {
	model, ok := repository.models[id]
	if !ok {
		return nil, nil
	}
	return &model, nil
}
func (repository *fakeAdminRepository) CreateProfile(_ context.Context, profile ContextProfile) (ContextProfile, error) {
	profile.ID = 1
	repository.createdProfile = &profile
	return profile, nil
}
func (repository *fakeAdminRepository) FindProfile(_ context.Context, id uint64) (*ContextProfile, error) {
	profile, ok := repository.profiles[id]
	if !ok {
		return nil, nil
	}
	return &profile, nil
}
func (repository *fakeAdminRepository) UpdateProfileMetadata(_ context.Context, id uint64, name string, status ProfileStatus) (ContextProfile, error) {
	profile := repository.profiles[id]
	profile.Name, profile.Status = name, status
	repository.profiles[id] = profile
	return profile, nil
}
func (repository *fakeAdminRepository) CompareAndSwapProfileIndex(context.Context, ProfileIndexCAS) (bool, error) {
	return true, nil
}
func (repository *fakeAdminRepository) CreateSpace(_ context.Context, space ContextSpace) (ContextSpace, error) {
	space.ID = 9
	return space, nil
}
func (repository *fakeAdminRepository) FindSpace(_ context.Context, platform string, id uint64) (*ContextSpace, error) {
	space, ok := repository.spaces[id]
	if !ok || space.Platform != platform {
		return nil, nil
	}
	return &space, nil
}
func (repository *fakeAdminRepository) SpaceHasReferences(context.Context, uint64) (bool, error) {
	return repository.spaceReferenced, nil
}
func (repository *fakeAdminRepository) UpdateSpace(_ context.Context, space ContextSpace) (ContextSpace, error) {
	repository.spaces[space.ID] = space
	return space, nil
}
func (repository *fakeAdminRepository) SoftDeleteSpace(context.Context, string, uint64) error {
	return nil
}
func (repository *fakeAdminRepository) CreateDocumentWithVersion(_ context.Context, document ContextDocument, version ContextDocumentVersion) (DocumentAdminDTO, error) {
	document.ID, version.ID, version.DocumentID = 5, 6, 5
	if repository.documents == nil {
		repository.documents = make(map[uint64]DocumentAdminDTO)
	}
	result := documentAdminDTO(document, version)
	repository.createdDocument = &result
	repository.documents[5] = result
	return result, nil
}
func (repository *fakeAdminRepository) FindDocument(_ context.Context, _ string, id uint64) (*DocumentAdminDTO, error) {
	value, ok := repository.documents[id]
	if !ok {
		return nil, nil
	}
	return &value, nil
}
func (repository *fakeAdminRepository) CreateDocumentVersion(_ context.Context, version ContextDocumentVersion) (DocumentAdminDTO, error) {
	version.ID = 7
	result := repository.documents[version.DocumentID]
	result.Version = documentVersionDTO(version)
	return result, nil
}
func (repository *fakeAdminRepository) AgentProfileChangeConflict(context.Context, uint64, uint64) (bool, error) {
	return repository.agentConflict, nil
}
func (repository *fakeAdminRepository) ListProfiles(_ context.Context, status ProfileStatus) ([]ContextProfile, error) {
	var result []ContextProfile
	for _, item := range repository.profiles {
		if status == "" || item.Status == status {
			result = append(result, item)
		}
	}
	return result, nil
}
func (repository *fakeAdminRepository) ListSpaces(_ context.Context, platform string, profileID uint64, status string) ([]ContextSpace, error) {
	var result []ContextSpace
	for _, item := range repository.spaces {
		if item.Platform == platform && (profileID == 0 || item.ProfileID == profileID) && (status == "" || item.Status == status) {
			result = append(result, item)
		}
	}
	return result, nil
}
func (repository *fakeAdminRepository) ListDocuments(context.Context, string, uint64, string) ([]DocumentAdminDTO, error) {
	return nil, nil
}
func (repository *fakeAdminRepository) ListDocumentVersions(context.Context, string, uint64) ([]DocumentVersionDTO, error) {
	return nil, nil
}
func (repository *fakeAdminRepository) UpdateDocumentStatus(_ context.Context, _ string, id uint64, status string) (DocumentAdminDTO, error) {
	item := repository.documents[id]
	item.Status = status
	repository.documents[id] = item
	return item, nil
}
func (repository *fakeAdminRepository) SoftDeleteDocument(context.Context, string, uint64) error {
	return nil
}
func (repository *fakeAdminRepository) GetAgentContextProfile(context.Context, uint64) (*uint64, error) {
	return nil, nil
}
func (repository *fakeAdminRepository) SetAgentContextProfile(context.Context, uint64, *uint64) error {
	return nil
}
func (repository *fakeAdminRepository) ListAgentContextSpaces(context.Context, uint64) ([]uint64, error) {
	return nil, nil
}
func (repository *fakeAdminRepository) ReplaceAgentContextSpaces(context.Context, uint64, []uint64) error {
	return nil
}

type fakeConditionalReader struct {
	metadata storage.ConditionalObjectMetadata
}

func (reader *fakeConditionalReader) Head(context.Context, storage.ConditionalObjectInput) (storage.ConditionalObjectMetadata, error) {
	return reader.metadata, nil
}
func (reader *fakeConditionalReader) Open(context.Context, storage.ConditionalObjectInput) (io.ReadCloser, storage.ConditionalObjectMetadata, error) {
	return nil, storage.ConditionalObjectMetadata{}, errors.New("unused")
}

type failingVersionEnqueuer struct{}

func (failingVersionEnqueuer) EnqueueDocumentVersion(context.Context, uint64) error {
	return errors.New("queue down")
}

type recordingProfileRebuildEnqueuer struct{ profiles []ContextProfile }

func (enqueuer *recordingProfileRebuildEnqueuer) EnqueueProfileRebuild(_ context.Context, profile ContextProfile) error {
	enqueuer.profiles = append(enqueuer.profiles, profile)
	return nil
}

func readyProfile(id uint64) ContextProfile {
	generation := uint64(1)
	return ContextProfile{ID: id, Name: "p", Status: ProfileEnabled, IndexState: ProfileIndexReady, ActiveIndexGeneration: &generation}
}
