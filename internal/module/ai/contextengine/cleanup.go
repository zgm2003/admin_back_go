package contextengine

import (
	"context"
	"errors"
	"strings"
	"time"
)

type CleanupKind string

const (
	TaskContextIndexCleanupV1                = "ai:context-index-cleanup:v1"
	CleanupDocumentVersionPoints CleanupKind = "document_version_points"
	CleanupRetiredCollection     CleanupKind = "retired_collection"
)

type IndexCleanupJobService interface {
	CleanupIndex(context.Context, ContextIndexCleanupV1) error
}

type ContextIndexCleanupV1 struct {
	Kind              CleanupKind `json:"kind"`
	ProfileID         uint64      `json:"profile_id"`
	IndexGeneration   uint64      `json:"index_generation"`
	DocumentVersionID uint64      `json:"document_version_id,omitempty"`
	NotBeforeUnixMS   int64       `json:"not_before_unix_ms,omitempty"`
}

func (cleanup ContextIndexCleanupV1) Validate() error {
	if cleanup.ProfileID == 0 || cleanup.IndexGeneration == 0 {
		return errors.New("cleanup profile and generation are required")
	}
	switch cleanup.Kind {
	case CleanupDocumentVersionPoints:
		if cleanup.DocumentVersionID == 0 || cleanup.NotBeforeUnixMS != 0 {
			return errors.New("document point cleanup identity is invalid")
		}
	case CleanupRetiredCollection:
		if cleanup.DocumentVersionID != 0 || cleanup.NotBeforeUnixMS <= 0 {
			return errors.New("retired collection cleanup identity is invalid")
		}
	default:
		return errors.New("cleanup kind is unsupported")
	}
	return nil
}

type CleanupRepository interface {
	FindRebuildProfile(context.Context, uint64) (*ContextProfile, error)
	DocumentVersionVisible(context.Context, uint64, uint64) (bool, error)
}

type IndexCleanupService struct {
	repository CleanupRepository
	index      IndexLifecycle
	prefix     string
	now        func() time.Time
}

func NewIndexCleanupService(repository CleanupRepository, index IndexLifecycle, prefix string, now func() time.Time) *IndexCleanupService {
	if now == nil {
		now = time.Now
	}
	return &IndexCleanupService{repository: repository, index: index, prefix: strings.TrimSpace(prefix), now: now}
}

func (service *IndexCleanupService) CleanupIndex(ctx context.Context, cleanup ContextIndexCleanupV1) error {
	if service == nil || service.repository == nil || service.index == nil || service.prefix == "" {
		return errors.New("context index cleanup service is not configured")
	}
	if err := cleanup.Validate(); err != nil {
		return err
	}
	collection := physicalCollectionName(service.prefix, cleanup.ProfileID, cleanup.IndexGeneration)
	switch cleanup.Kind {
	case CleanupDocumentVersionPoints:
		visible, err := service.repository.DocumentVersionVisible(ctx, cleanup.ProfileID, cleanup.DocumentVersionID)
		if err != nil || visible {
			return err
		}
		return service.index.DeleteDocumentVersionPoints(ctx, collection, cleanup.ProfileID, cleanup.IndexGeneration, cleanup.DocumentVersionID)
	case CleanupRetiredCollection:
		if service.now().UTC().Before(time.UnixMilli(cleanup.NotBeforeUnixMS).UTC()) {
			return errors.New("retired collection grace period has not elapsed")
		}
		profile, err := service.repository.FindRebuildProfile(ctx, cleanup.ProfileID)
		if err != nil {
			return err
		}
		if profile != nil && (equalGeneration(profile.ActiveIndexGeneration, &cleanup.IndexGeneration) || equalGeneration(profile.TargetIndexGeneration, &cleanup.IndexGeneration)) {
			return nil
		}
		aliasTarget, exists, err := service.index.AliasTarget(ctx, profileAliasName(service.prefix, cleanup.ProfileID))
		if err != nil {
			return err
		}
		if exists && aliasTarget == collection {
			return nil
		}
		return service.index.DeleteCollection(ctx, collection)
	}
	return errors.New("cleanup kind is unsupported")
}
