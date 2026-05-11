package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"admin_back_go/internal/platform/redisclient"

	"github.com/redis/go-redis/v9"
)

const (
	addressDictCacheKey        = "admin_go:dict:address:v1"
	addressDictSnapshotVersion = 1
)

var ErrAddressDictCacheCorrupt = errors.New("address dict cache corrupt")

type AddressDictCache interface {
	Get(ctx context.Context) (AddressDictSnapshot, bool, error)
	Set(ctx context.Context, snapshot AddressDictSnapshot) error
	Delete(ctx context.Context) error
}

type AddressDictSnapshot struct {
	Version          int                `json:"version"`
	GeneratedAt      string             `json:"generated_at"`
	RowCount         int                `json:"row_count"`
	SourceMaxUpdated string             `json:"source_max_updated"`
	Tree             []AddressTreeNode  `json:"tree"`
	PathByID         map[int64][]string `json:"path_by_id"`
}

type RedisAddressDictCache struct {
	client *redisclient.Client
	key    string
}

func NewRedisAddressDictCache(client *redisclient.Client) *RedisAddressDictCache {
	if client == nil || client.Redis == nil {
		return nil
	}
	return &RedisAddressDictCache{client: client, key: addressDictCacheKey}
}

func (c *RedisAddressDictCache) Get(ctx context.Context) (AddressDictSnapshot, bool, error) {
	if c == nil || c.client == nil || c.client.Redis == nil {
		return AddressDictSnapshot{}, false, nil
	}
	payload, err := c.client.Redis.Get(ctx, c.key).Bytes()
	if errors.Is(err, redis.Nil) {
		return AddressDictSnapshot{}, false, nil
	}
	if err != nil {
		return AddressDictSnapshot{}, false, err
	}
	return decodeAddressDictSnapshot(payload)
}

func (c *RedisAddressDictCache) Set(ctx context.Context, snapshot AddressDictSnapshot) error {
	if c == nil || c.client == nil || c.client.Redis == nil {
		return nil
	}
	payload, err := encodeAddressDictSnapshot(snapshot)
	if err != nil {
		return err
	}
	return c.client.Redis.Set(ctx, c.key, payload, 0).Err()
}

func (c *RedisAddressDictCache) Delete(ctx context.Context) error {
	if c == nil || c.client == nil || c.client.Redis == nil {
		return nil
	}
	return c.client.Redis.Del(ctx, c.key).Err()
}

func encodeAddressDictSnapshot(snapshot AddressDictSnapshot) ([]byte, error) {
	snapshot.Version = addressDictSnapshotVersion
	if snapshot.Tree == nil {
		snapshot.Tree = []AddressTreeNode{}
	}
	if snapshot.PathByID == nil {
		snapshot.PathByID = map[int64][]string{}
	}
	return json.Marshal(snapshot)
}

func decodeAddressDictSnapshot(payload []byte) (AddressDictSnapshot, bool, error) {
	var snapshot AddressDictSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return AddressDictSnapshot{}, false, fmt.Errorf("%w: %v", ErrAddressDictCacheCorrupt, err)
	}
	if snapshot.Version != addressDictSnapshotVersion {
		return AddressDictSnapshot{}, false, nil
	}
	if snapshot.Tree == nil {
		snapshot.Tree = []AddressTreeNode{}
	}
	if snapshot.PathByID == nil {
		snapshot.PathByID = map[int64][]string{}
	}
	return snapshot, true, nil
}
