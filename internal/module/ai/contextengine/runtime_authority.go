package contextengine

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

type GormAuthoritySnapshotLoader struct {
	platform string
}

func NewAuthoritySnapshotLoader(platform string) *GormAuthoritySnapshotLoader {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" {
		return nil
	}
	return &GormAuthoritySnapshotLoader{platform: platform}
}

func (loader *GormAuthoritySnapshotLoader) ReloadPlanAuthorityInTransaction(ctx context.Context, tx *gorm.DB, expected PlanAuthoritySnapshot) (PlanAuthoritySnapshot, error) {
	if loader == nil || tx == nil {
		return PlanAuthoritySnapshot{}, ErrPlanRepositoryNotConfigured
	}
	current := clonePlanAuthoritySnapshot(expected)
	if err := verifyFingerprintAuthority(ctx, tx, expected.Fingerprint); err != nil {
		if isAuthoritySnapshotConflict(err) {
			return changedAuthoritySnapshot(current), nil
		}
		return PlanAuthoritySnapshot{}, err
	}
	for _, source := range expected.Sources {
		if err := verifySelectedSource(ctx, tx, loader.platform, expected.Fingerprint, source); err != nil {
			if isAuthoritySnapshotConflict(err) {
				return changedAuthoritySnapshot(current), nil
			}
			return PlanAuthoritySnapshot{}, err
		}
	}
	return current, nil
}

func isAuthoritySnapshotConflict(err error) bool {
	return errors.Is(err, ErrInvalidContextValue) ||
		errors.Is(err, ErrInvalidContextPlan) ||
		errors.Is(err, ErrInvalidBudget) ||
		errors.Is(err, ErrInvalidFixedScore) ||
		errors.Is(err, ErrInvalidProfileIndex) ||
		errors.Is(err, ErrInvalidSHA256) ||
		errors.Is(err, ErrIndexGenerationUnavailable) ||
		errors.Is(err, errTurnInvalid) ||
		errors.Is(err, gorm.ErrRecordNotFound)
}

func changedAuthoritySnapshot(snapshot PlanAuthoritySnapshot) PlanAuthoritySnapshot {
	snapshot.Fingerprint.AgentSHA256[0] ^= 0xff
	changed, err := HashInputFingerprint(snapshot.Fingerprint)
	if err == nil {
		snapshot.InputFingerprintSHA256 = changed
	}
	return snapshot
}

var _ AuthoritySnapshotLoader = (*GormAuthoritySnapshotLoader)(nil)
