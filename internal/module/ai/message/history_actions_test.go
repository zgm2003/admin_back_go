package aimessage

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/module/ai/requestidentity"
	"admin_back_go/internal/shared/enum"

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
}

func (f *fakeHistoryRepository) Revise(_ context.Context, input EditInput) (HistoryAccepted, error) {
	f.revisionInput = input
	return HistoryAccepted{Reply: f.result, Replayed: f.replayed}, f.err
}

func (f *fakeHistoryRepository) Regenerate(_ context.Context, input RegenerateInput) (HistoryAccepted, error) {
	f.regenerationInput = input
	return HistoryAccepted{Reply: f.result, Replayed: f.replayed}, f.err
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
	} {
		t.Run(test.name, func(t *testing.T) {
			history := &fakeHistoryRepository{err: test.err}
			_, appErr := NewService(&fakeRepository{}, WithHistoryRepository(history)).Revise(context.Background(), 7, EditInput{
				ConversationID: 3, MessageID: 41, Content: "changed", RequestID: "revision-1",
			})
			if appErr == nil || appErr.HTTPStatus != test.wantStatus || appErr.Code != test.wantCode {
				t.Fatalf("error=%#v", appErr)
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
	facts, err := buildHistoryRequestFacts(HistoryOperationRevision, 7, 3, "new text", source, runtime, 4096)
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
	repository := &GormRepository{db: db, history: participant, pricing: testMessagePricingResolver()}
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

	_, err := repository.Revise(context.Background(), EditInput{UserID: 7, ConversationID: 3, MessageID: 41, Content: "new text", RequestID: "revision-1"})
	if err == nil || err.Error() != "run insert failed" {
		t.Fatalf("revision error=%v", err)
	}
	if participant.created.MetaJSON == nil || *participant.created.MetaJSON == "" || participant.created.Content != "new text" {
		t.Fatalf("edit did not inherit server metadata: %+v", participant.created)
	}
	if participant.created.Identity.Operation != HistoryOperationRevision || participant.created.Identity.SourceMessageID != 41 {
		t.Fatalf("typed identity=%+v", participant.created.Identity)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("revision transaction: %v", err)
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

	_, err := repository.Regenerate(context.Background(), RegenerateInput{UserID: 7, ConversationID: 3, AssistantMessageID: 97, RequestID: "regen-1"})
	if !errors.Is(err, ErrHistorySourceNotFound) {
		t.Fatalf("missing pair error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("missing pair queried after mutation: %v", err)
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
	pattern := "SELECT .* FROM .*ai_conversations.*ai_agents.*ai_providers"
	if locked {
		pattern += ".*FOR UPDATE"
	}
	mock.ExpectQuery(pattern).WillReturnRows(sqlmock.NewRows([]string{
		"agent_id", "provider_id", "model_id", "model_display_name", "engine_type", "billing_multiplier_ppm", "status", "scenes_json",
		"provider_model_status", "official_model_id", "official_catalog_version", "mapping_status",
	}).AddRow(5, 9, "gpt-4.1-mini", "GPT-4.1 mini", "openai", 1_250_000, enum.CommonYes, `["chat"]`,
		enum.CommonYes, "gpt-4.1-mini", "catalog-v3", officialmodel.MappingStatusMapped))
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
