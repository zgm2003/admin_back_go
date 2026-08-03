package contextengine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"admin_back_go/internal/infra/contextindex"
)

var (
	ErrProfileRebuildNotConfigured = errors.New("context profile rebuild service is not configured")
	ErrProfileRebuildConflict      = errors.New("context profile rebuild snapshot changed")
	ErrProfileIndexInconsistent    = errors.New("context profile index pointers are inconsistent")
)

const TaskContextProfileRebuildV1 = "ai:context-profile-rebuild:v1"

type ContextProfileRebuildV1 struct {
	ProfileID uint64 `json:"profile_id"`
}

type ProfileRebuildJobService interface {
	RebuildProfile(context.Context, uint64) error
}

type RebuildDocument struct {
	Work   DocumentIndexWork
	Chunks []PersistedChunk
}

type RebuildRepository interface {
	FindRebuildProfile(context.Context, uint64) (*ContextProfile, error)
	CompareAndSwapRebuildIndex(context.Context, ProfileIndexCAS) (bool, error)
	LoadRebuildDocuments(context.Context, uint64) ([]RebuildDocument, error)
	ActiveDocumentVersionIDs(context.Context, uint64) ([]uint64, error)
}

type IndexLifecycle interface {
	EnsureCollection(context.Context, contextindex.CollectionSpec) error
	VerifyCollection(context.Context, string, contextindex.ActiveCollection) error
	SwitchAlias(context.Context, string, string) error
	AliasTarget(context.Context, string) (string, bool, error)
	DeleteCollection(context.Context, string) error
	DeleteDocumentVersionPoints(context.Context, string, uint64, uint64, uint64) error
	Upsert(context.Context, string, []contextindex.IndexedPoint) error
	VerifyPoints(context.Context, string, []contextindex.PointRef, uint32) error
}

type RebuildDependencies struct {
	Repository       RebuildRepository
	Embeddings       EmbeddingResolver
	Index            IndexLifecycle
	CollectionPrefix string
	Now              func() time.Time
}

type ProfileRebuildService struct{ deps RebuildDependencies }

func NewProfileRebuildService(deps RebuildDependencies) *ProfileRebuildService {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &ProfileRebuildService{deps: deps}
}

func (service *ProfileRebuildService) RebuildProfile(ctx context.Context, profileID uint64) error {
	if service == nil || service.deps.Repository == nil || service.deps.Embeddings == nil || service.deps.Index == nil ||
		profileID == 0 || strings.TrimSpace(service.deps.CollectionPrefix) == "" {
		return ErrProfileRebuildNotConfigured
	}
	profile, err := service.deps.Repository.FindRebuildProfile(ctx, profileID)
	if err != nil {
		return err
	}
	if profile == nil {
		return errors.New("context profile does not exist")
	}
	expected, rebuilding, err := beginProfileRebuild(*profile)
	if err != nil {
		return err
	}
	if expected != rebuilding {
		changed, err := service.deps.Repository.CompareAndSwapRebuildIndex(ctx, ProfileIndexCAS{ID: profile.ID, Expected: expected, Next: rebuilding})
		if err != nil {
			return err
		}
		if !changed {
			return ErrProfileRebuildConflict
		}
		profile.IndexState = rebuilding.State
		profile.ActiveIndexGeneration = cloneUint64(rebuilding.ActiveGeneration)
		profile.TargetIndexGeneration = cloneUint64(rebuilding.TargetGeneration)
	}
	target := *rebuilding.TargetGeneration
	collection := physicalCollectionName(service.deps.CollectionPrefix, profile.ID, target)
	alias := profileAliasName(service.deps.CollectionPrefix, profile.ID)
	switched := false
	fail := func(cause error) error {
		if !switched {
			_ = service.deps.Index.DeleteCollection(context.WithoutCancel(ctx), collection)
			_ = service.failRebuild(context.WithoutCancel(ctx), profile.ID, rebuilding)
		}
		return cause
	}

	spec, err := contextindex.NewCollectionSpec(collection, uint64(profile.EmbeddingDimensions), contextindex.Distance(profile.DenseDistance))
	if err != nil {
		return fail(err)
	}
	if err := service.deps.Index.EnsureCollection(ctx, spec); err != nil {
		return fail(err)
	}
	documents, err := service.deps.Repository.LoadRebuildDocuments(ctx, profile.ID)
	if err != nil {
		return fail(err)
	}
	snapshot := rebuildVersionIDs(documents)
	if err := service.populate(ctx, *profile, target, collection, documents); err != nil {
		return fail(err)
	}
	current, err := service.deps.Repository.ActiveDocumentVersionIDs(ctx, profile.ID)
	if err != nil || !slices.Equal(snapshot, current) {
		if err == nil {
			err = ErrProfileRebuildConflict
		}
		return fail(err)
	}
	active := contextindex.ActiveCollection{ProfileID: profile.ID, IndexGeneration: target, DenseDimensions: uint64(profile.EmbeddingDimensions), DenseDistance: contextindex.Distance(profile.DenseDistance)}
	if err := service.deps.Index.VerifyCollection(ctx, collection, active); err != nil {
		return fail(err)
	}
	if err := service.deps.Index.SwitchAlias(ctx, alias, collection); err != nil {
		return fail(err)
	}
	switched = true
	ready := ProfileIndex{State: ProfileIndexReady, ActiveGeneration: rebuildUint64Pointer(target)}
	changed, err := service.deps.Repository.CompareAndSwapRebuildIndex(ctx, ProfileIndexCAS{ID: profile.ID, Expected: rebuilding, Next: ready})
	if err != nil {
		return err
	}
	if !changed {
		return ErrProfileIndexInconsistent
	}
	return nil
}

func (service *ProfileRebuildService) populate(ctx context.Context, profile ContextProfile, generation uint64, collection string, documents []RebuildDocument) error {
	client, err := service.deps.Embeddings.ResolveEmbedding(ctx, profile)
	if err != nil {
		return err
	}
	for _, document := range documents {
		work := document.Work
		work.IndexGeneration = generation
		for start := 0; start < len(document.Chunks); start += documentEmbeddingBatch {
			end := min(start+documentEmbeddingBatch, len(document.Chunks))
			texts := make([]string, end-start)
			for i, chunk := range document.Chunks[start:end] {
				texts[i] = chunk.Chunk.IndexText
			}
			result, err := client.Embed(ctx, texts)
			if err != nil {
				return err
			}
			if len(result.Vectors) != len(texts) {
				return ErrEmbeddingDimension
			}
			points := make([]contextindex.IndexedPoint, len(texts))
			refs := make([]contextindex.PointRef, len(texts))
			for i, vector := range result.Vectors {
				if len(vector) != int(profile.EmbeddingDimensions) {
					return ErrEmbeddingDimension
				}
				point, err := documentChunkPoint(work, document.Chunks[start+i], vector)
				if err != nil {
					return err
				}
				points[i], refs[i] = point, point.Metadata.Ref
			}
			if err := service.deps.Index.Upsert(ctx, collection, points); err != nil {
				return err
			}
			if err := service.deps.Index.VerifyPoints(ctx, collection, refs, profile.EmbeddingDimensions); err != nil {
				return err
			}
		}
	}
	return nil
}

func (service *ProfileRebuildService) failRebuild(ctx context.Context, profileID uint64, rebuilding ProfileIndex) error {
	code := ErrCodeIndexFailed
	if rebuilding.ActiveGeneration != nil {
		next := ProfileIndex{State: ProfileIndexReady, ActiveGeneration: cloneUint64(rebuilding.ActiveGeneration), ErrorCode: &code}
		_, err := service.deps.Repository.CompareAndSwapRebuildIndex(ctx, ProfileIndexCAS{ID: profileID, Expected: rebuilding, Next: next})
		return err
	}
	next := ProfileIndex{State: ProfileIndexFailed, TargetGeneration: cloneUint64(rebuilding.TargetGeneration), ErrorCode: &code}
	_, err := service.deps.Repository.CompareAndSwapRebuildIndex(ctx, ProfileIndexCAS{ID: profileID, Expected: rebuilding, Next: next})
	return err
}

func beginProfileRebuild(profile ContextProfile) (ProfileIndex, ProfileIndex, error) {
	current, err := profileIndexSnapshot(profile)
	if err != nil {
		return ProfileIndex{}, ProfileIndex{}, err
	}
	switch current.State {
	case ProfileIndexProvisioning, ProfileIndexRebuilding:
		return current, current, nil
	case ProfileIndexReady:
		target := *current.ActiveGeneration + 1
		return current, ProfileIndex{State: ProfileIndexRebuilding, ActiveGeneration: cloneUint64(current.ActiveGeneration), TargetGeneration: &target}, nil
	case ProfileIndexFailed:
		var target uint64 = 1
		if current.ActiveGeneration != nil {
			target = *current.ActiveGeneration + 1
		}
		if current.TargetGeneration != nil && target <= *current.TargetGeneration {
			target = *current.TargetGeneration + 1
		}
		return current, ProfileIndex{State: ProfileIndexRebuilding, ActiveGeneration: cloneUint64(current.ActiveGeneration), TargetGeneration: &target}, nil
	default:
		return ProfileIndex{}, ProfileIndex{}, ErrInvalidProfileIndex
	}
}

func profileIndexSnapshot(profile ContextProfile) (ProfileIndex, error) {
	current := ProfileIndex{State: profile.IndexState, ActiveGeneration: cloneUint64(profile.ActiveIndexGeneration), TargetGeneration: cloneUint64(profile.TargetIndexGeneration)}
	if profile.IndexErrorCode != nil {
		code := ErrorCode(*profile.IndexErrorCode)
		current.ErrorCode = &code
	}
	return current, current.Validate()
}

func rebuildVersionIDs(documents []RebuildDocument) []uint64 {
	ids := make([]uint64, len(documents))
	for i, document := range documents {
		ids[i] = document.Work.Version.ID
	}
	slices.Sort(ids)
	return ids
}

func profileAliasName(prefix string, profileID uint64) string {
	return fmt.Sprintf("%s_profile_%d", strings.TrimSpace(prefix), profileID)
}

func physicalCollectionName(prefix string, profileID, generation uint64) string {
	return fmt.Sprintf("%s_g%d", profileAliasName(prefix, profileID), generation)
}

func rebuildUint64Pointer(value uint64) *uint64 { return &value }
