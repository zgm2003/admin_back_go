package aimessage

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/database"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/module/ai/officialmodel"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/module/ai/requestidentity"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/uploadpolicy"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

type fakeHistoryRepository struct {
	revisionInput     EditInput
	regenerationInput RegenerateInput
	deleteInput       DeleteInput
	result            replycommand.CreateReplyResult
	deletedIDs        []int64
	replayed          bool
	err               error
	prepareInput      HistoryPrepareInput
	preparation       HistoryActionPreparation
	prepareErr        error
}

func (f *fakeHistoryRepository) PrepareAction(_ context.Context, input HistoryPrepareInput) (HistoryActionPreparation, error) {
	f.prepareInput = input
	return f.preparation, f.prepareErr
}

func (f *fakeHistoryRepository) Revise(_ context.Context, input EditInput) (HistoryAccepted, error) {
	f.revisionInput = input
	return HistoryAccepted{Reply: f.result, Replayed: f.replayed}, f.err
}

func (f *fakeHistoryRepository) Regenerate(_ context.Context, input RegenerateInput) (HistoryAccepted, error) {
	f.regenerationInput = input
	return HistoryAccepted{Reply: f.result, Replayed: f.replayed}, f.err
}

func TestHistoryAttachmentSelectionSemantics(t *testing.T) {
	source := Attachment{Type: "file", ObjectKey: "ai_chat_attachments/2026/07/source.pdf", Name: "source.pdf"}
	replacement := Attachment{Type: "file", ObjectKey: "ai_chat_attachments/2026/07/replacement.md", Name: "replacement.md"}
	metadata := map[string]storagecos.ObjectMetadata{
		source.ObjectKey: {
			Key: source.ObjectKey, MIMEType: "application/pdf", Size: 1024, ETag: `"source-v1"`, TrustedURL: "https://trusted.test/source.pdf",
		},
		replacement.ObjectKey: {
			Key: replacement.ObjectKey, MIMEType: "text/markdown", Size: 2048, ETag: `"replacement-v1"`, TrustedURL: "https://trusted.test/replacement.md",
		},
	}
	sourceDigest, err := historyAttachmentsDigest([]Attachment{source})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name          string
		operation     string
		attachments   *[]Attachment
		wantKeys      []string
		wantValidated []Attachment
	}{
		{name: "revision omitted keeps source", operation: HistoryOperationRevision, wantKeys: []string{source.ObjectKey}, wantValidated: []Attachment{{
			Type: "file", ObjectKey: source.ObjectKey, MIMEType: "application/pdf", URL: "https://trusted.test/source.pdf", Name: "source.pdf", Size: 1024, ETag: `"source-v1"`,
		}}},
		{name: "revision empty removes all", operation: HistoryOperationRevision, attachments: attachmentSlicePointer([]Attachment{}), wantKeys: []string{}, wantValidated: []Attachment{}},
		{name: "revision explicit replaces source", operation: HistoryOperationRevision, attachments: attachmentSlicePointer([]Attachment{replacement}), wantKeys: []string{replacement.ObjectKey}, wantValidated: []Attachment{{
			Type: "file", ObjectKey: replacement.ObjectKey, MIMEType: "text/markdown", URL: "https://trusted.test/replacement.md", Name: "replacement.md", Size: 2048, ETag: `"replacement-v1"`,
		}}},
		{name: "regeneration reuses source", operation: HistoryOperationRegeneration, wantKeys: []string{source.ObjectKey}, wantValidated: []Attachment{{
			Type: "file", ObjectKey: source.ObjectKey, MIMEType: "application/pdf", URL: "https://trusted.test/source.pdf", Name: "source.pdf", Size: 1024, ETag: `"source-v1"`,
		}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			history := &fakeHistoryRepository{
				preparation: HistoryActionPreparation{
					Runtime: validFileMessageAgent(), SourceAttachments: []Attachment{source}, SourceAttachmentsSHA256: sourceDigest,
				},
				result: replycommand.CreateReplyResult{UserMessageID: 12, CommandID: 99, RequestID: "history-rid", State: replycommand.StatePending},
			}
			inspector := &fakeMessageObjectInspector{metadata: metadata}
			service := newHistoryAttachmentTestService(history, inspector)
			if test.operation == HistoryOperationRevision {
				_, appErr := service.Revise(context.Background(), 7, EditInput{
					ConversationID: 3, MessageID: 41, Content: "new text", RequestID: "history-rid", Attachments: test.attachments,
				})
				if appErr != nil {
					t.Fatal(appErr)
				}
				if !reflect.DeepEqual(history.revisionInput.ValidatedAttachments, test.wantValidated) || history.revisionInput.SourceAttachmentsSHA256 != sourceDigest ||
					(history.revisionInput.UploadRuleToken == (uploadpolicy.ConsistencyToken{})) != (len(test.wantKeys) == 0) {
					t.Fatalf("revision input=%#v", history.revisionInput)
				}
			} else {
				_, appErr := service.Regenerate(context.Background(), 7, RegenerateInput{
					ConversationID: 3, AssistantMessageID: 97, RequestID: "history-rid",
				})
				if appErr != nil {
					t.Fatal(appErr)
				}
				if !reflect.DeepEqual(history.regenerationInput.ValidatedAttachments, test.wantValidated) || history.regenerationInput.SourceAttachmentsSHA256 != sourceDigest ||
					history.regenerationInput.UploadRuleToken == (uploadpolicy.ConsistencyToken{}) {
					t.Fatalf("regeneration input=%#v", history.regenerationInput)
				}
			}
			if len(inspector.calls) != len(test.wantKeys) || len(test.wantKeys) > 0 && !reflect.DeepEqual(inspector.calls, test.wantKeys) {
				t.Fatalf("HEAD calls=%v want=%v", inspector.calls, test.wantKeys)
			}
		})
	}
}

func TestPrepareHistoryActionReadsCanonicalSourceWithoutMutation(t *testing.T) {
	db, mock, cleanup := newMessageMockDB(t)
	defer cleanup()
	repository := &GormRepository{db: db}
	meta := `{"attachments":[{"type":"file","object_key":"ai_chat_attachments/2026/07/report.pdf","mime_type":"application/pdf","url":"https://trusted.test/report.pdf","name":"report.pdf","size":4096,"etag":"\"v1\""}]}`

	expectHistoryRuntime(mock, false)
	expectHistoryMessage(mock, 41, enum.AIMessageRoleUser, "summarize", meta, enum.CommonNo)
	preparation, err := repository.PrepareAction(context.Background(), HistoryPrepareInput{
		Operation: HistoryOperationRevision, UserID: 7, ConversationID: 3, SourceMessageID: 41,
	})
	if err != nil {
		t.Fatal(err)
	}
	if preparation.Runtime.APIProtocol != aiprovider.APIProtocolResponses || len(preparation.SourceAttachments) != 1 ||
		preparation.SourceAttachments[0].ETag != `"v1"` || preparation.SourceAttachmentsSHA256 == ([32]byte{}) {
		t.Fatalf("preparation=%#v", preparation)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("prepare action mutated paid facts: %v", err)
	}
	mock.ExpectClose()
}

func TestHistoryAttachmentETagChangeStopsBeforeRepositoryMutation(t *testing.T) {
	source := Attachment{
		Type: "file", ObjectKey: "ai_chat_attachments/2026/07/report.pdf", MIMEType: "application/pdf",
		URL: "https://trusted.test/report.pdf", Name: "report.pdf", Size: 4096, ETag: `"old"`,
	}
	digest, err := historyAttachmentsDigest([]Attachment{source})
	if err != nil {
		t.Fatal(err)
	}
	history := &fakeHistoryRepository{preparation: HistoryActionPreparation{
		Runtime: validFileMessageAgent(), SourceAttachments: []Attachment{source}, SourceAttachmentsSHA256: digest,
	}}
	inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{
		source.ObjectKey: {
			Key: source.ObjectKey, MIMEType: "application/pdf", Size: 4096, ETag: `"new"`, TrustedURL: "https://trusted.test/report.pdf",
		},
	}}
	_, appErr := newHistoryAttachmentTestService(history, inspector).Regenerate(context.Background(), 7, RegenerateInput{
		ConversationID: 3, AssistantMessageID: 97, RequestID: "regen-etag-change",
	})
	if appErr == nil || history.regenerationInput.RequestID != "" {
		t.Fatalf("etag change error=%#v mutation=%#v", appErr, history.regenerationInput)
	}
}

func TestHistoryMissingAttachmentStopsBeforeRepositoryMutation(t *testing.T) {
	source := Attachment{
		Type: "file", ObjectKey: "ai_chat_attachments/2026/07/deleted.pdf", MIMEType: "application/pdf",
		URL: "https://trusted.test/deleted.pdf", Name: "deleted.pdf", Size: 4096, ETag: `"old"`,
	}
	digest, err := historyAttachmentsDigest([]Attachment{source})
	if err != nil {
		t.Fatal(err)
	}
	history := &fakeHistoryRepository{preparation: HistoryActionPreparation{
		Runtime: validFileMessageAgent(), SourceAttachments: []Attachment{source}, SourceAttachmentsSHA256: digest,
	}}
	inspector := &fakeMessageObjectInspector{err: errors.New("object no longer exists")}

	_, appErr := newHistoryAttachmentTestService(history, inspector).Regenerate(context.Background(), 7, RegenerateInput{
		ConversationID: 3, AssistantMessageID: 97, RequestID: "regen-deleted-object",
	})

	if appErr == nil || appErr.MessageID != "aimessage.attachments.invalid" || history.regenerationInput.RequestID != "" {
		t.Fatalf("missing object error=%#v mutation=%#v", appErr, history.regenerationInput)
	}
	if !reflect.DeepEqual(inspector.calls, []string{source.ObjectKey}) {
		t.Fatalf("missing object HEAD calls=%v", inspector.calls)
	}
}

func TestHistoryAttachmentCapabilityFailureStopsBeforeInspectionOrMutation(t *testing.T) {
	source := Attachment{Type: "file", ObjectKey: "ai_chat_attachments/2026/07/report.pdf", Name: "report.pdf"}
	digest, err := historyAttachmentsDigest([]Attachment{source})
	if err != nil {
		t.Fatal(err)
	}
	history := &fakeHistoryRepository{preparation: HistoryActionPreparation{
		Runtime: validFileMessageAgent(), SourceAttachments: []Attachment{source}, SourceAttachmentsSHA256: digest,
	}}
	inspector := &fakeMessageObjectInspector{}
	service := NewService(
		&fakeRepository{},
		WithHistoryRepository(history),
		WithPricingResolver(officialmodel.ResolverFunc(func(context.Context, string) (officialmodel.ResolvedModel, error) {
			return officialmodel.ResolvedModel{}, errors.New("official model catalog unavailable")
		})),
		WithTransportCapabilityResolver(testMessageTransportCapabilities()),
		WithObjectInspector(inspector),
		WithUploadRuleResolver(testMessageUploadRuleResolver()),
	)

	_, appErr := service.Regenerate(context.Background(), 7, RegenerateInput{
		ConversationID: 3, AssistantMessageID: 97, RequestID: "regen-runtime-unavailable",
	})

	if appErr == nil || appErr.HTTPStatus != 409 || appErr.Code != "ai.message.history_runtime_unavailable" ||
		appErr.Category != "conflict" || appErr.MessageID != "aimessage.history.runtime_unavailable" {
		t.Fatalf("runtime capability error=%#v", appErr)
	}
	if len(inspector.calls) != 0 {
		t.Fatalf("runtime capability failure reached object inspection: %v", inspector.calls)
	}
	if history.regenerationInput.RequestID != "" {
		t.Fatalf("runtime capability failure reached history mutation: %#v", history.regenerationInput)
	}
}

func TestHistoryRegenerationRevalidatesLegacyImageNamespace(t *testing.T) {
	source := Attachment{
		Type: "image", ObjectKey: "ai_chat_images/2026/07/legacy.png", MIMEType: "image/png",
		URL: "https://trusted.test/legacy.png", Name: "legacy.png", Size: 1024,
	}
	digest, err := historyAttachmentsDigest([]Attachment{source})
	if err != nil {
		t.Fatal(err)
	}
	history := &fakeHistoryRepository{
		preparation: HistoryActionPreparation{
			Runtime: *validMessageAgent(), SourceAttachments: []Attachment{source}, SourceAttachmentsSHA256: digest,
		},
		result: replycommand.CreateReplyResult{UserMessageID: 71, CommandID: 81, RequestID: "regen-legacy", State: replycommand.StatePending},
	}
	inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{
		source.ObjectKey: {
			Key: source.ObjectKey, MIMEType: "image/png", Size: 1024, ETag: `"legacy-v1"`, TrustedURL: "https://trusted.test/legacy.png",
		},
	}}

	_, appErr := newHistoryAttachmentTestService(history, inspector).Regenerate(context.Background(), 7, RegenerateInput{
		ConversationID: 3, AssistantMessageID: 97, RequestID: "regen-legacy",
	})

	if appErr != nil {
		t.Fatal(appErr)
	}
	if len(history.regenerationInput.ValidatedAttachments) != 1 || history.regenerationInput.ValidatedAttachments[0].ETag != `"legacy-v1"` ||
		history.regenerationInput.UploadRuleToken == (uploadpolicy.ConsistencyToken{}) {
		t.Fatalf("legacy image regeneration=%#v", history.regenerationInput)
	}
}

func TestHistoryAttachmentsRequireUploadRuleConsistencyTokenBeforeMutation(t *testing.T) {
	source := Attachment{Type: "file", ObjectKey: "ai_chat_attachments/2026/07/report.pdf", Name: "report.pdf"}
	digest, err := historyAttachmentsDigest([]Attachment{source})
	if err != nil {
		t.Fatal(err)
	}
	history := &fakeHistoryRepository{preparation: HistoryActionPreparation{
		Runtime: validFileMessageAgent(), SourceAttachments: []Attachment{source}, SourceAttachmentsSHA256: digest,
	}}
	inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{
		source.ObjectKey: {Key: source.ObjectKey, MIMEType: "application/pdf", Size: 4096, ETag: `"v1"`, TrustedURL: "https://trusted.test/report.pdf"},
	}}
	capabilities := officialmodel.Capabilities{
		InputModalities: []string{"text", "image", "file"}, OutputModalities: []string{"text"}, SupportsStreaming: true, NativeFileInput: true,
	}
	service := NewService(
		&fakeRepository{},
		WithHistoryRepository(history),
		WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
		WithTransportCapabilityResolver(staticTransportCapabilityResolver{ok: true, metadata: infraai.CapabilityMetadata{
			InputModalities: []string{"text", "image", "file"}, OutputModalities: []string{"text"}, SupportsStreaming: true,
		}}),
		WithObjectInspector(inspector),
		WithUploadRuleResolver(uploadpolicy.ResolverFunc(func(context.Context) (uploadpolicy.Rule, error) {
			return uploadpolicy.Rule{MaxFileBytes: 100 << 20, FileExtensions: []string{"pdf"}}, nil
		})),
	)

	_, appErr := service.Regenerate(context.Background(), 7, RegenerateInput{
		ConversationID: 3, AssistantMessageID: 97, RequestID: "regen-token-missing",
	})

	if appErr == nil || appErr.MessageID != "aimessage.attachments.upload_rule_unavailable" || history.regenerationInput.RequestID != "" {
		t.Fatalf("missing rule token error=%#v mutation=%#v", appErr, history.regenerationInput)
	}
}

func attachmentSlicePointer(value []Attachment) *[]Attachment { return &value }

func validFileMessageAgent() AgentRuntime {
	agent := *validMessageAgent()
	agent.ModelID, agent.OfficialModelID = "gpt-5.6", "gpt-5.6"
	agent.APIProtocol = aiprovider.APIProtocolResponses
	return agent
}

func newHistoryAttachmentTestService(history HistoryRepository, inspector storagecos.ObjectInspector) *Service {
	capabilities := officialmodel.Capabilities{
		InputModalities: []string{"text", "image", "file"}, OutputModalities: []string{"text"},
		SupportsStreaming: true, NativeFileInput: true,
		ImageInput: &officialmodel.ImageInputCapability{MIMETypes: []string{"image/png"}, MaxFiles: 5, MaxBytes: 10 << 20},
	}
	return NewService(
		&fakeRepository{},
		WithHistoryRepository(history),
		WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
		WithTransportCapabilityResolver(staticTransportCapabilityResolver{ok: true, metadata: infraai.CapabilityMetadata{
			InputModalities: []string{"text", "image", "file"}, OutputModalities: []string{"text"}, SupportsStreaming: true,
		}}),
		WithObjectInspector(inspector),
		WithUploadRuleResolver(testMessageUploadRuleResolver()),
	)
}

func (f *fakeHistoryRepository) DeleteMessages(_ context.Context, input DeleteInput) ([]int64, error) {
	f.deleteInput = input
	return append([]int64(nil), f.deletedIDs...), f.err
}

func TestHistoryRevisionCommitsBeforeRunnerWakeAndReturnsSendIdentity(t *testing.T) {
	history := &fakeHistoryRepository{result: replycommand.CreateReplyResult{
		UserMessageID: 71, CommandID: 81, RequestID: "revision-1", State: replycommand.StatePending,
	}}
	waker := &fakeReplyWaker{}
	service := NewService(&fakeRepository{}, WithHistoryRepository(history), WithReplyWaker(waker))

	result, appErr := service.Revise(context.Background(), 7, EditInput{
		ConversationID: 3, MessageID: 41, Content: "  revised text  ", RequestID: " revision-1 ",
	})
	if appErr != nil {
		t.Fatalf("Revise returned error: %v", appErr)
	}
	if history.revisionInput.UserID != 7 || history.revisionInput.Content != "revised text" || history.revisionInput.RequestID != "revision-1" {
		t.Fatalf("normalized revision=%+v", history.revisionInput)
	}
	if result.ConversationID != 3 || result.UserMessageID != 71 || result.CommandID != 81 || result.RequestID != "revision-1" || result.State != replycommand.StatePending {
		t.Fatalf("result=%+v", result)
	}
	if waker.commandID != 81 {
		t.Fatalf("woke command=%d", waker.commandID)
	}
}

func TestHistoryRegenerationUsesAssistantSourceAndDoesNotWakeAfterRollback(t *testing.T) {
	history := &fakeHistoryRepository{err: errors.New("transaction rolled back")}
	waker := &fakeReplyWaker{}
	service := NewService(&fakeRepository{}, WithHistoryRepository(history), WithReplyWaker(waker))

	_, appErr := service.Regenerate(context.Background(), 7, RegenerateInput{ConversationID: 3, AssistantMessageID: 97, RequestID: "regen-1"})
	if appErr == nil || appErr.HTTPStatus != 500 {
		t.Fatalf("expected repository failure, got %#v", appErr)
	}
	if history.regenerationInput.UserID != 7 || history.regenerationInput.AssistantMessageID != 97 {
		t.Fatalf("regeneration input=%+v", history.regenerationInput)
	}
	if waker.commandID != 0 {
		t.Fatalf("rolled-back command was woken: %d", waker.commandID)
	}
}

func TestHistoryCanonicalReplayReturnsOriginalWithoutRunnerWake(t *testing.T) {
	history := &fakeHistoryRepository{replayed: true, result: replycommand.CreateReplyResult{
		UserMessageID: 71, CommandID: 81, RequestID: "revision-1", State: replycommand.StatePending,
	}}
	waker := &fakeReplyWaker{}
	result, appErr := NewService(&fakeRepository{}, WithHistoryRepository(history), WithReplyWaker(waker)).Revise(
		context.Background(), 7, EditInput{ConversationID: 3, MessageID: 41, Content: "changed", RequestID: "revision-1"},
	)
	if appErr != nil || result == nil || result.CommandID != 81 {
		t.Fatalf("replay result=%+v error=%v", result, appErr)
	}
	if waker.commandID != 0 {
		t.Fatalf("canonical replay woke command %d", waker.commandID)
	}
}

func TestHistoryActionsMapActiveAndHiddenSourceErrors(t *testing.T) {
	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "active", err: ErrHistoryActiveCommand, wantStatus: 409, wantCode: ErrorCodeHistoryActive},
		{name: "hidden source", err: ErrHistorySourceNotFound, wantStatus: 404, wantCode: "resource.not_found"},
		{name: "source drift", err: ErrHistorySourceChanged, wantStatus: 409, wantCode: "ai.message.history_source_changed"},
		{name: "runtime drift", err: ErrHistoryRuntimeChanged, wantStatus: 409, wantCode: "ai.message.history_acceptance_changed"},
		{name: "upload rule drift", err: ErrHistoryUploadRuleChanged, wantStatus: 409, wantCode: "ai.message.history_acceptance_changed"},
		{name: "runtime unavailable", err: ErrHistoryAgentUnavailable, wantStatus: 409, wantCode: "ai.message.history_runtime_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			history := &fakeHistoryRepository{err: test.err}
			_, appErr := NewService(&fakeRepository{}, WithHistoryRepository(history)).Revise(context.Background(), 7, EditInput{
				ConversationID: 3, MessageID: 41, Content: "changed", RequestID: "revision-1",
			})
			if appErr == nil || appErr.HTTPStatus != test.wantStatus || appErr.Code != test.wantCode {
				t.Fatalf("error=%#v", appErr)
			}
			if test.err == ErrHistoryAgentUnavailable && (appErr.Category != "conflict" || appErr.MessageID != "aimessage.history.runtime_unavailable") {
				t.Fatalf("runtime unavailable error=%#v", appErr)
			}
		})
	}
}

func TestDeleteMessagesRejectsDuplicatesAndReturnsSortedExactIDs(t *testing.T) {
	history := &fakeHistoryRepository{deletedIDs: []int64{97, 41, 63}}
	service := NewService(&fakeRepository{}, WithHistoryRepository(history))

	if _, appErr := service.DeleteMessages(context.Background(), 7, DeleteInput{ConversationID: 3, IDs: []int64{41, 41}}); appErr == nil || appErr.HTTPStatus != 400 {
		t.Fatalf("duplicate IDs error=%#v", appErr)
	}
	result, appErr := service.DeleteMessages(context.Background(), 7, DeleteInput{ConversationID: 3, IDs: []int64{97, 41, 63}})
	if appErr != nil {
		t.Fatalf("DeleteMessages: %v", appErr)
	}
	if len(result.DeletedIDs) != 3 || result.DeletedIDs[0] != 41 || result.DeletedIDs[1] != 63 || result.DeletedIDs[2] != 97 {
		t.Fatalf("deleted ids=%v", result.DeletedIDs)
	}
}

type fakeHistoryParticipant struct {
	replayResult *replycommand.CreateReplyResult
	replayErr    error
	createResult replycommand.CreateReplyResult
	createErr    error
	created      replycommand.HistoryCreateInput
}

type fakeHistoryUploadRuleGuard struct {
	err   error
	token uploadpolicy.ConsistencyToken
	tx    *gorm.DB
	calls int
}

func (guard *fakeHistoryUploadRuleGuard) GuardActiveInTransaction(_ context.Context, tx *gorm.DB, token uploadpolicy.ConsistencyToken) error {
	guard.calls++
	guard.tx = tx
	guard.token = token
	return guard.err
}

func (f *fakeHistoryParticipant) ReplayInTransaction(_ context.Context, _ *gorm.DB, _ replycommand.HistoryRequest) (*replycommand.CreateReplyResult, error) {
	return f.replayResult, f.replayErr
}

func (f *fakeHistoryParticipant) CreateInTransaction(_ context.Context, _ *gorm.DB, input replycommand.HistoryCreateInput) (replycommand.CreateReplyResult, error) {
	f.created = input
	return f.createResult, f.createErr
}

func TestHistoryCanonicalReplayRunsBeforeCurrentRuntimeOrSourceReads(t *testing.T) {
	db, mock, cleanup := newMessageMockDB(t)
	defer cleanup()
	participant := &fakeHistoryParticipant{replayResult: &replycommand.CreateReplyResult{
		UserMessageID: 71, CommandID: 81, RunID: 91, ChargeID: 101, RequestID: "revision-1", State: replycommand.StatePending,
	}}
	repository := &GormRepository{db: db, history: participant, pricing: testMessagePricingResolver()}

	mock.ExpectBegin()
	mock.ExpectCommit()
	accepted, err := repository.Revise(context.Background(), EditInput{
		UserID: 7, ConversationID: 3, MessageID: 41, Content: "new text", RequestID: "revision-1",
	})
	if err != nil || !accepted.Replayed || accepted.Reply.CommandID != 81 || accepted.Reply.RunID != 91 || accepted.Reply.ChargeID != 101 {
		t.Fatalf("canonical replay=%+v err=%v", accepted, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("canonical replay must precede current runtime/source reads: %v", err)
	}
	mock.ExpectClose()
}

func TestHistoryCanonicalReplaySurvivesDisabledRuntimeAndHiddenSource(t *testing.T) {
	db, mock, cleanup := newMessageMockDB(t)
	defer cleanup()
	replyRepository := replycommand.NewGormRepository(&database.Client{Gorm: db})
	repository := &GormRepository{db: db, history: replycommand.NewHistoryParticipant(replyRepository), pricing: testMessagePricingResolver()}
	runtime := AgentRuntime{AgentID: 5, ModelID: "gpt-4.1-mini"}
	source := historySourceSnapshot{target: Message{ID: 41}, user: Message{ID: 41, Content: "old text"}}
	facts, err := buildHistoryRequestFacts(HistoryOperationRevision, 7, 3, "new text", source, runtime, 4096, nil)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := requestidentity.BuildFingerprint(facts.identity)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").WithArgs(int64(7), "revision-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "request_fingerprint", "request_identity_status", "user_message_id", "state"}).
			AddRow(81, "revision-1", fingerprint[:], requestidentity.IdentityStatusReplayable, 71, replycommand.StatePending))
	mock.ExpectQuery("SELECT ai_agents.id AS agent_id.*FROM `ai_conversations`.*JOIN ai_agents").
		WithArgs(int64(3), int64(7), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"agent_id", "model_id"}).AddRow(5, "gpt-4.1-mini"))
	expectHistoryMessage(mock, 41, enum.AIMessageRoleUser, "old text", "", enum.CommonYes)
	mock.ExpectQuery("SELECT `id` FROM `ai_runs`").WithArgs(int64(7), "revision-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(91))
	mock.ExpectQuery("SELECT `id` FROM `ai_usage_charges`").WithArgs(int64(91), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(101))
	mock.ExpectCommit()

	accepted, err := repository.Revise(context.Background(), EditInput{
		UserID: 7, ConversationID: 3, MessageID: 41, Content: "new text", RequestID: "revision-1",
	})
	if err != nil || !accepted.Replayed || accepted.Reply.CommandID != 81 || accepted.Reply.RunID != 91 || accepted.Reply.ChargeID != 101 {
		t.Fatalf("canonical replay=%+v err=%v", accepted, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("disabled runtime or hidden source blocked canonical replay: %v", err)
	}
	mock.ExpectClose()
}

func TestHistoryRevisionRollsBackVisibleTailWhenParticipantFails(t *testing.T) {
	db, mock, cleanup := newMessageMockDB(t)
	defer cleanup()
	participant := &fakeHistoryParticipant{createErr: errors.New("run insert failed")}
	guard := &fakeHistoryUploadRuleGuard{}
	repository := &GormRepository{db: db, history: participant, pricing: testMessagePricingResolver(), uploadRuleGuard: guard}
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	repository.now = func() time.Time { return now }

	mock.ExpectBegin()
	expectNoActiveHistoryCommand(mock, false)
	expectOwnedConversationLock(mock)
	expectNoActiveHistoryCommand(mock, true)
	expectHistoryRuntime(mock, true)
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(id\\), 0\\).*FROM `ai_messages`").WillReturnRows(sqlmock.NewRows([]string{"max_id"}).AddRow(97))
	expectHistoryMessage(mock, 41, enum.AIMessageRoleUser, "old text", `{"attachments":[{"type":"image","object_key":"ai_chat_images/2026/07/28/a.png","mime_type":"image/png","url":"https://trusted.test/a.png","name":"a.png","size":10}]}`, enum.CommonNo)
	mock.ExpectExec("UPDATE `ai_messages` SET .*`is_del`=\\?.*conversation_id = \\? AND is_del = \\? AND id >= \\? AND id <= \\?").
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectRollback()

	sourceAttachments := []Attachment{{
		Type: "image", ObjectKey: "ai_chat_images/2026/07/28/a.png", MIMEType: "image/png",
		URL: "https://trusted.test/a.png", Name: "a.png", Size: 10,
	}}
	sourceDigest, digestErr := historyAttachmentsDigest(sourceAttachments)
	if digestErr != nil {
		t.Fatal(digestErr)
	}
	validatedAttachments := append([]Attachment(nil), sourceAttachments...)
	validatedAttachments[0].ETag = `"image-v1"`
	runtimeDigest := mustHistoryRuntimeDigest(t)
	_, err := repository.Revise(context.Background(), EditInput{
		UserID: 7, ConversationID: 3, MessageID: 41, Content: "new text", RequestID: "revision-1",
		ValidatedAttachments: validatedAttachments, SourceAttachmentsSHA256: sourceDigest, SourceRuntimeSHA256: runtimeDigest,
		UploadRuleToken: uploadpolicy.ConsistencyToken{1},
	})
	if err == nil || err.Error() != "run insert failed" {
		t.Fatalf("revision error=%v", err)
	}
	if participant.created.MetaJSON == nil || *participant.created.MetaJSON == "" || participant.created.Content != "new text" {
		t.Fatalf("edit did not inherit server metadata: %+v", participant.created)
	}
	if guard.calls != 1 || guard.tx == nil {
		t.Fatalf("upload rule guard calls=%d tx=%p", guard.calls, guard.tx)
	}
	var snapshot struct {
		RequestIdentity struct {
			Operation       string `json:"operation"`
			SourceMessageID int64  `json:"source_message_id"`
		} `json:"request_identity"`
	}
	if err := json.Unmarshal([]byte(participant.created.InputSnapshot), &snapshot); err != nil {
		t.Fatalf("decode history input snapshot: %v", err)
	}
	if snapshot.RequestIdentity.Operation != HistoryOperationRevision || snapshot.RequestIdentity.SourceMessageID != 41 {
		t.Fatalf("history request identity snapshot=%+v", snapshot.RequestIdentity)
	}
	if participant.created.Identity.Operation != HistoryOperationRevision || participant.created.Identity.SourceMessageID != 41 {
		t.Fatalf("typed identity=%+v", participant.created.Identity)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("revision transaction: %v", err)
	}
	mock.ExpectClose()
}

func TestHistoryRevisionRejectsSourceAttachmentDriftBeforeMutation(t *testing.T) {
	db, mock, cleanup := newMessageMockDB(t)
	defer cleanup()
	repository := &GormRepository{db: db, history: &fakeHistoryParticipant{}, pricing: testMessagePricingResolver()}

	mock.ExpectBegin()
	expectNoActiveHistoryCommand(mock, false)
	expectOwnedConversationLock(mock)
	expectNoActiveHistoryCommand(mock, true)
	expectHistoryRuntime(mock, true)
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(id\\), 0\\).*FROM `ai_messages`").WillReturnRows(sqlmock.NewRows([]string{"max_id"}).AddRow(41))
	expectHistoryMessage(mock, 41, enum.AIMessageRoleUser, "old text", "", enum.CommonNo)
	mock.ExpectRollback()

	_, err := repository.Revise(context.Background(), EditInput{
		UserID: 7, ConversationID: 3, MessageID: 41, Content: "new text", RequestID: "revision-drift",
		ValidatedAttachments: []Attachment{}, SourceAttachmentsSHA256: [32]byte{1}, SourceRuntimeSHA256: mustHistoryRuntimeDigest(t),
	})
	if !errors.Is(err, ErrHistorySourceChanged) {
		t.Fatalf("source drift error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("source drift reached history mutation: %v", err)
	}
	mock.ExpectClose()
}

func TestHistoryRevisionRejectsRuntimeDriftBeforeMutation(t *testing.T) {
	db, mock, cleanup := newMessageMockDB(t)
	defer cleanup()
	repository := &GormRepository{db: db, history: &fakeHistoryParticipant{}, pricing: testMessagePricingResolver()}
	expectedRuntime := historyRuntimeFixture()
	expectedDigest, err := historyRuntimeDigest(expectedRuntime)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	expectNoActiveHistoryCommand(mock, false)
	expectOwnedConversationLock(mock)
	expectNoActiveHistoryCommand(mock, true)
	mock.ExpectQuery("SELECT .*api_protocol.* FROM .*ai_conversations.*ai_agents.*ai_providers.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{
			"agent_id", "provider_id", "model_id", "model_display_name", "engine_type", "api_protocol", "billing_multiplier_ppm", "status", "scenes_json",
			"provider_model_status", "official_model_id", "official_catalog_version", "mapping_status",
		}).AddRow(5, 9, "gpt-4.1-mini", "GPT-4.1 mini", "openai", aiprovider.APIProtocolChatCompletions, 1_250_000, enum.CommonYes, `["chat"]`,
			enum.CommonYes, "gpt-4.1-mini", "catalog-v3", officialmodel.MappingStatusMapped))
	mock.ExpectRollback()

	_, err = repository.Revise(context.Background(), EditInput{
		UserID: 7, ConversationID: 3, MessageID: 41, Content: "new text", RequestID: "revision-runtime-drift",
		ValidatedAttachments: []Attachment{}, SourceRuntimeSHA256: expectedDigest,
	})
	if !errors.Is(err, ErrHistoryRuntimeChanged) {
		t.Fatalf("runtime drift error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("runtime drift reached history mutation: %v", err)
	}
	mock.ExpectClose()
}

func TestHistoryRevisionRejectsUploadRuleDriftBeforeMutation(t *testing.T) {
	db, mock, cleanup := newMessageMockDB(t)
	defer cleanup()
	guard := &fakeHistoryUploadRuleGuard{err: uploadpolicy.ErrRuleSnapshotChanged}
	participant := &fakeHistoryParticipant{}
	repository := &GormRepository{db: db, history: participant, pricing: testMessagePricingResolver(), uploadRuleGuard: guard}
	sourceAttachments := []Attachment{{
		Type: "file", ObjectKey: "ai_chat_attachments/2026/07/report.pdf", MIMEType: "application/pdf",
		URL: "https://trusted.test/report.pdf", Name: "report.pdf", Size: 4096, ETag: `"v1"`,
	}}
	sourceMetaBytes, err := json.Marshal(map[string]any{"attachments": sourceAttachments})
	if err != nil {
		t.Fatal(err)
	}
	sourceDigest, err := historyAttachmentsDigest(sourceAttachments)
	if err != nil {
		t.Fatal(err)
	}
	token := uploadpolicy.ConsistencyToken{1}

	mock.ExpectBegin()
	expectNoActiveHistoryCommand(mock, false)
	expectOwnedConversationLock(mock)
	expectNoActiveHistoryCommand(mock, true)
	expectHistoryRuntime(mock, true)
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(id\\), 0\\).*FROM `ai_messages`").WillReturnRows(sqlmock.NewRows([]string{"max_id"}).AddRow(41))
	expectHistoryMessage(mock, 41, enum.AIMessageRoleUser, "old text", string(sourceMetaBytes), enum.CommonNo)
	mock.ExpectRollback()

	_, err = repository.Revise(context.Background(), EditInput{
		UserID: 7, ConversationID: 3, MessageID: 41, Content: "new text", RequestID: "revision-rule-drift",
		ValidatedAttachments: sourceAttachments, SourceAttachmentsSHA256: sourceDigest, SourceRuntimeSHA256: mustHistoryRuntimeDigest(t),
		UploadRuleToken: token,
	})
	if !errors.Is(err, ErrHistoryUploadRuleChanged) {
		t.Fatalf("upload rule drift error=%v", err)
	}
	if guard.calls != 1 || guard.tx == nil || guard.token != token {
		t.Fatalf("upload rule guard calls=%d tx=%p token=%x", guard.calls, guard.tx, guard.token)
	}
	if participant.created.RequestID != "" {
		t.Fatalf("upload rule drift reached paid mutation: %#v", participant.created)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("upload rule drift reached history mutation: %v", err)
	}
	mock.ExpectClose()
}

func TestHistoryRevisionRollsBackBeforeMutationWhenPricingResolutionFails(t *testing.T) {
	db, mock, cleanup := newMessageMockDB(t)
	defer cleanup()
	participant := &fakeHistoryParticipant{}
	repository := &GormRepository{
		db: db, history: participant,
		pricing: officialmodel.ResolverFunc(func(context.Context, string) (officialmodel.ResolvedModel, error) {
			return officialmodel.ResolvedModel{}, errors.New("pricing catalog unavailable")
		}),
	}

	mock.ExpectBegin()
	expectNoActiveHistoryCommand(mock, false)
	expectOwnedConversationLock(mock)
	expectNoActiveHistoryCommand(mock, true)
	expectHistoryRuntime(mock, true)
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(id\\), 0\\).*FROM `ai_messages`").WillReturnRows(sqlmock.NewRows([]string{"max_id"}).AddRow(41))
	expectHistoryMessage(mock, 41, enum.AIMessageRoleUser, "old text", "", enum.CommonNo)
	mock.ExpectRollback()

	emptyDigest, err := historyAttachmentsDigest(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.Revise(context.Background(), EditInput{
		UserID: 7, ConversationID: 3, MessageID: 41, Content: "new text", RequestID: "revision-pricing-failure",
		ValidatedAttachments: []Attachment{}, SourceAttachmentsSHA256: emptyDigest, SourceRuntimeSHA256: mustHistoryRuntimeDigest(t),
	})
	if !errors.Is(err, ErrHistoryAgentUnavailable) {
		t.Fatalf("pricing resolution error=%v", err)
	}
	if participant.created.RequestID != "" {
		t.Fatalf("pricing failure reached paid history mutation: %#v", participant.created)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("pricing failure transaction: %v", err)
	}
	mock.ExpectClose()
}

func TestHistoryRegenerationRejectsMissingPairWithoutMutation(t *testing.T) {
	db, mock, cleanup := newMessageMockDB(t)
	defer cleanup()
	repository := &GormRepository{db: db, history: &fakeHistoryParticipant{}, pricing: testMessagePricingResolver()}

	mock.ExpectBegin()
	expectNoActiveHistoryCommand(mock, false)
	expectOwnedConversationLock(mock)
	expectNoActiveHistoryCommand(mock, true)
	expectHistoryRuntime(mock, true)
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(id\\), 0\\).*FROM `ai_messages`").WillReturnRows(sqlmock.NewRows([]string{"max_id"}).AddRow(97))
	expectHistoryMessage(mock, 97, enum.AIMessageRoleAssistant, "answer", "", enum.CommonNo)
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	_, err := repository.Regenerate(context.Background(), RegenerateInput{
		UserID: 7, ConversationID: 3, AssistantMessageID: 97, RequestID: "regen-1", SourceRuntimeSHA256: mustHistoryRuntimeDigest(t),
	})
	if !errors.Is(err, ErrHistorySourceNotFound) {
		t.Fatalf("missing pair error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("missing pair queried after mutation: %v", err)
	}
	mock.ExpectClose()
}

func TestHistoryActiveStoppedReplyRejectsRegenerationAndDelete(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*GormRepository) error
	}{
		{
			name: "regenerate",
			run: func(repository *GormRepository) error {
				_, err := repository.Regenerate(context.Background(), RegenerateInput{
					UserID: 7, ConversationID: 3, AssistantMessageID: 97, RequestID: "regen-1",
				})
				return err
			},
		},
		{
			name: "delete",
			run: func(repository *GormRepository) error {
				_, err := repository.DeleteMessages(context.Background(), DeleteInput{
					UserID: 7, ConversationID: 3, IDs: []int64{97},
				})
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, cleanup := newMessageMockDB(t)
			defer cleanup()
			repository := &GormRepository{db: db, history: &fakeHistoryParticipant{}, pricing: testMessagePricingResolver()}

			mock.ExpectBegin()
			mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`.*state IN.*LIMIT \\?$").
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(61))
			mock.ExpectRollback()

			if err := test.run(repository); !errors.Is(err, ErrHistoryActiveCommand) {
				t.Fatalf("active stopped reply error=%v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			mock.ExpectClose()
		})
	}
}

func TestRegenerateStoppedMessageAfterTerminalCommand(t *testing.T) {
	db, mock, cleanup := newMessageMockDB(t)
	defer cleanup()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	participant := &fakeHistoryParticipant{createResult: replycommand.CreateReplyResult{
		UserMessageID: 101, CommandID: 102, RunID: 103, ChargeID: 104, RequestID: "regen-1", State: replycommand.StatePending,
	}}
	repository := &GormRepository{db: db, history: participant, pricing: testMessagePricingResolver(), now: func() time.Time { return now }}

	mock.ExpectBegin()
	expectNoActiveHistoryCommand(mock, false)
	expectOwnedConversationLock(mock)
	expectNoActiveHistoryCommand(mock, true)
	expectHistoryRuntime(mock, true)
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(id\\), 0\\).*FROM `ai_messages`").
		WillReturnRows(sqlmock.NewRows([]string{"max_id"}).AddRow(97))
	mock.ExpectQuery("SELECT .* FROM `ai_messages`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "conversation_id", "role", "content_type", "content", "reply_command_id", "delivery_state", "is_del", "created_at", "updated_at",
		}).AddRow(97, 3, enum.AIMessageRoleAssistant, "text", "1234", 61, replycommand.DeliveryStateStopped, enum.CommonNo, now.Add(-time.Minute), now.Add(-time.Minute)))
	mock.ExpectQuery("SELECT .* FROM `ai_reply_commands`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "conversation_id", "user_message_id", "assistant_message_id", "state", "finished_at",
		}).AddRow(61, 7, 3, 41, 97, replycommand.StateCanceled, now.Add(-time.Minute)))
	mock.ExpectQuery("SELECT .* FROM `ai_messages`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "conversation_id", "role", "content_type", "content", "is_del", "created_at", "updated_at",
		}).AddRow(41, 3, enum.AIMessageRoleUser, "text", "count", enum.CommonNo, now.Add(-2*time.Minute), now.Add(-2*time.Minute)))
	mock.ExpectExec("UPDATE `ai_messages` SET .*`is_del`=\\?.*conversation_id = \\? AND is_del = \\? AND id >= \\? AND id <= \\?").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE `ai_conversations` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	emptyDigest, digestErr := historyAttachmentsDigest(nil)
	if digestErr != nil {
		t.Fatal(digestErr)
	}
	accepted, err := repository.Regenerate(context.Background(), RegenerateInput{
		UserID: 7, ConversationID: 3, AssistantMessageID: 97, RequestID: "regen-1",
		SourceAttachmentsSHA256: emptyDigest, SourceRuntimeSHA256: mustHistoryRuntimeDigest(t),
	})
	if err != nil || accepted.Reply.CommandID != 102 || accepted.Replayed {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}
	if participant.created.Identity.Operation != HistoryOperationRegeneration || participant.created.Identity.SourceMessageID != 97 || participant.created.Content != "count" {
		t.Fatalf("regeneration input=%+v", participant.created)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()
}

func TestHistoryDeleteSoftDeletesOnlySubmittedIDsAndPreservesAuditTables(t *testing.T) {
	db, mock, cleanup := newMessageMockDB(t)
	defer cleanup()
	repository := &GormRepository{db: db, history: &fakeHistoryParticipant{}, pricing: testMessagePricingResolver()}
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	repository.now = func() time.Time { return now }

	mock.ExpectBegin()
	expectNoActiveHistoryCommand(mock, false)
	expectOwnedConversationLock(mock)
	expectNoActiveHistoryCommand(mock, true)
	mock.ExpectQuery("SELECT .* FROM `ai_messages`.*id IN").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(41).AddRow(97))
	mock.ExpectExec("UPDATE `ai_messages` SET .*`is_del`=\\?.*id IN").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery("SELECT MAX\\(created_at\\).*FROM `ai_messages`").WillReturnRows(sqlmock.NewRows([]string{"last_message_at"}).AddRow(now.Add(-time.Minute)))
	mock.ExpectExec("UPDATE `ai_conversations` SET .*`last_message_at`=\\?.*WHERE id = \\? AND user_id = \\? AND is_del = \\?").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ids, err := repository.DeleteMessages(context.Background(), DeleteInput{UserID: 7, ConversationID: 3, IDs: []int64{97, 41}})
	if err != nil || len(ids) != 2 || ids[0] != 41 || ids[1] != 97 {
		t.Fatalf("deleted=%v err=%v", ids, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("delete must not query or mutate Run/billing/wallet/liked facts: %v", err)
	}
	mock.ExpectClose()
}

func expectOwnedConversationLock(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT .* FROM `ai_conversations`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "agent_id", "is_del"}).AddRow(3, 7, 5, enum.CommonNo))
}

func expectNoActiveHistoryCommand(mock sqlmock.Sqlmock, locked bool) {
	pattern := "SELECT .* FROM `ai_reply_commands`.*state IN.*LIMIT \\?"
	if locked {
		pattern += " FOR UPDATE"
	}
	mock.ExpectQuery(pattern + "$").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

func expectHistoryRuntime(mock sqlmock.Sqlmock, locked bool) {
	runtime := historyRuntimeFixture()
	pattern := "SELECT .*api_protocol.* FROM .*ai_conversations.*ai_agents.*ai_providers.*ai_provider_models.model_kind = \\?"
	if locked {
		pattern += ".*FOR UPDATE"
	}
	mock.ExpectQuery(pattern).WillReturnRows(sqlmock.NewRows([]string{
		"agent_id", "provider_id", "model_id", "model_display_name", "engine_type", "api_protocol", "billing_multiplier_ppm", "status", "scenes_json",
		"provider_model_status", "official_model_id", "official_catalog_version", "mapping_status",
	}).AddRow(runtime.AgentID, runtime.ProviderID, runtime.ModelID, runtime.ModelDisplayName, runtime.EngineType, runtime.APIProtocol,
		runtime.BillingMultiplierPPM, runtime.Status, runtime.ScenesJSON, runtime.ProviderModelStatus, runtime.OfficialModelID,
		runtime.OfficialCatalogVersion, runtime.MappingStatus))
}

func historyRuntimeFixture() AgentRuntime {
	return AgentRuntime{
		AgentID: 5, ProviderID: 9, ModelID: "gpt-4.1-mini", ModelDisplayName: "GPT-4.1 mini", EngineType: "openai",
		APIProtocol: aiprovider.APIProtocolResponses, BillingMultiplierPPM: 1_250_000,
		Status: enum.CommonYes, ScenesJSON: `["chat"]`, ProviderModelStatus: enum.CommonYes,
		OfficialModelID: "gpt-4.1-mini", OfficialCatalogVersion: "catalog-v3", MappingStatus: officialmodel.MappingStatusMapped,
	}
}

func mustHistoryRuntimeDigest(t *testing.T) [32]byte {
	t.Helper()
	digest, err := historyRuntimeDigest(historyRuntimeFixture())
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func expectHistoryMessage(mock sqlmock.Sqlmock, id int64, role int, content string, meta string, isDel int) {
	var metaValue any
	if meta != "" {
		metaValue = meta
	}
	mock.ExpectQuery("SELECT .* FROM `ai_messages`").WillReturnRows(sqlmock.NewRows([]string{
		"id", "conversation_id", "role", "content_type", "content", "meta_json", "is_del", "created_at", "updated_at",
	}).AddRow(id, 3, role, "text", content, metaValue, isDel, time.Now(), time.Now()))
}
