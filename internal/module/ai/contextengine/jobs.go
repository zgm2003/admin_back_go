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

const TaskContextMemoryBuildV1 = "ai:context-memory-build:v1"

type ContextMemoryBuildV1 struct {
	ProfileID        uint64   `json:"profile_id"`
	ProfileSHA256    [32]byte `json:"profile_sha256"`
	ConversationID   uint64   `json:"conversation_id"`
	PreviousMemoryID *uint64  `json:"previous_memory_id,omitempty"`
	FromMessageID    uint64   `json:"from_message_id"`
	ThroughMessageID uint64   `json:"through_message_id"`
	SourceSHA256     [32]byte `json:"source_sha256"`
	PolicyVersion    string   `json:"policy_version"`
}

func (payload ContextMemoryBuildV1) Validate() error {
	if payload.ProfileID == 0 || payload.ConversationID == 0 || payload.FromMessageID == 0 ||
		payload.ThroughMessageID < payload.FromMessageID || payload.ProfileSHA256 == ([32]byte{}) ||
		payload.SourceSHA256 == ([32]byte{}) || payload.PolicyVersion != MemoryPolicyVersionV1 {
		return ErrMemoryInvalid
	}
	if payload.PreviousMemoryID != nil && *payload.PreviousMemoryID == 0 {
		return ErrMemoryInvalid
	}
	return nil
}

type MemoryBuildJobService interface {
	BuildMemory(context.Context, ContextMemoryBuildV1) error
	FinalizeMemory(context.Context, ContextMemoryBuildV1, int) error
}

func MemoryTaskIdentity(payload ContextMemoryBuildV1) (string, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(append([]byte(TaskContextMemoryBuildV1+"\x00"), raw...))
	return hex.EncodeToString(digest[:]), nil
}

type QueueMemoryBuildEnqueuer struct{ queue taskqueue.Enqueuer }

func NewMemoryBuildEnqueuer(queue taskqueue.Enqueuer) *QueueMemoryBuildEnqueuer {
	return &QueueMemoryBuildEnqueuer{queue: queue}
}

func (enqueuer *QueueMemoryBuildEnqueuer) EnqueueMemoryBuild(ctx context.Context, payload ContextMemoryBuildV1) error {
	if enqueuer == nil || enqueuer.queue == nil {
		return errors.New("memory build enqueuer is not configured")
	}
	id, err := MemoryTaskIdentity(payload)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = enqueuer.queue.Enqueue(ctx, taskqueue.Task{Type: TaskContextMemoryBuildV1, ID: id, Payload: raw})
	if taskqueue.IsDuplicateTask(err) {
		return nil
	}
	return err
}

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

type QueueProfileRebuildEnqueuer struct{ queue taskqueue.Enqueuer }

func NewProfileRebuildEnqueuer(queue taskqueue.Enqueuer) *QueueProfileRebuildEnqueuer {
	return &QueueProfileRebuildEnqueuer{queue: queue}
}

func (enqueuer *QueueProfileRebuildEnqueuer) EnqueueProfileRebuild(ctx context.Context, profile ContextProfile) error {
	if enqueuer == nil || enqueuer.queue == nil || profile.ID == 0 || profile.TargetIndexGeneration == nil {
		return errors.New("profile rebuild enqueuer is not configured")
	}
	profileSHA256, err := profileConfigSHA256(profile)
	if err != nil {
		return fmt.Errorf("hash profile rebuild facts: %w", err)
	}
	preimage := fmt.Sprintf("%s\x00%d\x00%s\x00%d", TaskContextProfileRebuildV1,
		profile.ID, hex.EncodeToString(profileSHA256[:]), *profile.TargetIndexGeneration)
	taskID := sha256.Sum256([]byte(preimage))
	payload, err := json.Marshal(ContextProfileRebuildV1{ProfileID: profile.ID})
	if err != nil {
		return fmt.Errorf("encode profile rebuild task: %w", err)
	}
	_, err = enqueuer.queue.Enqueue(ctx, taskqueue.Task{
		ID: hex.EncodeToString(taskID[:]), Type: TaskContextProfileRebuildV1, Payload: payload,
	})
	if taskqueue.IsDuplicateTask(err) {
		return nil
	}
	return err
}
