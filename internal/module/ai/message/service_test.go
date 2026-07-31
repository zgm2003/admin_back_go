package aimessage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/capability"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/uploadpolicy"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMessagePricingSnapshotUsesInjectedResolver(t *testing.T) {
	resolverCalls := 0
	service := NewService(&fakeRepository{}, WithPricingResolver(officialmodel.ResolverFunc(func(_ context.Context, modelID string) (officialmodel.ResolvedModel, error) {
		resolverCalls++
		rates := []pricing.Rate{
			{Category: pricing.InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1_000_000},
			{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 2, UnitScale: 1_000_000},
		}
		return officialmodel.ResolvedModel{
			Model:          officialmodel.Model{CatalogVersion: "catalog-v3", CatalogVendor: "openai", ModelID: modelID, ContextWindowTokens: 8192, MaxOutputTokens: 4096},
			EffectivePrice: pricing.PriceBook{ModelID: modelID, Rates: rates}, PriceSource: officialmodel.PriceSourceOfficial,
			PriceSourceURL: "https://openai.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		}, nil
	})))
	raw, effective, err := service.pricingSnapshotForSend(context.Background(), AgentRuntime{
		ModelID: "injected-message-model", EngineType: "openai", ProviderModelStatus: enum.CommonYes,
		OfficialModelID: "injected-message-model", OfficialCatalogVersion: "catalog-v3", MappingStatus: officialmodel.MappingStatusMapped,
		BillingMultiplierPPM: 1_100_000,
	}, nil)
	if err != nil || effective != 4096 || resolverCalls != 1 {
		t.Fatalf("snapshot result = %q, %d, %v; calls=%d", raw, effective, err, resolverCalls)
	}
	snapshot, parseErr := aigateway.ParsePricingSnapshot(raw)
	if parseErr != nil || snapshot.SchemaVersion != aigateway.CurrentPricingSnapshotSchemaVersion || snapshot.RequestedModelID != "injected-message-model" || snapshot.PriceSource != "official" {
		t.Fatalf("snapshot = %#v, %v", snapshot, parseErr)
	}
}

type fakeRepository struct {
	conversation         *Conversation
	agent                *AgentRuntime
	rows                 []MessageProjection
	listQuery            ListQuery
	replyInput           replycommand.CreateReplyInput
	replyResult          replycommand.CreateReplyResult
	replyErr             error
	cancelConversationID int64
	cancelUserID         int64
	cancelRequestID      string
	cancelDeliveredSeq   uint32
	cancelResult         replycommand.RequestCancelResult
	cancelErr            error
}

type fakeGuardedReplyRepository struct {
	replycommand.Repository
	guardedCalls int
	input        replycommand.CreateReplyInput
	guard        replycommand.UploadRuleTransactionGuard
}

func (f *fakeGuardedReplyRepository) CreateReply(context.Context, replycommand.CreateReplyInput) (replycommand.CreateReplyResult, error) {
	return replycommand.CreateReplyResult{}, errors.New("unguarded create called")
}

func (f *fakeGuardedReplyRepository) CreateReplyWithUploadRuleGuard(_ context.Context, input replycommand.CreateReplyInput, guard replycommand.UploadRuleTransactionGuard) (replycommand.CreateReplyResult, error) {
	f.guardedCalls++
	f.input = input
	f.guard = guard
	return replycommand.CreateReplyResult{CommandID: 99, RequestID: input.RequestID}, nil
}

type captureSendUploadRuleGuard struct{}

func (*captureSendUploadRuleGuard) GuardActiveInTransaction(context.Context, *gorm.DB, uploadpolicy.ConsistencyToken) error {
	return nil
}

func (f *fakeRepository) RequestCancel(_ context.Context, input replycommand.RequestCancelInput) (replycommand.RequestCancelResult, error) {
	f.cancelConversationID = input.ConversationID
	f.cancelUserID = input.UserID
	f.cancelRequestID = input.RequestID
	f.cancelDeliveredSeq = input.DeliveredSeq
	if f.cancelResult.CommandID == 0 {
		f.cancelResult = replycommand.RequestCancelResult{
			CommandID: 99, Status: replycommand.CancelStatusStopped, AssistantMessageID: 97, SettlementPending: true,
		}
	}
	return f.cancelResult, f.cancelErr
}

func (f *fakeRepository) Conversation(ctx context.Context, id int64) (*Conversation, error) {
	return f.conversation, nil
}
func (f *fakeRepository) AgentForConversation(ctx context.Context, conversationID int64, userID int64) (*AgentRuntime, error) {
	return f.agent, nil
}
func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]MessageProjection, bool, error) {
	f.listQuery = query
	return f.rows, len(f.rows) > query.Limit, nil
}
func (f *fakeRepository) CreateReply(ctx context.Context, input replycommand.CreateReplyInput) (replycommand.CreateReplyResult, error) {
	f.replyInput = input
	if f.replyResult.CommandID == 0 {
		f.replyResult = replycommand.CreateReplyResult{UserMessageID: 12, CommandID: 99, RequestID: input.RequestID, State: replycommand.StatePending}
	}
	return f.replyResult, f.replyErr
}

type fakeCancelPublisher struct {
	commandID uint64
	err       error
}

type fakeReplyWaker struct {
	commandID uint64
	err       error
}

type staticTransportCapabilityResolver struct {
	metadata infraai.CapabilityMetadata
	ok       bool
}

func (resolver staticTransportCapabilityResolver) ResolveCapabilities(infraai.EngineType) (infraai.CapabilityMetadata, bool) {
	return resolver.metadata, resolver.ok
}

type fakeMessageObjectInspector struct {
	mu        sync.Mutex
	metadata  map[string]storagecos.ObjectMetadata
	err       error
	calls     []string
	active    int
	maxActive int
	delay     time.Duration
}

func (inspector *fakeMessageObjectInspector) Head(_ context.Context, key string) (storagecos.ObjectMetadata, error) {
	inspector.mu.Lock()
	inspector.calls = append(inspector.calls, key)
	inspector.active++
	if inspector.active > inspector.maxActive {
		inspector.maxActive = inspector.active
	}
	inspector.mu.Unlock()
	if inspector.delay > 0 {
		time.Sleep(inspector.delay)
	}
	inspector.mu.Lock()
	inspector.active--
	inspector.mu.Unlock()
	if inspector.err != nil {
		return storagecos.ObjectMetadata{}, inspector.err
	}
	return inspector.metadata[key], nil
}

func (f *fakeReplyWaker) WakeReply(_ context.Context, commandID uint64) error {
	f.commandID = commandID
	return f.err
}

func (f *fakeCancelPublisher) PublishCancel(_ context.Context, commandID uint64) error {
	f.commandID = commandID
	return f.err
}

func TestListUsesMessageCursorAndReturnsChronologicalOrder(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}, rows: []MessageProjection{
		{Message: Message{ID: 11, ConversationID: 3, Role: enum.AIMessageRoleAssistant, ContentType: "text", Content: "second", CreatedAt: now, UpdatedAt: now}},
		{Message: Message{ID: 10, ConversationID: 3, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "first", CreatedAt: now, UpdatedAt: now}},
	}}
	res, appErr := NewService(repo).List(context.Background(), 7, ListQuery{ConversationID: 3, BeforeID: 20})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if repo.listQuery.Limit != 20 || repo.listQuery.BeforeID != 20 {
		t.Fatalf("unexpected list query: %#v", repo.listQuery)
	}
	if len(res.List) != 2 || res.List[0].ID != 10 || res.List[1].ID != 11 || res.List[0].ContentType != "text" {
		t.Fatalf("unexpected response: %#v", res)
	}
}

func TestListPreservesAttachmentCardsWithoutInspectingDeletedObjects(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	meta := `{"attachments":[{"type":"file","object_key":"ai_chat_attachments/2026/07/deleted.pdf","mime_type":"application/pdf","url":"https://trusted.test/deleted.pdf","name":"deleted.pdf","size":4096,"etag":"\"v1\""}]}`
	inspector := &fakeMessageObjectInspector{err: errors.New("object no longer exists")}
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}, rows: []MessageProjection{{Message: Message{
		ID: 41, ConversationID: 3, Role: enum.AIMessageRoleUser, ContentType: "text", Content: "summarize", MetaJSON: &meta,
		CreatedAt: now, UpdatedAt: now,
	}}}}

	result, appErr := NewService(repo, WithObjectInspector(inspector)).List(context.Background(), 7, ListQuery{ConversationID: 3})

	if appErr != nil || len(result.List) != 1 {
		t.Fatalf("list=%#v error=%v", result, appErr)
	}
	decoded, ok := result.List[0].MetaJSON.(map[string]any)
	attachments, okAttachments := decoded["attachments"].([]any)
	if !ok || !okAttachments || len(attachments) != 1 {
		t.Fatalf("attachment card metadata=%#v", result.List[0].MetaJSON)
	}
	if len(inspector.calls) != 0 {
		t.Fatalf("message list inspected historical objects: %v", inspector.calls)
	}
}

func TestListProjectsReplyCommandPairsRunAndLikedWithoutAdjacencyGuessing(t *testing.T) {
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	userID, assistantID, runID := int64(41), int64(97), int64(501)
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}, rows: []MessageProjection{
		{Message: Message{ID: assistantID, ConversationID: 3, Role: enum.AIMessageRoleAssistant, Content: "answer", CreatedAt: now, UpdatedAt: now}, PairedMessageID: &userID, RunID: &runID, Liked: true},
		{Message: Message{ID: 63, ConversationID: 3, Role: enum.AIMessageRoleAssistant, Content: "orphan", CreatedAt: now, UpdatedAt: now}},
		{Message: Message{ID: userID, ConversationID: 3, Role: enum.AIMessageRoleUser, Content: "question", CreatedAt: now, UpdatedAt: now}, PairedMessageID: &assistantID},
	}}

	res, appErr := NewService(repo).List(context.Background(), 7, ListQuery{ConversationID: 3})
	if appErr != nil {
		t.Fatalf("List returned error: %v", appErr)
	}
	if len(res.List) != 3 {
		t.Fatalf("list length=%d", len(res.List))
	}
	user, orphan, assistant := res.List[0], res.List[1], res.List[2]
	if user.ID != userID || user.PairedMessageID == nil || *user.PairedMessageID != assistantID || user.RunID != nil || user.Liked {
		t.Fatalf("user projection=%+v", user)
	}
	if orphan.ID != 63 || orphan.PairedMessageID != nil || orphan.RunID != nil || orphan.Liked {
		t.Fatalf("orphan projection=%+v", orphan)
	}
	if assistant.ID != assistantID || assistant.PairedMessageID == nil || *assistant.PairedMessageID != userID || assistant.RunID == nil || *assistant.RunID != runID || !assistant.Liked {
		t.Fatalf("assistant projection=%+v", assistant)
	}
}

func TestListProjectsStoppedDeliveryAndSettlementState(t *testing.T) {
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	stopped := DeliveryStateStopped
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}, rows: []MessageProjection{{
		Message:           Message{ID: 97, ConversationID: 3, Role: enum.AIMessageRoleAssistant, Content: "1234", DeliveryState: &stopped, CreatedAt: now, UpdatedAt: now},
		SettlementPending: true,
	}}}

	res, appErr := NewService(repo).List(context.Background(), 7, ListQuery{ConversationID: 3})
	if appErr != nil {
		t.Fatal(appErr)
	}
	if len(res.List) != 1 || res.List[0].DeliveryState == nil || *res.List[0].DeliveryState != DeliveryStateStopped || !res.List[0].SettlementPending {
		t.Fatalf("list=%+v", res.List)
	}
}

func TestListProjectionUsesOneBoundedPageQueryAndCanonicalRunIdentity(t *testing.T) {
	db, mock, cleanup := newMessageMockDB(t)
	defer cleanup()
	repository := &GormRepository{db: db}

	rows := sqlmock.NewRows([]string{
		"id", "conversation_id", "role", "content_type", "content", "meta_json", "reply_command_id", "delivery_state", "is_del", "created_at", "updated_at",
		"paired_message_id", "run_id", "liked", "settlement_pending",
	}).AddRow(97, 3, enum.AIMessageRoleAssistant, "text", "answer", nil, 12, DeliveryStateCompleted, enum.CommonNo, time.Now(), time.Now(), 41, 501, true, false).
		AddRow(63, 3, enum.AIMessageRoleAssistant, "text", "orphan", nil, nil, DeliveryStateStopped, enum.CommonNo, time.Now(), time.Now(), nil, nil, false, true).
		AddRow(41, 3, enum.AIMessageRoleUser, "text", "question", nil, nil, nil, enum.CommonNo, time.Now(), time.Now(), 97, nil, false, true)
	mock.ExpectQuery("SELECT .*delivery_state.*paired_message_id.*run_id.*liked.*settlement_pending.*FROM ai_messages.*ai_reply_commands.*paired_messages.*LEFT JOIN ai_runs ON ai_runs.user_id = assistant_commands.user_id AND ai_runs.request_id = assistant_commands.request_id AND ai_runs.assistant_message_id = m.id.*ORDER BY m.id DESC LIMIT \\?").
		WithArgs(int64(7), enum.CommonNo, enum.AIMessageRoleUser, enum.AIMessageRoleAssistant, enum.CommonNo, enum.AIMessageRoleAssistant, int64(3), enum.CommonNo, 3).
		WillReturnRows(rows)

	projected, hasMore, err := repository.List(context.Background(), ListQuery{UserID: 7, ConversationID: 3, Limit: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(projected) != 2 || !hasMore {
		t.Fatalf("rows=%d hasMore=%v", len(projected), hasMore)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("page projection must use exactly one bounded query: %v", err)
	}
	mock.ExpectClose()
}

func newMessageMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("gorm: %v", err)
	}
	return db, mock, func() {
		if err := sqlDB.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
			t.Fatalf("close db: %v", err)
		}
	}
}

func TestListRejectsConversationNotOwnedByCurrentUser(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 8}}
	_, appErr := NewService(repo).List(context.Background(), 7, ListQuery{ConversationID: 3})
	if appErr == nil || appErr.LegacyCode != 403 {
		t.Fatalf("expected forbidden, got %#v", appErr)
	}
}

func TestSendCommitsTextUserMessageAndDurableReplyCommand(t *testing.T) {
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5},
		agent:        validMessageAgent(),
	}
	res, appErr := NewService(repo, WithPricingResolver(testMessagePricingResolver())).Send(context.Background(), 7, SendInput{ConversationID: 3, Content: " hello ", RequestID: "rid", RuntimeParams: map[string]float64{"temperature": 0.7}})
	if appErr != nil {
		t.Fatalf("Send returned error: %v", appErr)
	}
	if res.UserMessageID != 12 || res.CommandID != 99 || res.ConversationID != 3 || res.RequestID != "rid" || res.State != replycommand.StatePending {
		t.Fatalf("unexpected response: %#v", res)
	}
	if repo.replyInput.Content != "hello" || repo.replyInput.ConversationID != 3 || repo.replyInput.UserID != 7 || repo.replyInput.RequestID != "rid" {
		t.Fatalf("unexpected durable reply input: %#v", repo.replyInput)
	}
	if repo.replyInput.MetaJSON == nil || !strings.Contains(*repo.replyInput.MetaJSON, "temperature") {
		t.Fatalf("runtime parameters must be stored in metadata, got %#v", repo.replyInput.MetaJSON)
	}
	if repo.replyInput.RequestFingerprint == ([32]byte{}) || repo.replyInput.RequestIdentityStatus != "replayable" || repo.replyInput.RequestIdentityMarker != "" {
		t.Fatalf("missing canonical request identity: %#v", repo.replyInput)
	}
	if repo.replyInput.AgentID != 5 || repo.replyInput.ProviderID != 9 || repo.replyInput.ModelID != "gpt-4.1-mini" || repo.replyInput.ModelDisplayName != "GPT-4.1 mini" {
		t.Fatalf("missing immutable run identity: %#v", repo.replyInput)
	}
	snapshot, err := aigateway.ParsePricingSnapshot(repo.replyInput.PricingSnapshotJSON)
	if err != nil {
		t.Fatalf("invalid pricing snapshot: %v", err)
	}
	if snapshot.MultiplierPPM != 1_250_000 || snapshot.EffectiveMaxOutputTokens != 4096 || snapshot.TransportEngine != "openai" {
		t.Fatalf("pricing snapshot=%+v", snapshot)
	}
	if strings.TrimSpace(repo.replyInput.InputSnapshot) == "" {
		t.Fatal("input snapshot was not accepted with the paid run")
	}
}

func TestSendPersistsReceivedAndAcceptedTimes(t *testing.T) {
	receivedAt := time.Date(2026, 7, 28, 9, 30, 0, 123456000, time.UTC)
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5},
		agent:        validMessageAgent(),
	}

	_, appErr := NewService(repo, WithPricingResolver(testMessagePricingResolver())).Send(
		context.Background(),
		7,
		SendInput{ConversationID: 3, Content: "hello", RequestID: "latency-rid", RequestReceivedAt: receivedAt},
	)
	if appErr != nil {
		t.Fatalf("Send returned error: %v", appErr)
	}
	if !repo.replyInput.RequestReceivedAt.Equal(receivedAt) {
		t.Fatalf("request_received_at=%v want=%v", repo.replyInput.RequestReceivedAt, receivedAt)
	}
}

func TestSendRejectsUserControlledMaxTokens(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}}
	_, appErr := NewService(repo).Send(context.Background(), 7, SendInput{
		ConversationID: 3,
		Content:        "hello",
		RequestID:      "rid",
		RuntimeParams:  map[string]float64{"max_tokens": 2048},
	})
	if appErr == nil || appErr.HTTPStatus != 400 || !strings.Contains(appErr.Message, "官方模型上限") {
		t.Fatalf("max_tokens error=%#v", appErr)
	}
}

func TestLifecycleRetiredRejectsCallBeforeBilling(t *testing.T) {
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5},
		agent:        validMessageAgent(),
	}
	resolver := officialmodel.ResolverFunc(func(_ context.Context, modelID string) (officialmodel.ResolvedModel, error) {
		resolved, err := testMessagePricingResolver().Resolve(context.Background(), modelID)
		resolved.Model.LifecycleStatus = officialmodel.LifecycleRetired
		return resolved, err
	})
	_, appErr := NewService(repo, WithPricingResolver(resolver)).Send(context.Background(), 7, SendInput{
		ConversationID: 3, Content: "hello", RequestID: "rid",
	})
	if appErr == nil || appErr.Code != "ai.official_model.retired" || appErr.HTTPStatus != 409 {
		t.Fatalf("retired error=%#v", appErr)
	}
	if repo.replyInput.RequestID != "" {
		t.Fatalf("retired route reached durable billing acceptance: %#v", repo.replyInput)
	}
}

func TestSendWakesCommittedCommandAndDoesNotFailWhenWakeupFails(t *testing.T) {
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5},
		agent:        validMessageAgent(),
	}
	waker := &fakeReplyWaker{err: errors.New("redis unavailable")}
	res, appErr := NewService(repo, WithReplyWaker(waker), WithPricingResolver(testMessagePricingResolver())).Send(context.Background(), 7, SendInput{ConversationID: 3, Content: "hello", RequestID: "rid"})
	if appErr != nil {
		t.Fatalf("durable send must survive best-effort wake failure: %v", appErr)
	}
	if res.CommandID != 99 || waker.commandID != 99 {
		t.Fatalf("response=%+v wake command=%d", res, waker.commandID)
	}
}

func TestSendKeepsImageAttachmentsInMetaJSON(t *testing.T) {
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5},
		agent:        validMessageAgent(),
	}
	key := "ai_chat_images/2026/07/28/a.png"
	inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{
		key: {Key: key, MIMEType: "image/png", Size: 10, ETag: `"image-v1"`, TrustedURL: "https://trusted.test/a.png"},
	}}
	_, appErr := NewService(repo, WithPricingResolver(testMessagePricingResolver()), WithObjectInspector(inspector), WithUploadRuleResolver(testMessageUploadRuleResolver())).Send(context.Background(), 7, SendInput{
		ConversationID: 3, Content: "看图", RequestID: "rid",
		Attachments: []Attachment{{Type: "image", ObjectKey: key, MIMEType: "image/png", URL: "https://evil.test/a.png", Name: "a.png", Size: 1}},
	})
	if appErr != nil {
		t.Fatalf("Send returned error: %v", appErr)
	}
	if repo.replyInput.MetaJSON == nil || !strings.Contains(*repo.replyInput.MetaJSON, "attachments") || !strings.Contains(*repo.replyInput.MetaJSON, "https://trusted.test/a.png") {
		t.Fatalf("missing attachment meta json: %#v", repo.replyInput.MetaJSON)
	}
}

func TestSendChecksAtMostFiveImagesConcurrently(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: validMessageAgent()}
	inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{}, delay: 20 * time.Millisecond}
	attachments := make([]Attachment, 0, 5)
	for index := 0; index < 5; index++ {
		key := "ai_chat_images/2026/07/28/" + strconv.Itoa(index) + ".png"
		attachments = append(attachments, Attachment{Type: "image", ObjectKey: key, MIMEType: "image/png", Name: strconv.Itoa(index) + ".png"})
		inspector.metadata[key] = storagecos.ObjectMetadata{Key: key, MIMEType: "image/png", Size: 10, ETag: `"image-v1"`, TrustedURL: "https://trusted.test/" + strconv.Itoa(index) + ".png"}
	}

	_, appErr := NewService(repo, WithPricingResolver(testMessagePricingResolver()), WithObjectInspector(inspector), WithUploadRuleResolver(testMessageUploadRuleResolver())).Send(context.Background(), 7, SendInput{
		ConversationID: 3, Content: "看图", RequestID: "rid", Attachments: attachments,
	})
	if appErr != nil {
		t.Fatalf("Send: %v", appErr)
	}
	if inspector.maxActive <= 1 || inspector.maxActive > 5 {
		t.Fatalf("image HEAD concurrency=%d", inspector.maxActive)
	}
}

type cancelAwareMessageObjectInspector struct {
	slowStarted  chan struct{}
	slowCanceled chan struct{}
}

func (inspector *cancelAwareMessageObjectInspector) Head(ctx context.Context, key string) (storagecos.ObjectMetadata, error) {
	if strings.HasSuffix(key, "slow.png") {
		close(inspector.slowStarted)
		select {
		case <-ctx.Done():
			close(inspector.slowCanceled)
			return storagecos.ObjectMetadata{}, ctx.Err()
		case <-time.After(250 * time.Millisecond):
			return storagecos.ObjectMetadata{}, errors.New("slow HEAD was not canceled")
		}
	}
	<-inspector.slowStarted
	return storagecos.ObjectMetadata{}, errors.New("first HEAD failed")
}

func TestSendCancelsConcurrentHEADsAfterFirstFailure(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: validMessageAgent()}
	inspector := &cancelAwareMessageObjectInspector{slowStarted: make(chan struct{}), slowCanceled: make(chan struct{})}
	service := NewService(repo,
		WithPricingResolver(testMessagePricingResolver()),
		WithObjectInspector(inspector),
		WithUploadRuleResolver(testMessageUploadRuleResolver()),
	)

	_, appErr := service.Send(context.Background(), 7, SendInput{
		ConversationID: 3, Content: "看图", RequestID: "cancel-heads",
		Attachments: []Attachment{
			{Type: "image", ObjectKey: "ai_chat_images/2026/07/28/slow.png", Name: "slow.png"},
			{Type: "image", ObjectKey: "ai_chat_images/2026/07/28/fail.png", Name: "fail.png"},
		},
	})

	if appErr == nil {
		t.Fatal("failed HEAD was accepted")
	}
	select {
	case <-inspector.slowCanceled:
	default:
		t.Fatal("first HEAD failure did not cancel the concurrent request")
	}
}

func TestSendRejectsUnsupportedTemperature(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: validMessageAgent()}
	capabilities := officialmodel.Capabilities{
		InputModalities: []string{officialmodel.ModalityText}, OutputModalities: []string{officialmodel.ModalityText}, SupportsStreaming: true,
	}
	service := NewService(
		repo,
		WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
		WithTransportCapabilityResolver(testMessageTransportCapabilities()),
	)

	_, appErr := service.Send(context.Background(), 7, SendInput{
		ConversationID: 3, Content: "hello", RequestID: "rid", RuntimeParams: map[string]float64{"temperature": 0.7},
	})
	if appErr == nil || appErr.HTTPStatus != 400 || !strings.Contains(appErr.Message, "temperature") {
		t.Fatalf("unsupported temperature error=%#v", appErr)
	}
	if repo.replyInput.RequestID != "" {
		t.Fatalf("unsupported temperature reached durable acceptance: %#v", repo.replyInput)
	}
}

func TestSendRejectsImageWithoutEffectiveImageCapability(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: validMessageAgent()}
	inspector := &fakeMessageObjectInspector{}
	capabilities := officialmodel.Capabilities{
		InputModalities: []string{officialmodel.ModalityText}, OutputModalities: []string{officialmodel.ModalityText}, SupportsStreaming: true,
	}
	service := NewService(
		repo,
		WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
		WithTransportCapabilityResolver(testMessageTransportCapabilities()),
		WithObjectInspector(inspector),
		WithUploadRuleResolver(testMessageUploadRuleResolver()),
	)

	_, appErr := service.Send(context.Background(), 7, SendInput{
		ConversationID: 3, Content: "看图", RequestID: "rid",
		Attachments: []Attachment{{Type: "image", ObjectKey: "ai_chat_images/2026/07/28/a.png", MIMEType: "image/png", URL: "https://evil.test/a.png", Name: "a.png", Size: 1}},
	})
	if appErr == nil || appErr.HTTPStatus != 400 || !strings.Contains(appErr.Message, "图片") {
		t.Fatalf("image capability error=%#v", appErr)
	}
	if len(inspector.calls) != 0 || repo.replyInput.RequestID != "" {
		t.Fatalf("unsupported image reached inspector or durable acceptance: calls=%v input=%#v", inspector.calls, repo.replyInput)
	}
}

func TestSendUsesTrustedObjectMetadataForMimeAndSize(t *testing.T) {
	capabilities := officialmodel.Capabilities{
		InputModalities:  []string{officialmodel.ModalityText, officialmodel.ModalityImage},
		OutputModalities: []string{officialmodel.ModalityText}, SupportsStreaming: true,
		ImageInput: &officialmodel.ImageInputCapability{MIMETypes: []string{"image/jpeg"}, MaxFiles: 5, MaxBytes: 1000},
	}
	key := "ai_chat_images/2026/07/28/a.jpg"

	t.Run("rejects forged client facts", func(t *testing.T) {
		repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: validMessageAgent()}
		inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{
			key: {Key: key, MIMEType: "image/png", Size: 2000, ETag: `"image-v1"`, TrustedURL: "https://trusted.test/a.jpg"},
		}}
		service := NewService(repo,
			WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
			WithTransportCapabilityResolver(testMessageTransportCapabilities()),
			WithObjectInspector(inspector),
			WithUploadRuleResolver(testMessageUploadRuleResolver()),
		)

		_, appErr := service.Send(context.Background(), 7, SendInput{
			ConversationID: 3, Content: "看图", RequestID: "rid",
			Attachments: []Attachment{{Type: "image", ObjectKey: key, MIMEType: "image/jpeg", URL: "https://evil.test/a.jpg", Name: "a.jpg", Size: 1}},
		})
		if appErr == nil || appErr.HTTPStatus != 400 {
			t.Fatalf("forged metadata error=%#v", appErr)
		}
		if repo.replyInput.RequestID != "" {
			t.Fatalf("forged metadata reached durable acceptance: %#v", repo.replyInput)
		}
	})

	t.Run("persists trusted facts", func(t *testing.T) {
		repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: validMessageAgent()}
		inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{
			key: {Key: key, MIMEType: "image/jpeg", Size: 500, ETag: `"image-v1"`, TrustedURL: "https://trusted.test/a.jpg"},
		}}
		service := NewService(repo,
			WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
			WithTransportCapabilityResolver(testMessageTransportCapabilities()),
			WithObjectInspector(inspector),
			WithUploadRuleResolver(testMessageUploadRuleResolver()),
		)

		_, appErr := service.Send(context.Background(), 7, SendInput{
			ConversationID: 3, Content: "看图", RequestID: "rid",
			Attachments: []Attachment{{Type: "image", ObjectKey: key, MIMEType: "image/png", URL: "https://evil.test/a.jpg", Name: "a.jpg", Size: 1}},
		})
		if appErr != nil {
			t.Fatalf("trusted attachment rejected: %v", appErr)
		}
		if repo.replyInput.MetaJSON == nil {
			t.Fatal("trusted attachment metadata missing")
		}
		for _, wanted := range []string{key, `"mime_type":"image/jpeg"`, `"size":500`, "https://trusted.test/a.jpg"} {
			if !strings.Contains(*repo.replyInput.MetaJSON, wanted) {
				t.Fatalf("trusted metadata missing %q in %s", wanted, *repo.replyInput.MetaJSON)
			}
		}
		if strings.Contains(*repo.replyInput.MetaJSON, "evil.test") {
			t.Fatalf("client URL survived trusted normalization: %s", *repo.replyInput.MetaJSON)
		}
	})
}

func TestSendRejectsImageExtensionMIMEConflictAndUnverifiedGIF(t *testing.T) {
	capabilities := officialmodel.Capabilities{
		InputModalities: []string{officialmodel.ModalityText, officialmodel.ModalityImage}, OutputModalities: []string{officialmodel.ModalityText}, SupportsStreaming: true,
		ImageInput: &officialmodel.ImageInputCapability{MIMETypes: []string{"image/png", "image/gif"}, MaxFiles: 5, MaxBytes: 1000},
	}
	tests := []struct {
		name, key, mime string
		gifVerified     bool
	}{
		{name: "png key with gif MIME", key: "ai_chat_attachments/2026/07/a.png", mime: "image/gif", gifVerified: true},
		{name: "gif key with png MIME", key: "ai_chat_attachments/2026/07/a.gif", mime: "image/png"},
		{name: "gif without static proof", key: "ai_chat_attachments/2026/07/a.gif", mime: "image/gif"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: validMessageAgent()}
			inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{
				test.key: {Key: test.key, MIMEType: test.mime, Size: 100, ETag: `"v1"`, TrustedURL: "https://trusted.test/a", GIFStaticVerified: test.gifVerified},
			}}
			service := NewService(repo,
				WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
				WithTransportCapabilityResolver(testMessageTransportCapabilities()),
				WithObjectInspector(inspector), WithUploadRuleResolver(testMessageUploadRuleResolver()),
			)
			_, appErr := service.Send(context.Background(), 7, SendInput{
				ConversationID: 3, Content: "看图", RequestID: "rid-" + test.name,
				Attachments: []Attachment{{Type: "image", ObjectKey: test.key, Name: path.Base(test.key), MIMEType: test.mime, URL: "https://client.test/a", Size: 1}},
			})
			if appErr == nil || repo.replyInput.RequestID != "" {
				t.Fatalf("error=%#v accepted=%#v", appErr, repo.replyInput)
			}
		})
	}
}

func TestSendNormalizesMixedAttachmentsFromTrustedHEAD(t *testing.T) {
	agent := validMessageAgent()
	agent.ModelID, agent.OfficialModelID = "gpt-5.6", "gpt-5.6"
	agent.FileInputMode = aiprovider.FileInputModeChatCompletions
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: agent}
	key := "ai_chat_attachments/2026/07/report.pdf"
	inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{
		key: {Key: key, MIMEType: "application/pdf", Size: 4096, ETag: `"v1"`, TrustedURL: "https://trusted.test/report.pdf"},
	}}
	capabilities := officialmodel.Capabilities{
		InputModalities: []string{"text", "image", "file"}, OutputModalities: []string{"text"},
		SupportsStreaming: true, NativeFileInput: true,
		ImageInput: &officialmodel.ImageInputCapability{MIMETypes: []string{"image/png"}, MaxFiles: 5, MaxBytes: 10 << 20},
	}
	service := NewService(repo,
		WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
		WithTransportCapabilityResolver(staticTransportCapabilityResolver{ok: true, metadata: infraai.CapabilityMetadata{
			InputModalities: []string{"text", "image", "file"}, OutputModalities: []string{"text"}, SupportsStreaming: true,
		}}),
		WithObjectInspector(inspector),
		WithUploadRuleResolver(uploadpolicy.ResolverFunc(func(context.Context) (uploadpolicy.Rule, error) {
			return uploadpolicy.Rule{MaxFileBytes: 100 << 20, ImageExtensions: []string{"png"}, FileExtensions: []string{"pdf"}, ConsistencyToken: uploadpolicy.ConsistencyToken{1}}, nil
		})),
	)
	_, appErr := service.Send(context.Background(), 7, SendInput{
		ConversationID: 3, RequestID: "rid", Content: "总结文件",
		Attachments: []Attachment{{
			Type: "file", ObjectKey: key, MIMEType: "text/plain", URL: "https://evil.test/report.pdf", Name: "report.pdf", Size: 1,
		}},
	})
	if appErr != nil {
		t.Fatal(appErr)
	}
	want := Attachment{Type: "file", ObjectKey: key, MIMEType: "application/pdf", URL: "https://trusted.test/report.pdf", Name: "report.pdf", Size: 4096, ETag: `"v1"`}
	var meta struct {
		Attachments []Attachment `json:"attachments"`
	}
	if repo.replyInput.MetaJSON == nil || json.Unmarshal([]byte(*repo.replyInput.MetaJSON), &meta) != nil || !reflect.DeepEqual(meta.Attachments, []Attachment{want}) {
		t.Fatalf("attachment meta=%#v raw=%v", meta.Attachments, repo.replyInput.MetaJSON)
	}
}

func TestSendPropagatesUploadRuleTokenAndMapsTransactionalDrift(t *testing.T) {
	agent := validFileMessageAgent()
	repo := &fakeRepository{
		conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: &agent,
		replyErr: replycommand.ErrUploadRuleChanged,
	}
	key := "ai_chat_attachments/2026/07/report.pdf"
	inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{
		key: {Key: key, MIMEType: "application/pdf", Size: 4096, ETag: `"v1"`, TrustedURL: "https://trusted.test/report.pdf"},
	}}
	capabilities := officialmodel.Capabilities{
		InputModalities: []string{"text", "file"}, OutputModalities: []string{"text"}, SupportsStreaming: true, NativeFileInput: true,
	}
	service := NewService(repo,
		WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
		WithTransportCapabilityResolver(staticTransportCapabilityResolver{ok: true, metadata: infraai.CapabilityMetadata{
			InputModalities: []string{"text", "file"}, OutputModalities: []string{"text"}, SupportsStreaming: true,
		}}),
		WithObjectInspector(inspector), WithUploadRuleResolver(testMessageUploadRuleResolver()),
	)

	_, appErr := service.Send(context.Background(), 7, SendInput{
		ConversationID: 3, RequestID: "rule-drift", Content: "总结文件",
		Attachments: []Attachment{{Type: "file", ObjectKey: key, Name: "report.pdf"}},
	})
	if appErr == nil || appErr.HTTPStatus != 409 || appErr.MessageID != "aimessage.attachments.upload_rule_changed" {
		t.Fatalf("upload rule drift error=%#v", appErr)
	}
	if repo.replyInput.UploadRuleToken != (uploadpolicy.ConsistencyToken{1}) {
		t.Fatalf("upload rule token=%x", repo.replyInput.UploadRuleToken)
	}
}

func TestMessageRepositoryDelegatesAttachmentCreateWithConfiguredTransactionGuard(t *testing.T) {
	delegate := &fakeGuardedReplyRepository{}
	guard := &captureSendUploadRuleGuard{}
	repository := &GormRepository{replies: delegate, uploadRuleGuard: guard}
	input := replycommand.CreateReplyInput{RequestID: "guarded", UploadRuleToken: uploadpolicy.ConsistencyToken{7}}

	result, err := repository.CreateReply(context.Background(), input)
	if err != nil || result.CommandID != 99 {
		t.Fatalf("CreateReply result=%+v err=%v", result, err)
	}
	if delegate.guardedCalls != 1 || delegate.guard != guard || delegate.input.UploadRuleToken != input.UploadRuleToken {
		t.Fatalf("guarded delegation calls=%d guard=%T input=%+v", delegate.guardedCalls, delegate.guard, delegate.input)
	}
}

func TestSendEnforcesTrustedAttachmentByteLimits(t *testing.T) {
	tests := []struct {
		name          string
		sizes         []int64
		systemMax     int64
		wantMessageID string
		wantCode      string
	}{
		{name: "native file is strictly below fifty MiB", sizes: []int64{50 << 20}, systemMax: 100 << 20, wantMessageID: "aimessage.attachments.file_size_exceeded", wantCode: "ai.attachment.file_too_large"},
		{name: "message aggregate is at most fifty MiB", sizes: []int64{30 << 20, 21 << 20}, systemMax: 100 << 20, wantMessageID: "aimessage.attachments.total_size_exceeded", wantCode: "ai.attachment.message_total_too_large"},
		{name: "system upload rule remains authoritative", sizes: []int64{2 << 20}, systemMax: 1 << 20, wantMessageID: "aimessage.attachments.system_size_exceeded", wantCode: "ai.attachment.file_too_large"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			agent := validFileMessageAgent()
			repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: &agent}
			inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{}}
			attachments := make([]Attachment, len(test.sizes))
			for index, size := range test.sizes {
				name := "report-" + strconv.Itoa(index) + ".pdf"
				key := "ai_chat_attachments/2026/07/" + name
				attachments[index] = Attachment{Type: "file", ObjectKey: key, Name: name}
				inspector.metadata[key] = storagecos.ObjectMetadata{
					Key: key, MIMEType: "application/pdf", Size: size, ETag: `"v1"`, TrustedURL: "https://trusted.test/" + name,
				}
			}
			capabilities := officialmodel.Capabilities{
				InputModalities: []string{"text", "file"}, OutputModalities: []string{"text"}, SupportsStreaming: true, NativeFileInput: true,
			}
			service := NewService(repo,
				WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
				WithTransportCapabilityResolver(staticTransportCapabilityResolver{ok: true, metadata: infraai.CapabilityMetadata{
					InputModalities: []string{"text", "file"}, OutputModalities: []string{"text"}, SupportsStreaming: true,
				}}),
				WithObjectInspector(inspector),
				WithUploadRuleResolver(uploadpolicy.ResolverFunc(func(context.Context) (uploadpolicy.Rule, error) {
					return uploadpolicy.Rule{MaxFileBytes: test.systemMax, FileExtensions: []string{"pdf"}, ConsistencyToken: uploadpolicy.ConsistencyToken{1}}, nil
				})),
			)
			_, appErr := service.Send(context.Background(), 7, SendInput{
				ConversationID: 3, Content: "summarize", RequestID: "limit-rid", Attachments: attachments,
			})
			if appErr == nil || appErr.Code != test.wantCode || appErr.MessageID != test.wantMessageID ||
				appErr.Category != apperror.CategoryValidation || appErr.HTTPStatus != http.StatusBadRequest || appErr.Retry != apperror.Permanent || repo.replyInput.RequestID != "" {
				t.Fatalf("limit error=%#v reply=%#v", appErr, repo.replyInput)
			}
		})
	}
}

func TestNativeFileAttachmentStableAcceptanceErrors(t *testing.T) {
	tests := []struct {
		name, reason, code, messageID string
	}{
		{name: "official model", reason: capability.NativeFileDisabledOfficialModel, code: "ai.attachment.model_unsupported", messageID: "aimessage.attachments.official_model_unsupported"},
		{name: "provider mode", reason: capability.NativeFileDisabledProviderMode, code: "ai.attachment.provider_file_input_disabled", messageID: "aimessage.attachments.provider_file_input_disabled"},
		{name: "transport", reason: capability.NativeFileDisabledTransport, code: "ai.attachment.transport_unsupported", messageID: "aimessage.attachments.transport_unsupported"},
		{name: "platform type set", reason: capability.NativeFileDisabledPlatform, code: "ai.attachment.type_unsupported", messageID: "aimessage.attachments.platform_unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			appErr := nativeFileCapabilityError(test.reason)
			if appErr.Code != test.code || appErr.MessageID != test.messageID || appErr.Category != apperror.CategoryValidation ||
				appErr.HTTPStatus != http.StatusBadRequest || appErr.Retry != apperror.Permanent {
				t.Fatalf("stable error=%#v", appErr)
			}
		})
	}

	tooMany := make([]Attachment, capability.MaxAttachmentsPerMessage+1)
	_, _, appErr := (&Service{}).inspectAttachments(context.Background(), AgentRuntime{}, officialmodel.Capabilities{}, officialmodel.Capabilities{}, tooMany)
	if appErr == nil || appErr.Code != "ai.attachment.too_many" || appErr.Category != apperror.CategoryValidation || appErr.HTTPStatus != http.StatusBadRequest || appErr.Retry != apperror.Permanent {
		t.Fatalf("too many error=%#v", appErr)
	}

	_, appErr = normalizeLocalAttachment(Attachment{Type: "archive"}, uploadpolicy.Rule{}, capability.NativeFileCapability{}, officialmodel.Capabilities{})
	if appErr == nil || appErr.Code != "ai.attachment.type_unsupported" || appErr.Category != apperror.CategoryValidation || appErr.HTTPStatus != http.StatusBadRequest || appErr.Retry != apperror.Permanent {
		t.Fatalf("type error=%#v", appErr)
	}
}

func TestNativeFileAttachmentObjectErrorsRemainDistinct(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "object unavailable", err: storagecos.ErrObjectUnavailable, code: "ai.attachment.object_unavailable"},
		{name: "object version changed", err: storagecos.ErrObjectVersionChanged, code: "ai.attachment.object_version_changed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &Service{
				capabilities:    staticTransportCapabilityResolver{ok: true, metadata: infraai.CapabilityMetadata{InputModalities: []string{"text", "file"}}},
				uploadRules:     testMessageUploadRuleResolver(),
				objectInspector: &fakeMessageObjectInspector{err: test.err},
			}
			runtime := AgentRuntime{
				EngineType: "openai", FileInputMode: aiprovider.FileInputModeChatCompletions,
				ProviderModelStatus: enum.CommonYes, MappingStatus: officialmodel.MappingStatusMapped,
			}
			capabilities := officialmodel.Capabilities{InputModalities: []string{"text", "file"}, NativeFileInput: true}
			_, _, appErr := service.inspectAttachments(context.Background(), runtime, capabilities, capabilities, []Attachment{{
				Type: "file", ObjectKey: "ai_chat_attachments/2026/07/report.pdf", Name: "report.pdf",
			}})
			if appErr == nil || appErr.Code != test.code || appErr.Category != apperror.CategoryConflict ||
				appErr.HTTPStatus != http.StatusConflict || appErr.Retry != apperror.Permanent {
				t.Fatalf("object error=%#v", appErr)
			}
		})
	}
}

func TestSendRejectsNativeFileWhenProviderProtocolIsDisabled(t *testing.T) {
	agent := validFileMessageAgent()
	agent.FileInputMode = aiprovider.FileInputModeDisabled
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: &agent}
	inspector := &fakeMessageObjectInspector{}
	capabilities := officialmodel.Capabilities{
		InputModalities: []string{"text", "file"}, OutputModalities: []string{"text"}, SupportsStreaming: true, NativeFileInput: true,
	}
	service := NewService(repo,
		WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
		WithTransportCapabilityResolver(staticTransportCapabilityResolver{ok: true, metadata: infraai.CapabilityMetadata{
			InputModalities: []string{"text", "file"}, OutputModalities: []string{"text"}, SupportsStreaming: true,
		}}),
		WithObjectInspector(inspector),
		WithUploadRuleResolver(testMessageUploadRuleResolver()),
	)
	_, appErr := service.Send(context.Background(), 7, SendInput{
		ConversationID: 3, Content: "summarize", RequestID: "provider-disabled",
		Attachments: []Attachment{{Type: "file", ObjectKey: "ai_chat_attachments/2026/07/report.pdf", Name: "report.pdf"}},
	})
	if appErr == nil || appErr.MessageID != "aimessage.attachments.provider_file_input_disabled" || len(inspector.calls) != 0 || repo.replyInput.RequestID != "" {
		t.Fatalf("provider disabled error=%#v calls=%v reply=%#v", appErr, inspector.calls, repo.replyInput)
	}
}

func TestSendAcceptsAmbiguousPlainTextOnlyForTextLikeFiles(t *testing.T) {
	tests := []struct {
		name, fileName string
		wantOK         bool
	}{
		{name: "code file", fileName: "main.go", wantOK: true},
		{name: "binary document", fileName: "report.pdf", wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			agent := validFileMessageAgent()
			repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: &agent}
			key := "ai_chat_attachments/2026/07/" + test.fileName
			inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{
				key: {Key: key, MIMEType: "text/plain", Size: 128, ETag: `"v1"`, TrustedURL: "https://trusted.test/" + test.fileName},
			}}
			capabilities := officialmodel.Capabilities{
				InputModalities: []string{"text", "file"}, OutputModalities: []string{"text"}, SupportsStreaming: true, NativeFileInput: true,
			}
			service := NewService(repo,
				WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
				WithTransportCapabilityResolver(staticTransportCapabilityResolver{ok: true, metadata: infraai.CapabilityMetadata{
					InputModalities: []string{"text", "file"}, OutputModalities: []string{"text"}, SupportsStreaming: true,
				}}),
				WithObjectInspector(inspector), WithUploadRuleResolver(testMessageUploadRuleResolver()),
			)
			_, appErr := service.Send(context.Background(), 7, SendInput{
				ConversationID: 3, Content: "read", RequestID: "mime-rid", Attachments: []Attachment{{Type: "file", ObjectKey: key, Name: test.fileName}},
			})
			if (appErr == nil) != test.wantOK {
				t.Fatalf("mime result error=%#v", appErr)
			}
		})
	}
}

func validMessageAgent() *AgentRuntime {
	return &AgentRuntime{
		AgentID: 5, ProviderID: 9, ModelID: "gpt-4.1-mini", ModelDisplayName: "GPT-4.1 mini", EngineType: "openai",
		ProviderModelStatus: enum.CommonYes, OfficialModelID: "gpt-4.1-mini", OfficialCatalogVersion: "catalog-v3", MappingStatus: officialmodel.MappingStatusMapped,
		BillingMultiplierPPM: 1_250_000, Status: enum.CommonYes, ScenesJSON: `["chat"]`,
	}
}

func testMessagePricingResolver() officialmodel.Resolver {
	return testMessagePricingResolverWithCapabilities(officialmodel.Capabilities{
		InputModalities:  []string{officialmodel.ModalityText, officialmodel.ModalityImage},
		OutputModalities: []string{officialmodel.ModalityText}, SupportsStreaming: true, SupportsTools: true,
		SupportedParameters: []string{officialmodel.ParameterTemperature},
		ImageInput:          &officialmodel.ImageInputCapability{MIMETypes: []string{"image/jpeg", "image/png", "image/webp", "image/gif"}, MaxFiles: 5, MaxBytes: 10 << 20},
	})
}

func testMessagePricingResolverWithCapabilities(capabilities officialmodel.Capabilities) officialmodel.Resolver {
	return officialmodel.ResolverFunc(func(_ context.Context, modelID string) (officialmodel.ResolvedModel, error) {
		rates := []pricing.Rate{
			{Category: pricing.InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1_000_000},
			{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 2, UnitScale: 1_000_000},
		}
		return officialmodel.ResolvedModel{
			Model:          officialmodel.Model{CatalogVersion: "catalog-v3", CatalogVendor: "openai", ModelID: modelID, ContextWindowTokens: 8192, MaxOutputTokens: 4096, Capabilities: capabilities},
			EffectivePrice: pricing.PriceBook{ModelID: modelID, Rates: rates}, PriceSource: officialmodel.PriceSourceOfficial,
			PriceSourceURL: "https://openai.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		}, nil
	})
}

func testMessageTransportCapabilities() staticTransportCapabilityResolver {
	return staticTransportCapabilityResolver{ok: true, metadata: infraai.CapabilityMetadata{
		InputModalities: []string{officialmodel.ModalityText, officialmodel.ModalityImage}, OutputModalities: []string{officialmodel.ModalityText},
		SupportedParameters: []string{officialmodel.ParameterTemperature}, SupportsStreaming: true, SupportsTools: true, SupportsStructuredOutput: true,
	}}
}

func testMessageUploadRuleResolver() uploadpolicy.Resolver {
	return uploadpolicy.ResolverFunc(func(context.Context) (uploadpolicy.Rule, error) {
		return uploadpolicy.Rule{
			MaxFileBytes: 100 << 20, ImageExtensions: []string{"jpeg", "jpg", "png", "gif", "webp"}, FileExtensions: []string{"pdf", "md", "go"},
			ConsistencyToken: uploadpolicy.ConsistencyToken{1},
		}, nil
	})
}

func TestCancelRequiresOwnedConversation(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7}}
	publisher := &fakeCancelPublisher{err: errors.New("redis unavailable")}
	res, appErr := NewService(repo, WithCancelPublisher(publisher)).Cancel(context.Background(), 7, CancelInput{ConversationID: 3, RequestID: "rid", DeliveredSeq: 4})
	if appErr != nil {
		t.Fatalf("Cancel returned error: %v", appErr)
	}
	if res.ConversationID != 3 || res.RequestID != "rid" || res.Status != string(replycommand.CancelStatusStopped) || res.AssistantMessageID == nil || *res.AssistantMessageID != 97 || !res.SettlementPending {
		t.Fatalf("unexpected cancel response: %#v", res)
	}
	if repo.cancelConversationID != 3 || repo.cancelUserID != 7 || repo.cancelRequestID != "rid" || repo.cancelDeliveredSeq != 4 || publisher.commandID != 99 {
		t.Fatalf("durable cancel repo=(%d,%d,%q) signal=%d", repo.cancelConversationID, repo.cancelUserID, repo.cancelRequestID, publisher.commandID)
	}
}

func TestSendRejectsNonChatAgent(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: &AgentRuntime{AgentID: 5, Status: enum.CommonYes, ScenesJSON: `["image"]`}}
	_, appErr := NewService(repo).Send(context.Background(), 7, SendInput{ConversationID: 3, Content: "hello", RequestID: "rid"})
	if appErr == nil || appErr.LegacyCode != 100 || appErr.Message != "该智能体不支持对话场景" {
		t.Fatalf("expected non-chat bad request, got %#v", appErr)
	}
}
