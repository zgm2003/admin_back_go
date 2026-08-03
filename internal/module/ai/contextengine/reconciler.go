package contextengine

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/contextindex"
)

type DocumentIndexReconciler struct {
	repository                 IngestionRepository
	enqueuer                   *QueueDocumentVersionEnqueuer
	batchSize                  int
	maxAttempts                uint32
	consistencyRepository      ProfileIndexConsistencyRepository
	index                      IndexLifecycle
	collectionPrefix           string
	conversationRepository     ConversationIndexRepairRepository
	conversationEnqueuer       *QueueConversationTurnEnqueuer
	conversationAfterRunID     uint64
	conversationDocuments      ConversationDocumentRepairRepository
	conversationEnsurer        ConversationDocumentEnsurer
	conversationAfterMessageID uint64
}

func WithConversationIndexRepair(repository ConversationIndexRepairRepository, enqueuer *QueueConversationTurnEnqueuer) DocumentIndexReconcilerOption {
	return func(reconciler *DocumentIndexReconciler) {
		reconciler.conversationRepository = repository
		reconciler.conversationEnqueuer = enqueuer
	}
}

func WithConversationDocumentRepair(repository ConversationDocumentRepairRepository, ensurer ConversationDocumentEnsurer) DocumentIndexReconcilerOption {
	return func(reconciler *DocumentIndexReconciler) {
		reconciler.conversationDocuments = repository
		reconciler.conversationEnsurer = ensurer
	}
}

type ProfileIndexConsistencyRepository interface {
	ListIndexConsistencyProfiles(context.Context, int) ([]ContextProfile, error)
	CompareAndSwapRebuildIndex(context.Context, ProfileIndexCAS) (bool, error)
	LoadRebuildDocuments(context.Context, uint64) ([]RebuildDocument, error)
}

type DocumentIndexReconcilerOption func(*DocumentIndexReconciler)

func WithProfileIndexConsistency(repository ProfileIndexConsistencyRepository, index IndexLifecycle, collectionPrefix string) DocumentIndexReconcilerOption {
	return func(reconciler *DocumentIndexReconciler) {
		reconciler.consistencyRepository = repository
		reconciler.index = index
		reconciler.collectionPrefix = strings.TrimSpace(collectionPrefix)
	}
}

func NewDocumentIndexReconciler(repository IngestionRepository, enqueuer *QueueDocumentVersionEnqueuer, batchSize int, maxAttempts uint32, options ...DocumentIndexReconcilerOption) *DocumentIndexReconciler {
	reconciler := &DocumentIndexReconciler{repository: repository, enqueuer: enqueuer, batchSize: batchSize, maxAttempts: maxAttempts}
	for _, option := range options {
		if option != nil {
			option(reconciler)
		}
	}
	return reconciler
}

func (reconciler *DocumentIndexReconciler) Reconcile(ctx context.Context, now time.Time) error {
	_, err := reconciler.reconcile(ctx, now)
	return err
}

func (reconciler *DocumentIndexReconciler) RunOnce(ctx context.Context) (bool, error) {
	return reconciler.reconcile(ctx, time.Now().UTC())
}

func (reconciler *DocumentIndexReconciler) reconcile(ctx context.Context, now time.Time) (bool, error) {
	if reconciler == nil || reconciler.repository == nil || reconciler.enqueuer == nil || reconciler.enqueuer.queue == nil || reconciler.batchSize <= 0 || reconciler.maxAttempts == 0 {
		return false, errors.New("document index reconciler is not configured")
	}
	candidates, err := reconciler.repository.ListReconcileCandidates(ctx, now, reconciler.batchSize)
	if err != nil {
		return false, err
	}
	for _, candidate := range candidates {
		if candidate.State == DocumentVersionProcessing && candidate.AttemptCount >= reconciler.maxAttempts {
			attempt := DocumentIndexAttempt{VersionID: candidate.VersionID, LeaseToken: candidate.LeaseToken, AttemptCount: candidate.AttemptCount}
			if err := reconciler.repository.FinalizeVersion(ctx, attempt, "ai.context.index_retry_exhausted", true, now); err != nil && !errors.Is(err, ErrVersionLeaseLost) {
				return false, err
			}
			continue
		}
		if err := reconciler.enqueuer.EnqueueDocumentVersion(ctx, candidate.VersionID); err != nil {
			return false, err
		}
	}
	consistent, err := reconciler.reconcileProfileIndexes(ctx)
	if err != nil {
		return len(candidates) > 0 || consistent, err
	}
	conversationWorked, err := reconciler.reconcileConversationIndexes(ctx)
	if err != nil {
		return len(candidates) > 0 || consistent || conversationWorked, err
	}
	attachmentWorked, err := reconciler.reconcileConversationDocuments(ctx)
	return len(candidates) > 0 || consistent || conversationWorked || attachmentWorked, err
}

func (reconciler *DocumentIndexReconciler) reconcileConversationDocuments(ctx context.Context) (bool, error) {
	if reconciler.conversationDocuments == nil && reconciler.conversationEnsurer == nil {
		return false, nil
	}
	if reconciler.conversationDocuments == nil || reconciler.conversationEnsurer == nil {
		return false, errors.New("conversation document reconciler is not configured")
	}
	messageIDs, next, err := reconciler.conversationDocuments.ListConversationAttachmentMessageIDs(ctx, reconciler.conversationAfterMessageID, reconciler.batchSize)
	if err != nil {
		return false, err
	}
	for _, messageID := range messageIDs {
		if err := reconciler.conversationEnsurer.EnsureConversationDocuments(ctx, messageID); err != nil {
			return false, err
		}
	}
	reconciler.conversationAfterMessageID = next
	return len(messageIDs) > 0, nil
}

func (reconciler *DocumentIndexReconciler) reconcileConversationIndexes(ctx context.Context) (bool, error) {
	if reconciler.conversationRepository == nil && reconciler.conversationEnqueuer == nil {
		return false, nil
	}
	if reconciler.conversationRepository == nil || reconciler.conversationEnqueuer == nil {
		return false, errors.New("conversation index reconciler is not configured")
	}
	payloads, next, err := reconciler.conversationRepository.ListConversationIndexPayloads(ctx, reconciler.conversationAfterRunID, reconciler.batchSize)
	if err != nil {
		return false, err
	}
	for _, payload := range payloads {
		if err := reconciler.conversationEnqueuer.EnqueueConversationTurn(ctx, payload); err != nil {
			return false, err
		}
	}
	reconciler.conversationAfterRunID = next
	return len(payloads) > 0, nil
}

func (reconciler *DocumentIndexReconciler) reconcileProfileIndexes(ctx context.Context) (bool, error) {
	if reconciler.consistencyRepository == nil && reconciler.index == nil && reconciler.collectionPrefix == "" {
		return false, nil
	}
	if reconciler.consistencyRepository == nil || reconciler.index == nil || reconciler.collectionPrefix == "" {
		return false, errors.New("profile index consistency reconciler is not configured")
	}
	profiles, err := reconciler.consistencyRepository.ListIndexConsistencyProfiles(ctx, reconciler.batchSize)
	if err != nil {
		return false, err
	}
	worked := false
	for _, profile := range profiles {
		changed, err := reconciler.reconcileProfileIndex(ctx, profile)
		if err != nil {
			return worked, err
		}
		worked = worked || changed
	}
	return worked, nil
}

func (reconciler *DocumentIndexReconciler) reconcileProfileIndex(ctx context.Context, profile ContextProfile) (bool, error) {
	current, err := profileIndexSnapshot(profile)
	if err != nil {
		return false, err
	}
	alias := profileAliasName(reconciler.collectionPrefix, profile.ID)
	aliasTarget, aliasExists, err := reconciler.index.AliasTarget(ctx, alias)
	if err != nil {
		return false, err
	}
	switch current.State {
	case ProfileIndexReady:
		active := *current.ActiveGeneration
		collection := physicalCollectionName(reconciler.collectionPrefix, profile.ID, active)
		if err := reconciler.verifyProfileGeneration(ctx, profile, active); err != nil {
			return reconciler.failProfileIndex(ctx, profile.ID, current)
		}
		if !aliasExists || aliasTarget != collection {
			return true, reconciler.index.SwitchAlias(ctx, alias, collection)
		}
		return false, nil
	case ProfileIndexProvisioning, ProfileIndexRebuilding:
		target := *current.TargetGeneration
		targetCollection := physicalCollectionName(reconciler.collectionPrefix, profile.ID, target)
		if aliasExists && aliasTarget == targetCollection {
			if err := reconciler.verifyProfileGeneration(ctx, profile, target); err != nil {
				return reconciler.restoreHealthyActive(ctx, profile, current, alias)
			}
			next := ProfileIndex{State: ProfileIndexReady, ActiveGeneration: rebuildUint64Pointer(target)}
			if err := current.ValidateTransition(next); err != nil {
				return false, err
			}
			changed, err := reconciler.consistencyRepository.CompareAndSwapRebuildIndex(ctx, ProfileIndexCAS{ID: profile.ID, Expected: current, Next: next})
			return changed, err
		}
		if current.ActiveGeneration == nil {
			return false, nil
		}
		active := *current.ActiveGeneration
		activeCollection := physicalCollectionName(reconciler.collectionPrefix, profile.ID, active)
		if err := reconciler.verifyProfileGeneration(ctx, profile, active); err != nil {
			return reconciler.failProfileIndex(ctx, profile.ID, current)
		}
		if !aliasExists || aliasTarget != activeCollection {
			return true, reconciler.index.SwitchAlias(ctx, alias, activeCollection)
		}
		return false, nil
	default:
		return false, nil
	}
}

func (reconciler *DocumentIndexReconciler) verifyProfileGeneration(ctx context.Context, profile ContextProfile, generation uint64) error {
	collection := physicalCollectionName(reconciler.collectionPrefix, profile.ID, generation)
	active := contextindex.ActiveCollection{ProfileID: profile.ID, IndexGeneration: generation,
		DenseDimensions: uint64(profile.EmbeddingDimensions), DenseDistance: contextindex.Distance(profile.DenseDistance)}
	if err := reconciler.index.VerifyCollection(ctx, collection, active); err != nil {
		return err
	}
	documents, err := reconciler.consistencyRepository.LoadRebuildDocuments(ctx, profile.ID)
	if err != nil {
		return err
	}
	for _, document := range documents {
		work := document.Work
		work.Profile = profile
		work.IndexGeneration = generation
		for start := 0; start < len(document.Chunks); start += documentEmbeddingBatch {
			end := min(start+documentEmbeddingBatch, len(document.Chunks))
			refs := make([]contextindex.PointRef, end-start)
			for index, chunk := range document.Chunks[start:end] {
				ref, err := documentChunkPointRef(work, chunk)
				if err != nil {
					return err
				}
				refs[index] = ref
			}
			if err := reconciler.index.VerifyPoints(ctx, collection, refs, profile.EmbeddingDimensions); err != nil {
				return err
			}
		}
	}
	return nil
}

func (reconciler *DocumentIndexReconciler) restoreHealthyActive(ctx context.Context, profile ContextProfile, current ProfileIndex, alias string) (bool, error) {
	if current.ActiveGeneration == nil {
		return reconciler.failProfileIndex(ctx, profile.ID, current)
	}
	active := *current.ActiveGeneration
	if err := reconciler.verifyProfileGeneration(ctx, profile, active); err != nil {
		return reconciler.failProfileIndex(ctx, profile.ID, current)
	}
	if err := reconciler.index.SwitchAlias(ctx, alias, physicalCollectionName(reconciler.collectionPrefix, profile.ID, active)); err != nil {
		return false, err
	}
	code := ErrCodeIndexFailed
	next := ProfileIndex{State: ProfileIndexReady, ActiveGeneration: rebuildUint64Pointer(active), ErrorCode: &code}
	if err := current.ValidateTransition(next); err != nil {
		return false, err
	}
	changed, err := reconciler.consistencyRepository.CompareAndSwapRebuildIndex(ctx, ProfileIndexCAS{ID: profile.ID, Expected: current, Next: next})
	return changed, err
}

func (reconciler *DocumentIndexReconciler) failProfileIndex(ctx context.Context, profileID uint64, current ProfileIndex) (bool, error) {
	code := ErrCodeIndexInconsistent
	next := ProfileIndex{State: ProfileIndexFailed, ActiveGeneration: cloneUint64(current.ActiveGeneration),
		TargetGeneration: cloneUint64(current.TargetGeneration), ErrorCode: &code}
	if err := current.ValidateTransition(next); err != nil {
		return false, err
	}
	changed, err := reconciler.consistencyRepository.CompareAndSwapRebuildIndex(ctx, ProfileIndexCAS{ID: profileID, Expected: current, Next: next})
	return changed, err
}
