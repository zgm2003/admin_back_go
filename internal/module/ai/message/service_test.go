package aimessage

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/shared/enum"

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
		key: {Key: key, MIMEType: "image/png", Size: 10, TrustedURL: "https://trusted.test/a.png"},
	}}
	_, appErr := NewService(repo, WithPricingResolver(testMessagePricingResolver()), WithObjectInspector(inspector)).Send(context.Background(), 7, SendInput{
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
		attachments = append(attachments, Attachment{Type: "image", ObjectKey: key, MIMEType: "image/png"})
		inspector.metadata[key] = storagecos.ObjectMetadata{Key: key, MIMEType: "image/png", Size: 10, TrustedURL: "https://trusted.test/" + strconv.Itoa(index) + ".png"}
	}

	_, appErr := NewService(repo, WithPricingResolver(testMessagePricingResolver()), WithObjectInspector(inspector)).Send(context.Background(), 7, SendInput{
		ConversationID: 3, Content: "看图", RequestID: "rid", Attachments: attachments,
	})
	if appErr != nil {
		t.Fatalf("Send: %v", appErr)
	}
	if inspector.maxActive <= 1 || inspector.maxActive > 5 {
		t.Fatalf("image HEAD concurrency=%d", inspector.maxActive)
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
	)

	_, appErr := service.Send(context.Background(), 7, SendInput{
		ConversationID: 3, Content: "看图", RequestID: "rid",
		Attachments: []Attachment{{Type: "image", ObjectKey: "ai_chat_images/2026/07/28/a.png", MIMEType: "image/png", URL: "https://evil.test/a.png", Size: 1}},
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
			key: {Key: key, MIMEType: "image/png", Size: 2000, TrustedURL: "https://trusted.test/a.jpg"},
		}}
		service := NewService(repo,
			WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
			WithTransportCapabilityResolver(testMessageTransportCapabilities()),
			WithObjectInspector(inspector),
		)

		_, appErr := service.Send(context.Background(), 7, SendInput{
			ConversationID: 3, Content: "看图", RequestID: "rid",
			Attachments: []Attachment{{Type: "image", ObjectKey: key, MIMEType: "image/jpeg", URL: "https://evil.test/a.jpg", Size: 1}},
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
			key: {Key: key, MIMEType: "image/jpeg", Size: 500, TrustedURL: "https://trusted.test/a.jpg"},
		}}
		service := NewService(repo,
			WithPricingResolver(testMessagePricingResolverWithCapabilities(capabilities)),
			WithTransportCapabilityResolver(testMessageTransportCapabilities()),
			WithObjectInspector(inspector),
		)

		_, appErr := service.Send(context.Background(), 7, SendInput{
			ConversationID: 3, Content: "看图", RequestID: "rid",
			Attachments: []Attachment{{Type: "image", ObjectKey: key, MIMEType: "image/png", URL: "https://evil.test/a.jpg", Size: 1}},
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

func TestSendNeverAcceptsNativeDocumentInThisRelease(t *testing.T) {
	repo := &fakeRepository{conversation: &Conversation{ID: 3, UserID: 7, AgentID: 5}, agent: validMessageAgent()}
	_, appErr := NewService(repo).Send(context.Background(), 7, SendInput{
		ConversationID: 3, RequestID: "rid",
		Attachments: []Attachment{{Type: "file", ObjectKey: "ai_chat_images/2026/07/28/a.pdf", MIMEType: "application/pdf"}},
	})
	if appErr == nil || appErr.HTTPStatus != 400 {
		t.Fatalf("native document error=%#v", appErr)
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
