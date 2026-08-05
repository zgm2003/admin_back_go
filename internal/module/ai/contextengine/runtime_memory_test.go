package contextengine

import (
	"context"
	"crypto/sha256"
	"errors"
	"regexp"
	"strings"
	"testing"

	infraai "admin_back_go/internal/infra/ai"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type runtimeMemoryReaderFixture struct {
	record *MemoryRecord
	err    error
	calls  int
}

func (fixture *runtimeMemoryReaderFixture) LatestReadyMemory(context.Context, uint64, uint64, [sha256.Size]byte) (*MemoryRecord, error) {
	fixture.calls++
	return fixture.record, fixture.err
}

func TestRuntimeMemoryDistinguishesNormalAbsenceFromExpectedMemory(t *testing.T) {
	memoryModelID := uint64(33)

	t.Run("profile without memory model", func(t *testing.T) {
		reader := &runtimeMemoryReaderFixture{}
		history := &historyPagerFixture{turns: []ConversationTurn{runtimeMemoryTurn(t, 1, strings.Repeat("x", 256))}}
		materializer := NewPlanMaterializer(runtimeFactsFixture{}, nil, history).WithMemoryReader(reader)
		memoryContext, err := materializer.runtimeMemory(t.Context(), RuntimeInput{ConversationID: 3, UserID: 7}, runtimeMemoryFacts(nil, 100))
		if err != nil || memoryContext.Record != nil || memoryContext.Expected {
			t.Fatalf("context=%+v err=%v", memoryContext, err)
		}
		if reader.calls != 0 || history.calls != 0 {
			t.Fatalf("reader calls=%d history calls=%d", reader.calls, history.calls)
		}
	})

	t.Run("uncovered turns below memory window", func(t *testing.T) {
		reader := &runtimeMemoryReaderFixture{}
		history := &historyPagerFixture{turns: []ConversationTurn{runtimeMemoryTurn(t, 1, "short")}}
		materializer := NewPlanMaterializer(runtimeFactsFixture{}, nil, history).WithMemoryReader(reader)
		memoryContext, err := materializer.runtimeMemory(t.Context(), RuntimeInput{ConversationID: 3, UserID: 7}, runtimeMemoryFacts(&memoryModelID, 10000))
		if err != nil || memoryContext.Record != nil || memoryContext.Expected {
			t.Fatalf("context=%+v err=%v", memoryContext, err)
		}
	})

	t.Run("memory window crossed without ready record", func(t *testing.T) {
		reader := &runtimeMemoryReaderFixture{}
		history := &historyPagerFixture{turns: []ConversationTurn{runtimeMemoryTurn(t, 1, strings.Repeat("x", 256))}}
		materializer := NewPlanMaterializer(runtimeFactsFixture{}, nil, history).WithMemoryReader(reader)
		memoryContext, err := materializer.runtimeMemory(t.Context(), RuntimeInput{ConversationID: 3, UserID: 7}, runtimeMemoryFacts(&memoryModelID, 100))
		if err != nil || memoryContext.Record != nil || !memoryContext.Expected {
			t.Fatalf("context=%+v err=%v", memoryContext, err)
		}
	})

	t.Run("valid old ready memory", func(t *testing.T) {
		record := validRuntimeMemoryRecord()
		reader := &runtimeMemoryReaderFixture{record: &record}
		materializer := NewPlanMaterializer(runtimeFactsFixture{}, nil, &historyPagerFixture{}).WithMemoryReader(reader)
		memoryContext, err := materializer.runtimeMemory(t.Context(), RuntimeInput{ConversationID: 3, UserID: 7}, runtimeMemoryFacts(&memoryModelID, 100))
		if err != nil || memoryContext.Record == nil || memoryContext.Record.ID != record.ID || !memoryContext.Expected {
			t.Fatalf("context=%+v err=%v", memoryContext, err)
		}
	})
}

func TestRuntimeMemoryRepositoryErrorRemainsStrict(t *testing.T) {
	memoryModelID := uint64(33)
	cause := errors.New("memory repository unavailable")
	reader := &runtimeMemoryReaderFixture{err: cause}
	materializer := NewPlanMaterializer(runtimeFactsFixture{}, nil, &historyPagerFixture{}).WithMemoryReader(reader)

	_, err := materializer.runtimeMemory(t.Context(), RuntimeInput{ConversationID: 3, UserID: 7}, runtimeMemoryFacts(&memoryModelID, 100))
	if !errors.Is(err, cause) {
		t.Fatalf("repository error was replaced: %v", err)
	}
	if _, ok := AsEnhancementFailure(err); ok {
		t.Fatalf("repository error became degradable: %v", err)
	}
}

func TestRuntimeMemoryInvalidReadyRecordIsExpectedButNotUsed(t *testing.T) {
	memoryModelID := uint64(33)
	tests := map[string]func(*MemoryRecord){
		"conversation": func(record *MemoryRecord) { record.ConversationID++ },
		"profile":      func(record *MemoryRecord) { record.ProfileID++ },
		"profile hash": func(record *MemoryRecord) {
			hash := testSHA256("wrong-profile")
			record.ProfileSHA256 = hash[:]
		},
		"range": func(record *MemoryRecord) { record.ThroughMessageID = record.FromMessageID - 1 },
		"summary hash": func(record *MemoryRecord) {
			hash := testSHA256("wrong-summary")
			record.SummarySHA256 = hash[:]
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			record := validRuntimeMemoryRecord()
			mutate(&record)
			reader := &runtimeMemoryReaderFixture{record: &record}
			materializer := NewPlanMaterializer(runtimeFactsFixture{}, nil, &historyPagerFixture{}).WithMemoryReader(reader)
			memoryContext, err := materializer.runtimeMemory(t.Context(), RuntimeInput{ConversationID: 3, UserID: 7}, runtimeMemoryFacts(&memoryModelID, 10000))
			if err != nil || memoryContext.Record != nil || !memoryContext.Expected {
				t.Fatalf("context=%+v err=%v", memoryContext, err)
			}
		})
	}
}

func TestOldReadyMemoryIsNotHiddenByNewerFailedJob(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlDB.Close() }()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	record := validRuntimeMemoryRecord()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `ai_conversation_memories` WHERE conversation_id = ? AND context_profile_id_snapshot = ? AND state = ? ORDER BY through_message_id ASC, id ASC")).
		WithArgs(record.ConversationID, record.ProfileID, MemoryStateReady).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "conversation_id", "context_profile_id_snapshot", "context_profile_sha256", "previous_memory_id",
			"from_message_id", "through_message_id", "source_sha256", "summary_sha256", "policy_version", "summary", "state",
		}).AddRow(record.ID, record.ConversationID, record.ProfileID, record.ProfileSHA256, nil,
			record.FromMessageID, record.ThroughMessageID, record.SourceSHA256, record.SummarySHA256, MemoryPolicyVersionV1, *record.Summary, MemoryStateReady))

	got, err := NewMemoryRepositoryWithDB(db, nil).LatestReadyMemory(t.Context(), record.ConversationID, record.ProfileID, testSHA256("profile"))
	if err != nil || got == nil || got.ID != record.ID {
		t.Fatalf("memory=%+v err=%v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func runtimeMemoryFacts(memoryModelID *uint64, knownInputBudget int64) RuntimeFacts {
	profileHash := testSHA256("profile")
	return RuntimeFacts{
		Profile: &ProfileSnapshot{ID: 7, SHA256: profileHash},
		Retrieval: &RuntimeRetrievalFacts{Profile: ContextProfile{
			ID: 7, MemoryProviderModelID: memoryModelID,
		}},
		ModelCapability: ModelCapabilityHashInput{TokenCounterID: infraai.TokenCounterUTF8BytesV1},
		Budget:          Budget{KnownInputBudget: knownInputBudget},
	}
}

func runtimeMemoryTurn(t *testing.T, id uint64, content string) ConversationTurn {
	t.Helper()
	turn := ConversationTurn{
		ConversationID: 3, UserID: 7, AgentID: 5,
		UserMessage:       TurnMessage{ID: id, Role: "user", Content: content},
		AssistantMessage:  TurnMessage{ID: id + 100, Role: "assistant", Content: content},
		AssistantDelivery: "completed",
	}
	if err := turn.ComputeSourceSHA256(); err != nil {
		t.Fatal(err)
	}
	return turn
}

func validRuntimeMemoryRecord() MemoryRecord {
	summary := "stable conversation memory"
	profileHash := testSHA256("profile")
	sourceHash := testSHA256("memory-source")
	summaryHash := sha256.Sum256([]byte(summary))
	return MemoryRecord{
		ID: 9, ConversationID: 3, ProfileID: 7, ProfileSHA256: profileHash[:],
		FromMessageID: 1, ThroughMessageID: 4, SourceSHA256: sourceHash[:], SummarySHA256: summaryHash[:],
		PolicyVersion: MemoryPolicyVersionV1, Summary: &summary, State: MemoryStateReady,
	}
}
