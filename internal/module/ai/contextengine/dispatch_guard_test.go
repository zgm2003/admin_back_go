package contextengine

import (
	"context"
	"errors"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDispatchGuardMemoryRejectsChangedSummaryAndBrokenParent(t *testing.T) {
	summary := "stable summary"
	profileHash, sourceHash, summaryHash := testSHA256("profile"), testSHA256("source"), testSHA256(summary)
	row := MemoryRecord{ID: 7, ConversationID: 3, ProfileID: 5, ProfileSHA256: profileHash[:], FromMessageID: 1, ThroughMessageID: 9,
		SourceSHA256: sourceHash[:], SummarySHA256: summaryHash[:], Summary: &summary, State: MemoryStateReady}
	source := AuthoritySource{SourceType: "conversation_memory", SourceRef: "conversation_memory:7", SourceSHA256: sourceHash}
	if err := validateDispatchMemory(row, nil, 3, 5, profileHash, source); err != nil {
		t.Fatal(err)
	}

	changed := row
	changedSummary := "changed"
	changed.Summary = &changedSummary
	if err := validateDispatchMemory(changed, nil, 3, 5, profileHash, source); !errors.Is(err, errDispatchPermission) {
		t.Fatalf("changed summary error=%v", err)
	}

	parentID := uint64(6)
	row.ParentMemoryID = &parentID
	if err := validateDispatchMemory(row, nil, 3, 5, profileHash, source); !errors.Is(err, errDispatchPermission) {
		t.Fatalf("missing parent error=%v", err)
	}
}

func TestDispatchGuardRejectsDeletedConversationBeforeProviderDispatch(t *testing.T) {
	planRepository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()
	command := lockedReplyCommand{ID: 77, UserID: 7, ConversationID: 3}

	mock.ExpectQuery("SELECT .* FROM `ai_conversations` WHERE id = \\? AND user_id = \\? AND is_del = \\? ORDER BY `ai_conversations`.`id` LIMIT \\?").
		WithArgs(int64(3), int64(7), 2, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	err := requireActiveDispatchConversation(context.Background(), planRepository.db, command)
	if !errors.Is(err, errDispatchPlanConflict) {
		t.Fatalf("deleted conversation dispatch error=%v", err)
	}
	assertPlanMockExpectations(t, mock)
}

func TestDispatchGuardAllowsHistoricalCOSAttachmentFromRunConversation(t *testing.T) {
	repository, mock, closeDB := newPlanRepositoryFixture(t)
	defer closeDB()

	attachment := runtimeAttachment{
		Kind: infraai.AttachmentFile, ObjectKey: "ai_chat_attachments/2026/08/report.md", ETag: `"etag-1"`,
		Size: 90523, MIMEType: "text/markdown", Filename: "report.md",
	}
	attachmentHash, err := hashRuntimeFacts(attachment)
	if err != nil {
		t.Fatal(err)
	}
	metaJSON := `{"attachments":[{"type":"file","object_key":"ai_chat_attachments/2026/08/report.md","etag":"\"etag-1\"","size":90523,"mime_type":"text/markdown","name":"report.md"}]}`
	mock.ExpectQuery(`SELECT message\.id, message\.conversation_id, conversation\.user_id, message\.content, message\.meta_json, message\.role, message\.is_del FROM ai_messages AS message .*message\.id = \?.*LIMIT \?`).
		WithArgs(uint64(53), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "conversation_id", "user_id", "content", "meta_json", "role", "is_del"}).
			AddRow(uint64(53), uint64(10), uint64(1), "read this", metaJSON, enum.AIMessageRoleUser, enum.CommonNo))

	run := dispatchRunRow{ID: 36, UserID: 1, ConversationID: uint64Pointer(10), UserMessageID: uint64Pointer(55)}
	err = verifyDispatchAttachment(t.Context(), repository.db, run, AuthoritySource{
		SourceType: "attachment", SourceRef: "message:53/attachment:0", SourceSHA256: attachmentHash,
	})
	if err != nil {
		t.Errorf("historical COS attachment was rejected before dispatch: %v", err)
	}
	assertPlanMockExpectations(t, mock)
}
