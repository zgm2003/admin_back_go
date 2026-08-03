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

type DocumentIndexFactsLoader interface {
	LoadDocumentIndexFacts(context.Context, uint64) (DocumentIndexFacts, error)
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

type QueueDocumentVersionEnqueuer struct {
	queue taskqueue.Enqueuer
	facts DocumentIndexFactsLoader
}

func NewDocumentVersionEnqueuer(queue taskqueue.Enqueuer, loaders ...DocumentIndexFactsLoader) *QueueDocumentVersionEnqueuer {
	var loader DocumentIndexFactsLoader
	if len(loaders) > 0 {
		loader = loaders[0]
	}
	return &QueueDocumentVersionEnqueuer{queue: queue, facts: loader}
}

func (enqueuer *QueueDocumentVersionEnqueuer) EnqueueDocumentVersion(ctx context.Context, versionID uint64) error {
	if enqueuer == nil || enqueuer.queue == nil || versionID == 0 {
		return errors.New("document version enqueuer is not configured")
	}
	payload, err := json.Marshal(ContextDocumentIndexV1{DocumentVersionID: versionID})
	if err != nil {
		return fmt.Errorf("encode document index task: %w", err)
	}
	task := taskqueue.Task{Type: TaskContextDocumentIndexV1, Payload: payload}
	if enqueuer.facts != nil {
		facts, err := enqueuer.facts.LoadDocumentIndexFacts(ctx, versionID)
		if err != nil {
			return fmt.Errorf("load document index facts: %w", err)
		}
		if facts.VersionID != versionID {
			return errors.New("document index facts version mismatch")
		}
		task.ID, err = DocumentIndexIdempotencyKey(facts)
		if err != nil {
			return fmt.Errorf("derive document index task identity: %w", err)
		}
	}
	_, err = enqueuer.queue.Enqueue(ctx, task)
	if taskqueue.IsDuplicateTask(err) {
		return nil
	}
	return err
}

type DocumentIndexJobService interface {
	IndexDocument(context.Context, uint64) (DocumentIndexAttempt, error)
	FinalizeDocumentIndex(context.Context, DocumentIndexAttempt, string, int) error
}
