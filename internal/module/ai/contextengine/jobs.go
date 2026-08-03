package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"admin_back_go/internal/infra/taskqueue"
)

const TaskContextDocumentIndexV1 = "ai:context-document-index:v1"

type ContextDocumentIndexV1 struct {
	DocumentVersionID uint64 `json:"document_version_id"`
}

type DocumentIndexFacts struct {
	VersionID         uint64
	ProfileID         uint64
	SourceFactsSHA256 [sha256.Size]byte
	ParserVersion     string
	ChunkerVersion    string
}

func DocumentIndexIdempotencyKey(facts DocumentIndexFacts) (string, error) {
	if facts.VersionID == 0 || facts.ProfileID == 0 || facts.SourceFactsSHA256 == ([sha256.Size]byte{}) ||
		strings.TrimSpace(facts.ParserVersion) == "" || strings.TrimSpace(facts.ChunkerVersion) == "" {
		return "", errors.New("document index facts are incomplete")
	}
	preimage := fmt.Sprintf("%s\x00%d\x00%d\x00%s\x00%s\x00%s", TaskContextDocumentIndexV1,
		facts.VersionID, facts.ProfileID, hex.EncodeToString(facts.SourceFactsSHA256[:]), facts.ParserVersion, facts.ChunkerVersion)
	digest := sha256.Sum256([]byte(preimage))
	return hex.EncodeToString(digest[:]), nil
}

type QueueDocumentVersionEnqueuer struct{ queue taskqueue.Enqueuer }

func NewDocumentVersionEnqueuer(queue taskqueue.Enqueuer) *QueueDocumentVersionEnqueuer {
	return &QueueDocumentVersionEnqueuer{queue: queue}
}

func (enqueuer *QueueDocumentVersionEnqueuer) EnqueueDocumentVersion(ctx context.Context, versionID uint64) error {
	if enqueuer == nil || enqueuer.queue == nil || versionID == 0 {
		return errors.New("document version enqueuer is not configured")
	}
	payload, err := json.Marshal(ContextDocumentIndexV1{DocumentVersionID: versionID})
	if err != nil {
		return fmt.Errorf("encode document index task: %w", err)
	}
	_, err = enqueuer.queue.Enqueue(ctx, taskqueue.Task{Type: TaskContextDocumentIndexV1, Payload: payload})
	return err
}

type DocumentIndexJobService interface {
	IndexDocument(context.Context, uint64) error
	FinalizeDocumentIndex(context.Context, uint64, string) error
}
