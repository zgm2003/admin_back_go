package permission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/infra/redisclient"

	"github.com/redis/go-redis/v9"
)

var ErrPrincipalCacheNotConfigured = errors.New("principal cache not configured")

type PrincipalCache interface {
	Load(context.Context, int64, string) (*PrincipalSnapshot, PrincipalCacheState, error)
	Store(context.Context, PrincipalSnapshot, time.Duration) (bool, error)
	Begin(context.Context, []PrincipalVersion, string) error
	Publish(context.Context, []PrincipalVersion, []PrincipalVersion, string) error
	Abort(context.Context, []PrincipalVersion, string) error
	Reconcile(context.Context, []PrincipalVersion) error
}

type PrincipalCacheConfig struct {
	RedisPrefix string
}

type RedisPrincipalCache struct {
	client *redis.Client
	prefix string
}

func NewRedisPrincipalCache(client *redisclient.Client, cfg PrincipalCacheConfig) PrincipalCache {
	if client == nil || client.Redis == nil {
		return nil
	}
	return &RedisPrincipalCache{client: client.Redis, prefix: normalizePrincipalRedisPrefix(cfg.RedisPrefix)}
}

func normalizePrincipalRedisPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix != "" && !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	return prefix
}

var loadPrincipalScript = redis.NewScript(`
local gate = redis.call('HGET', KEYS[1], 'gate')
if gate == 'invalidating' then
  return {2, ''}
end
local version = redis.call('HGET', KEYS[1], 'version')
local role_id = redis.call('HGET', KEYS[1], 'role_id')
if not version or not role_id then
  return {0, ''}
end
local snapshot_key = ARGV[1] .. ARGV[2] .. ':' .. ARGV[3] .. ':' .. role_id .. ':' .. version
local payload = redis.call('GET', snapshot_key)
if not payload then
  return {0, ''}
end
return {1, payload}
`)

func (c *RedisPrincipalCache) Load(ctx context.Context, userID int64, platform string) (*PrincipalSnapshot, PrincipalCacheState, error) {
	if c == nil || c.client == nil {
		return nil, PrincipalCacheMiss, ErrPrincipalCacheNotConfigured
	}
	platform = strings.TrimSpace(platform)
	if userID <= 0 || platform == "" {
		return nil, PrincipalCacheMiss, nil
	}
	result, err := loadPrincipalScript.Run(
		ctx,
		c.client,
		[]string{c.prefix + principalStateKey(userID, platform)},
		c.prefix+"authz:principal:v1:",
		platform,
		strconv.FormatInt(userID, 10),
	).Result()
	if err != nil {
		return nil, PrincipalCacheMiss, err
	}
	status, payload, err := principalCacheScriptResult(result)
	if err != nil {
		return nil, PrincipalCacheMiss, err
	}
	switch PrincipalCacheState(status) {
	case PrincipalCacheInvalidating:
		return nil, PrincipalCacheInvalidating, nil
	case PrincipalCacheHit:
		var snapshot PrincipalSnapshot
		if err := json.Unmarshal([]byte(payload), &snapshot); err != nil || !validPrincipalSnapshot(snapshot, userID, platform) {
			if deleteErr := c.deleteSnapshotForState(ctx, userID, platform); deleteErr != nil {
				if err != nil {
					return nil, PrincipalCacheMiss, errors.Join(err, deleteErr)
				}
				return nil, PrincipalCacheMiss, deleteErr
			}
			return nil, PrincipalCacheMiss, nil
		}
		return &snapshot, PrincipalCacheHit, nil
	default:
		return nil, PrincipalCacheMiss, nil
	}
}

func principalCacheScriptResult(result any) (int, string, error) {
	values, ok := result.([]any)
	if !ok || len(values) != 2 {
		return 0, "", fmt.Errorf("invalid principal cache script result: %#v", result)
	}
	status64, err := scriptInteger(values[0])
	if err != nil {
		return 0, "", err
	}
	payload, _ := values[1].(string)
	return int(status64), payload, nil
}

func scriptInteger(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("invalid Redis integer result: %#v", value)
	}
}

func validPrincipalSnapshot(snapshot PrincipalSnapshot, userID int64, platform string) bool {
	return snapshot.UserID == userID && snapshot.UserID > 0 &&
		snapshot.RoleID >= 0 && snapshot.Platform == platform && snapshot.Version > 0
}

var storePrincipalScript = redis.NewScript(`
local gate = redis.call('HGET', KEYS[1], 'gate')
if gate == 'invalidating' then
  return 2
end
local current_version = redis.call('HGET', KEYS[1], 'version')
local current_role = redis.call('HGET', KEYS[1], 'role_id')
if current_version and (current_version ~= ARGV[1] or current_role ~= ARGV[2]) then
  return 0
end
redis.call('HSET', KEYS[1], 'version', ARGV[1], 'role_id', ARGV[2], 'gate', 'ready')
redis.call('HDEL', KEYS[1], 'token', 'started_at')
redis.call('PSETEX', KEYS[2], ARGV[4], ARGV[3])
return 1
`)

func (c *RedisPrincipalCache) Store(ctx context.Context, snapshot PrincipalSnapshot, ttl time.Duration) (bool, error) {
	if c == nil || c.client == nil {
		return false, ErrPrincipalCacheNotConfigured
	}
	if !validPrincipalSnapshot(snapshot, snapshot.UserID, snapshot.Platform) {
		return false, errors.New("invalid principal snapshot")
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return false, err
	}
	result, err := storePrincipalScript.Run(ctx, c.client, []string{
		c.prefix + principalStateKey(snapshot.UserID, snapshot.Platform),
		c.prefix + PrincipalKey(snapshot.UserID, snapshot.RoleID, snapshot.Platform, snapshot.Version),
	}, strconv.FormatUint(snapshot.Version, 10), strconv.FormatInt(snapshot.RoleID, 10), payload, ttl.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return result == int64(PrincipalCacheHit), nil
}

var beginPrincipalInvalidationScript = redis.NewScript(`
local redis_time = redis.call('TIME')
local now = tonumber(redis_time[1])
for i = 1, #KEYS do
  local offset = 3 + ((i - 1) * 4)
  local gate = redis.call('HGET', KEYS[i], 'gate')
  local stored_version = redis.call('HGET', KEYS[i], 'version')
  local database_version = ARGV[offset + 2]
  if stored_version and tonumber(database_version) < tonumber(stored_version) then
    return i
  end
  if gate == 'invalidating' then
    local started_at = tonumber(redis.call('HGET', KEYS[i], 'started_at') or '0')
    if not stored_version or tonumber(database_version) < tonumber(stored_version) then
      return i
    end
    if tonumber(database_version) == tonumber(stored_version) and (started_at == 0 or now - started_at < 30) then
      return i
    end
  end
end
for i = 1, #KEYS do
  local offset = 3 + ((i - 1) * 4)
  local platform = ARGV[offset]
  local user_id = ARGV[offset + 1]
  local version = ARGV[offset + 2]
  local role_id = ARGV[offset + 3]
  local old_version = redis.call('HGET', KEYS[i], 'version')
  local old_role = redis.call('HGET', KEYS[i], 'role_id')
  if old_version and old_role and (old_version ~= version or old_role ~= role_id) then
    redis.call('DEL', ARGV[2] .. platform .. ':' .. user_id .. ':' .. old_role .. ':' .. old_version)
  end
  redis.call('HSET', KEYS[i], 'version', version, 'role_id', role_id, 'gate', 'invalidating', 'token', ARGV[1], 'started_at', now)
end
return 0
`)

func (c *RedisPrincipalCache) Begin(ctx context.Context, current []PrincipalVersion, token string) error {
	if c == nil || c.client == nil {
		return ErrPrincipalCacheNotConfigured
	}
	current = normalizePrincipalVersions(current)
	if len(current) == 0 {
		return nil
	}
	keys, args := c.mutationScriptInput(current, token)
	blockedIndex, err := beginPrincipalInvalidationScript.Run(ctx, c.client, keys, args...).Int64()
	if err != nil {
		return err
	}
	if blockedIndex != 0 {
		return fmt.Errorf("principal mutation already invalidating at index %d", blockedIndex)
	}
	return nil
}

var publishPrincipalInvalidationScript = redis.NewScript(`
for i = 1, #KEYS do
  if redis.call('HGET', KEYS[i], 'gate') ~= 'invalidating' or redis.call('HGET', KEYS[i], 'token') ~= ARGV[1] then
    return i
  end
end
for i = 1, #KEYS do
  local offset = 3 + ((i - 1) * 4)
  local platform = ARGV[offset]
  local user_id = ARGV[offset + 1]
  local version = ARGV[offset + 2]
  local role_id = ARGV[offset + 3]
  local old_version = redis.call('HGET', KEYS[i], 'version')
  local old_role = redis.call('HGET', KEYS[i], 'role_id')
  if old_version and old_role then
    redis.call('DEL', ARGV[2] .. platform .. ':' .. user_id .. ':' .. old_role .. ':' .. old_version)
  end
  redis.call('HSET', KEYS[i], 'version', version, 'role_id', role_id, 'gate', 'ready')
  redis.call('HDEL', KEYS[i], 'token', 'started_at')
end
return 0
`)

func (c *RedisPrincipalCache) Publish(ctx context.Context, current []PrincipalVersion, next []PrincipalVersion, token string) error {
	if c == nil || c.client == nil {
		return ErrPrincipalCacheNotConfigured
	}
	current = normalizePrincipalVersions(current)
	next = normalizePrincipalVersions(next)
	if len(current) == 0 && len(next) == 0 {
		return nil
	}
	if !samePrincipalVersionSubjects(current, next) {
		return errors.New("principal version subjects changed during mutation")
	}
	keys, args := c.mutationScriptInput(next, token)
	mismatchIndex, err := publishPrincipalInvalidationScript.Run(ctx, c.client, keys, args...).Int64()
	if err != nil {
		return err
	}
	if mismatchIndex != 0 {
		return fmt.Errorf("principal invalidation token mismatch at index %d", mismatchIndex)
	}
	return nil
}

var abortPrincipalInvalidationScript = redis.NewScript(`
for i = 1, #KEYS do
  if redis.call('HGET', KEYS[i], 'gate') == 'invalidating' and redis.call('HGET', KEYS[i], 'token') == ARGV[1] then
    local offset = 3 + ((i - 1) * 4)
    redis.call('HSET', KEYS[i], 'version', ARGV[offset + 2], 'role_id', ARGV[offset + 3], 'gate', 'ready')
    redis.call('HDEL', KEYS[i], 'token', 'started_at')
  end
end
return 0
`)

func (c *RedisPrincipalCache) Abort(ctx context.Context, current []PrincipalVersion, token string) error {
	if c == nil || c.client == nil {
		return ErrPrincipalCacheNotConfigured
	}
	current = normalizePrincipalVersions(current)
	if len(current) == 0 {
		return nil
	}
	keys, args := c.mutationScriptInput(current, token)
	return abortPrincipalInvalidationScript.Run(ctx, c.client, keys, args...).Err()
}

var reconcilePrincipalScript = redis.NewScript(`
local redis_time = redis.call('TIME')
local now = tonumber(redis_time[1])
for i = 1, #KEYS do
  local offset = 2 + ((i - 1) * 4)
  local platform = ARGV[offset]
  local user_id = ARGV[offset + 1]
  local version = ARGV[offset + 2]
  local role_id = ARGV[offset + 3]
  local old_version = redis.call('HGET', KEYS[i], 'version')
  local old_role = redis.call('HGET', KEYS[i], 'role_id')
  local repair = true
  if redis.call('HGET', KEYS[i], 'gate') == 'invalidating' and old_version then
    local started_at = tonumber(redis.call('HGET', KEYS[i], 'started_at') or '0')
    if tonumber(version) < tonumber(old_version) then
      repair = false
    elseif tonumber(version) == tonumber(old_version) and (started_at == 0 or now - started_at < 30) then
      repair = false
    end
  end
  if repair then
    if old_version and old_role and (old_version ~= version or old_role ~= role_id) then
      redis.call('DEL', ARGV[1] .. platform .. ':' .. user_id .. ':' .. old_role .. ':' .. old_version)
    end
    redis.call('HSET', KEYS[i], 'version', version, 'role_id', role_id, 'gate', 'ready')
    redis.call('HDEL', KEYS[i], 'token', 'started_at')
  end
end
return 0
`)

func (c *RedisPrincipalCache) Reconcile(ctx context.Context, versions []PrincipalVersion) error {
	if c == nil || c.client == nil {
		return ErrPrincipalCacheNotConfigured
	}
	versions = normalizePrincipalVersions(versions)
	const batchSize = 200
	for start := 0; start < len(versions); start += batchSize {
		end := start + batchSize
		if end > len(versions) {
			end = len(versions)
		}
		batch := versions[start:end]
		keys := make([]string, 0, len(batch))
		args := []any{c.prefix + "authz:principal:v1:"}
		for _, version := range batch {
			keys = append(keys, c.prefix+principalStateKey(version.UserID, version.Platform))
			args = append(args, version.Platform, strconv.FormatInt(version.UserID, 10), strconv.FormatUint(version.Version, 10), strconv.FormatInt(version.RoleID, 10))
		}
		if err := reconcilePrincipalScript.Run(ctx, c.client, keys, args...).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (c *RedisPrincipalCache) mutationScriptInput(versions []PrincipalVersion, token string) ([]string, []any) {
	keys := make([]string, 0, len(versions))
	args := []any{token, c.prefix + "authz:principal:v1:"}
	for _, version := range versions {
		keys = append(keys, c.prefix+principalStateKey(version.UserID, version.Platform))
		args = append(args, version.Platform, strconv.FormatInt(version.UserID, 10), strconv.FormatUint(version.Version, 10), strconv.FormatInt(version.RoleID, 10))
	}
	return keys, args
}

func normalizePrincipalVersions(versions []PrincipalVersion) []PrincipalVersion {
	bySubject := make(map[string]PrincipalVersion, len(versions))
	for _, version := range versions {
		version.Platform = strings.TrimSpace(version.Platform)
		if version.UserID <= 0 || version.Platform == "" || version.Version == 0 {
			continue
		}
		bySubject[fmt.Sprintf("%s:%d", version.Platform, version.UserID)] = version
	}
	subjects := make([]PrincipalSubject, 0, len(bySubject))
	for _, version := range bySubject {
		subjects = append(subjects, PrincipalSubject{UserID: version.UserID, Platform: version.Platform})
	}
	subjects = normalizePrincipalSubjects(subjects)
	result := make([]PrincipalVersion, 0, len(subjects))
	for _, subject := range subjects {
		result = append(result, bySubject[fmt.Sprintf("%s:%d", subject.Platform, subject.UserID)])
	}
	return result
}

func samePrincipalVersionSubjects(left, right []PrincipalVersion) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].UserID != right[index].UserID || left[index].Platform != right[index].Platform {
			return false
		}
	}
	return true
}

var deletePrincipalSnapshotForStateScript = redis.NewScript(`
local version = redis.call('HGET', KEYS[1], 'version')
local role_id = redis.call('HGET', KEYS[1], 'role_id')
if version and role_id then
  redis.call('DEL', ARGV[1] .. ARGV[2] .. ':' .. ARGV[3] .. ':' .. role_id .. ':' .. version)
end
return 0
`)

func (c *RedisPrincipalCache) deleteSnapshotForState(ctx context.Context, userID int64, platform string) error {
	return deletePrincipalSnapshotForStateScript.Run(ctx, c.client, []string{c.prefix + principalStateKey(userID, platform)}, c.prefix+"authz:principal:v1:", platform, strconv.FormatInt(userID, 10)).Err()
}
