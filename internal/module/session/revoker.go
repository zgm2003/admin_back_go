package session

import (
	"context"
	"strconv"
	"strings"
)

type RevocationConfig struct {
	RedisPrefix string
}

type RevocationService struct {
	cache Cache
	cfg   RevocationConfig
}

func NewRevocationService(cache Cache, cfg RevocationConfig) *RevocationService {
	if cfg.RedisPrefix == "" {
		cfg.RedisPrefix = "token:"
	}
	return &RevocationService{cache: cache, cfg: cfg}
}

func (s *RevocationService) RevokeCache(ctx context.Context, row Session) error {
	if s == nil || s.cache == nil {
		return ErrCacheNotConfigured
	}

	if strings.TrimSpace(row.AccessTokenHash) != "" {
		if err := s.cache.Del(ctx, s.cacheKey(row.AccessTokenHash)); err != nil {
			return err
		}
	}

	if row.ID <= 0 || row.UserID <= 0 || strings.TrimSpace(row.Platform) == "" {
		return nil
	}

	pointerKey := s.singleSessionPointerKey(row.Platform, row.UserID)
	current, err := s.cache.Get(ctx, pointerKey)
	if err != nil {
		return err
	}
	if sameSessionID(current, row.ID) {
		if err := s.cache.Del(ctx, pointerKey); err != nil {
			return err
		}
	}
	return nil
}

func (s *RevocationService) RevokeCaches(ctx context.Context, rows []Session) error {
	for _, row := range rows {
		if err := s.RevokeCache(ctx, row); err != nil {
			return err
		}
	}
	return nil
}

func (s *RevocationService) cacheKey(tokenHash string) string {
	return s.cfg.RedisPrefix + strings.TrimSpace(tokenHash)
}

func (s *RevocationService) singleSessionPointerKey(platform string, userID int64) string {
	return s.cfg.RedisPrefix + "cur_sess:" + strings.ToLower(strings.TrimSpace(platform)) + ":" + strconv.FormatInt(userID, 10)
}
