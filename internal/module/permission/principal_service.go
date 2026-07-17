package permission

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"admin_back_go/internal/shared/apperror"
)

const defaultPrincipalSnapshotTTL = 30 * time.Minute

type PrincipalMutation func() ([]PrincipalVersion, error)

type PrincipalMutationCoordinator interface {
	Mutate(context.Context, []PrincipalSubject, PrincipalMutation) error
}

type PrincipalServiceOptions struct {
	SnapshotTTL   time.Duration
	MutationToken func() (string, error)
}

type PrincipalService struct {
	repository    PrincipalRepository
	cache         PrincipalCache
	snapshotTTL   time.Duration
	mutationToken func() (string, error)
}

func NewPrincipalService(repository PrincipalRepository, cache PrincipalCache, options PrincipalServiceOptions) *PrincipalService {
	if options.SnapshotTTL <= 0 {
		options.SnapshotTTL = defaultPrincipalSnapshotTTL
	}
	if options.MutationToken == nil {
		options.MutationToken = newPrincipalMutationToken
	}
	return &PrincipalService{
		repository:    repository,
		cache:         cache,
		snapshotTTL:   options.SnapshotTTL,
		mutationToken: options.MutationToken,
	}
}

func (s *PrincipalService) Authorize(ctx context.Context, userID int64, platform string, code string) *apperror.Error {
	platform = strings.TrimSpace(platform)
	code = strings.TrimSpace(code)
	if userID <= 0 {
		return apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if platform == "" || code == "" {
		return apperror.ForbiddenKey("permission.code_missing", nil, "权限标识未配置")
	}
	if s == nil || s.repository == nil || s.cache == nil {
		return principalUnavailable(ErrPrincipalCacheNotConfigured)
	}

	snapshot, state, err := s.cache.Load(ctx, userID, platform)
	if err != nil {
		return principalUnavailable(err)
	}
	switch state {
	case PrincipalCacheInvalidating:
		return principalInvalidating()
	case PrincipalCacheMiss:
		loaded, loadErr := s.repository.LoadSnapshot(ctx, userID, platform)
		if errors.Is(loadErr, ErrPrincipalNotFound) {
			return apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
		}
		if loadErr != nil {
			return principalRepositoryFailure(loadErr)
		}
		stored, storeErr := s.cache.Store(ctx, loaded, s.snapshotTTL)
		if storeErr != nil {
			return principalUnavailable(storeErr)
		}
		if !stored {
			return principalInvalidating()
		}
		snapshot = &loaded
	case PrincipalCacheHit:
		if snapshot == nil {
			return principalUnavailable(errors.New("principal cache returned an empty hit"))
		}
	default:
		return principalUnavailable(fmt.Errorf("unknown principal cache state %d", state))
	}
	return authorizePrincipalSnapshot(snapshot, code)
}

func authorizePrincipalSnapshot(snapshot *PrincipalSnapshot, code string) *apperror.Error {
	if snapshot == nil || snapshot.UserID <= 0 || snapshot.Version == 0 {
		return apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期")
	}
	if !snapshot.UserActive {
		return apperror.UnauthorizedKey("auth.user_inactive", nil, "用户已停用或删除")
	}
	if !snapshot.RoleActive || snapshot.RoleID <= 0 {
		return apperror.ForbiddenKey("permission.principal_role_inactive", nil, "角色无效或已删除")
	}
	for _, ownedCode := range snapshot.RouteCodes {
		if ownedCode == code {
			return nil
		}
	}
	return apperror.ForbiddenKey("permission.api.denied", nil, "无接口权限")
}

func (s *PrincipalService) Mutate(ctx context.Context, subjects []PrincipalSubject, mutation PrincipalMutation) error {
	if mutation == nil {
		return errors.New("principal mutation callback is required")
	}
	subjects = normalizePrincipalSubjects(subjects)
	if len(subjects) == 0 {
		_, err := mutation()
		return err
	}
	if s == nil || s.repository == nil || s.cache == nil || s.mutationToken == nil {
		return ErrPrincipalCacheNotConfigured
	}
	current, err := s.repository.CurrentVersions(ctx, subjects)
	if err != nil {
		return fmt.Errorf("load current principal versions: %w", err)
	}
	current = normalizePrincipalVersions(current)
	if !sameSubjectsAndVersions(subjects, current) {
		return errors.New("current principal versions do not match mutation subjects")
	}
	token, err := s.mutationToken()
	if err != nil {
		return fmt.Errorf("generate principal invalidation token: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("principal invalidation token is empty")
	}
	if err := s.cache.Begin(ctx, current, token); err != nil {
		return fmt.Errorf("begin principal invalidation: %w", err)
	}

	next, mutationErr := mutation()
	if mutationErr != nil {
		if abortErr := s.cache.Abort(ctx, current, token); abortErr != nil {
			return errors.Join(mutationErr, fmt.Errorf("abort principal invalidation: %w", abortErr))
		}
		return mutationErr
	}
	next = normalizePrincipalVersions(next)
	if err := validateNextPrincipalVersions(current, next); err != nil {
		// The database callback returned success without the required version bump.
		// Keep the gate closed instead of making stale authorization usable.
		return err
	}
	if err := s.cache.Publish(ctx, current, next, token); err != nil {
		return fmt.Errorf("publish principal versions: %w", err)
	}
	return nil
}

func (s *PrincipalService) Reconcile(ctx context.Context) error {
	if s == nil || s.repository == nil || s.cache == nil {
		return ErrPrincipalCacheNotConfigured
	}
	versions, err := s.repository.AllVersions(ctx)
	if err != nil {
		return fmt.Errorf("load principal versions for reconciliation: %w", err)
	}
	if err := s.cache.Reconcile(ctx, versions); err != nil {
		return fmt.Errorf("reconcile principal cache: %w", err)
	}
	return nil
}

func PrincipalSubjects(userIDs []int64, platform string) []PrincipalSubject {
	subjects := make([]PrincipalSubject, 0, len(userIDs))
	for _, userID := range userIDs {
		subjects = append(subjects, PrincipalSubject{UserID: userID, Platform: platform})
	}
	return normalizePrincipalSubjects(subjects)
}

func BumpWith(repository any, ctx context.Context, subjects []PrincipalSubject) ([]PrincipalVersion, error) {
	bumper, ok := repository.(PrincipalVersionBumper)
	if !ok || bumper == nil {
		return nil, errors.New("repository does not support principal version bumps")
	}
	return bumper.BumpPrincipalVersions(ctx, subjects)
}

func sameSubjectsAndVersions(subjects []PrincipalSubject, versions []PrincipalVersion) bool {
	if len(subjects) != len(versions) {
		return false
	}
	for index := range subjects {
		if subjects[index].UserID != versions[index].UserID || subjects[index].Platform != versions[index].Platform || versions[index].Version == 0 {
			return false
		}
	}
	return true
}

func validateNextPrincipalVersions(current, next []PrincipalVersion) error {
	if !samePrincipalVersionSubjects(current, next) {
		return errors.New("principal version subjects changed during mutation")
	}
	for index := range current {
		if next[index].Version <= current[index].Version {
			return fmt.Errorf("principal version did not increase for user %d", current[index].UserID)
		}
	}
	return nil
}

func newPrincipalMutationToken() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func principalUnavailable(cause error) *apperror.Error {
	return apperror.Wrap(
		"permission.principal_cache_unavailable",
		apperror.CategoryDependency,
		0,
		apperror.Retryable,
		"permission.principal_cache_unavailable",
		nil,
		"权限缓存不可用",
		cause,
	)
}

func principalRepositoryFailure(cause error) *apperror.Error {
	return apperror.Wrap(
		"permission.principal_repository_failed",
		apperror.CategoryInternal,
		0,
		apperror.Retryable,
		"permission.principal_repository_failed",
		nil,
		"权限主体读取失败",
		cause,
	)
}

func principalInvalidating() *apperror.Error {
	return apperror.New(
		"permission.principal_invalidating",
		apperror.CategoryAuthorization,
		0,
		apperror.Retryable,
		"permission.principal_invalidating",
		nil,
		"权限正在更新",
	)
}
