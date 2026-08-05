package contextengine

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestSelectedFingerprintSourcesRequireExactFacts(t *testing.T) {
	agentHash := testSHA256("agent")
	messageHash := testSHA256("message")
	toolHash := testSHA256("tool")
	attachment := FingerprintAttachment{
		Ordinal: 0, Kind: infraai.AttachmentFile, ObjectKey: "chat/u/7/a.pdf", ETag: "etag-1",
		Size: 12, MIMEType: "application/pdf", Filename: "a.pdf",
	}
	attachmentHash, err := hashRuntimeFacts(runtimeAttachment{
		Kind: attachment.Kind, URL: attachment.URL, ObjectKey: attachment.ObjectKey, ETag: attachment.ETag,
		Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Filename,
	})
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := InputFingerprintHashInput{
		AgentID: 7, AgentSHA256: agentHash,
		Messages: []FingerprintMessage{{ID: 9, Role: infraai.MessageRoleUser, ContentSHA256: messageHash, Attachments: []FingerprintAttachment{attachment}}},
		Tools:    []FingerprintTool{{ID: 11, Name: "search", DefinitionSHA256: toolHash}},
	}
	if !sameFingerprintAttachments(fingerprint.Messages[0].Attachments, []runtimeAttachment{{
		Kind: attachment.Kind, URL: attachment.URL, ObjectKey: attachment.ObjectKey, ETag: attachment.ETag,
		Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Filename,
	}}) {
		t.Fatal("equal attachment facts were rejected")
	}
	changedAttachments := []runtimeAttachment{{
		Kind: attachment.Kind, URL: attachment.URL, ObjectKey: attachment.ObjectKey, ETag: "changed",
		Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Filename,
	}}
	if sameFingerprintAttachments(fingerprint.Messages[0].Attachments, changedAttachments) {
		t.Fatal("changed attachment facts were accepted")
	}

	valid := []AuthoritySource{
		{SourceType: "agent", SourceRef: "agent:7", SourceSHA256: agentHash},
		{SourceType: "message", SourceRef: "message:9", SourceSHA256: messageHash},
		{SourceType: "attachment", SourceRef: "message:9/attachment:0", SourceSHA256: attachmentHash},
		{SourceType: "tool", SourceRef: "tool:11", SourceSHA256: toolHash},
	}
	for _, source := range valid {
		handled, verifyErr := verifySelectedFingerprintSource(fingerprint, source)
		if verifyErr != nil || !handled {
			t.Fatalf("source=%+v handled=%v err=%v", source, handled, verifyErr)
		}
	}

	invalid := valid
	invalid[0].SourceSHA256 = testSHA256("changed")
	invalid[1].SourceRef = "message:10"
	invalid[2].SourceRef = "message:9/attachment:1"
	invalid[3].SourceSHA256 = testSHA256("changed")
	for _, source := range invalid {
		handled, verifyErr := verifySelectedFingerprintSource(fingerprint, source)
		if verifyErr == nil || !handled {
			t.Fatalf("source=%+v handled=%v err=%v", source, handled, verifyErr)
		}
	}
}

func TestSelectedHistoricalAttachmentReloadsDurableCOSFacts(t *testing.T) {
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
	fingerprint := InputFingerprintHashInput{
		Messages: []FingerprintMessage{{ID: 55, Role: infraai.MessageRoleUser, ContentSHA256: testSHA256("current")}},
	}
	metaJSON := `{"attachments":[{"type":"file","object_key":"ai_chat_attachments/2026/08/report.md","etag":"\"etag-1\"","size":90523,"mime_type":"text/markdown","name":"report.md"}]}`

	mock.ExpectQuery(`SELECT message\.conversation_id, conversation\.user_id FROM ai_messages AS message .*message\.id = \?.*message\.is_del = \?.*LIMIT \?`).
		WithArgs(enum.CommonNo, uint64(55), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"conversation_id", "user_id"}).AddRow(uint64(10), uint64(1)))
	mock.ExpectQuery(`SELECT message\.id, message\.conversation_id, conversation\.user_id, message\.content, message\.meta_json, message\.role, message\.is_del FROM ai_messages AS message .*message\.id = \?.*LIMIT \?`).
		WithArgs(uint64(53), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "conversation_id", "user_id", "content", "meta_json", "role", "is_del"}).
			AddRow(uint64(53), uint64(10), uint64(1), "read this", metaJSON, enum.AIMessageRoleUser, enum.CommonNo))

	err = verifySelectedSource(t.Context(), repository.db, "admin", fingerprint, AuthoritySource{
		SourceType: "attachment", SourceRef: "message:53/attachment:0", SourceSHA256: attachmentHash,
	})
	if err != nil {
		t.Errorf("historical COS attachment was rejected: %v", err)
	}
	assertPlanMockExpectations(t, mock)
}

func TestAuthorityConflictClassificationDoesNotHideDatabaseErrors(t *testing.T) {
	if !isAuthoritySnapshotConflict(ErrInvalidContextPlan) || !isAuthoritySnapshotConflict(gorm.ErrRecordNotFound) {
		t.Fatal("known authority changes must be snapshot conflicts")
	}
	if isAuthoritySnapshotConflict(errors.New("mysql connection reset")) {
		t.Fatal("database failures must escape the authorization guard")
	}
}

func TestAuthorityRepositoryErrorIsNotDegraded(t *testing.T) {
	input, dependencies := retrievalClassificationFixture(t)
	cause := errors.New("mysql authority query failed")
	dependencies.Authority = fakeCandidateAuthority{err: cause}
	_, err := Retrieve(t.Context(), input, dependencies)
	if !errors.Is(err, cause) {
		t.Fatalf("authority error was replaced: %v", err)
	}
	if _, ok := AsEnhancementFailure(err); ok {
		t.Fatalf("authority error became degradable: %v", err)
	}
}

func TestContextPolicyAuthorityAcceptsOnlyFixedDegradedInstruction(t *testing.T) {
	source := degradedContextPolicySource()
	handled, err := verifySelectedFingerprintSource(InputFingerprintHashInput{}, source)
	if !handled || err != nil {
		t.Fatalf("fixed policy handled=%v err=%v", handled, err)
	}

	tampered := source
	tampered.SourceSHA256 = testSHA256("tampered policy")
	if handled, err := verifySelectedFingerprintSource(InputFingerprintHashInput{}, tampered); !handled || !errors.Is(err, ErrInvalidContextPlan) {
		t.Fatalf("tampered policy handled=%v err=%v", handled, err)
	}
}

func TestDocumentChunkAuthorityHashMatchesMergedRetrievalHash(t *testing.T) {
	chunkIDs := []uint64{41, 42}
	chunkHashes := [][sha256.Size]byte{testSHA256("chunk-41"), testSHA256("chunk-42")}
	raw, err := json.Marshal(struct {
		Schema string `json:"schema"`
		Chunks []struct {
			ChunkID uint64 `json:"chunk_id"`
			SHA256  string `json:"chunk_facts_sha256"`
		} `json:"chunks"`
	}{
		Schema: "merged_document_chunks_v1",
		Chunks: []struct {
			ChunkID uint64 `json:"chunk_id"`
			SHA256  string `json:"chunk_facts_sha256"`
		}{
			{ChunkID: chunkIDs[0], SHA256: fmt.Sprintf("%x", chunkHashes[0])},
			{ChunkID: chunkIDs[1], SHA256: fmt.Sprintf("%x", chunkHashes[1])},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(raw)
	got, err := documentChunkSourceSHA256(chunkIDs, chunkHashes)
	if err != nil || got != want {
		t.Fatalf("got=%x want=%x err=%v", got, want, err)
	}
	single, err := documentChunkSourceSHA256(chunkIDs[:1], chunkHashes[:1])
	if err != nil || single != chunkHashes[0] {
		t.Fatalf("single=%x want=%x err=%v", single, chunkHashes[0], err)
	}
	parsed, err := parseDocumentChunkAuthorityRef("document_chunks:41,42")
	if err != nil || len(parsed) != 2 || parsed[0] != 41 || parsed[1] != 42 {
		t.Fatalf("parsed=%v err=%v", parsed, err)
	}
}
