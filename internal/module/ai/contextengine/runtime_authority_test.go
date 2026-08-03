package contextengine

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	infraai "admin_back_go/internal/infra/ai"

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

func TestAuthorityConflictClassificationDoesNotHideDatabaseErrors(t *testing.T) {
	if !isAuthoritySnapshotConflict(ErrInvalidContextPlan) || !isAuthoritySnapshotConflict(gorm.ErrRecordNotFound) {
		t.Fatal("known authority changes must be snapshot conflicts")
	}
	if isAuthoritySnapshotConflict(errors.New("mysql connection reset")) {
		t.Fatal("database failures must escape the authorization guard")
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
