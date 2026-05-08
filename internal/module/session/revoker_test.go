package session

import (
	"context"
	"testing"
	"time"
)

type fakeRevocationCache struct {
	values      map[string]string
	deletedKeys []string
}

func (f *fakeRevocationCache) Get(ctx context.Context, key string) (string, error) {
	return f.values[key], nil
}

func (f *fakeRevocationCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return nil
}

func (f *fakeRevocationCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return nil
}

func (f *fakeRevocationCache) Del(ctx context.Context, key string) error {
	f.deletedKeys = append(f.deletedKeys, key)
	delete(f.values, key)
	return nil
}

func TestRevocationServiceDeletesAccessTokenAndMatchingPointer(t *testing.T) {
	cache := &fakeRevocationCache{values: map[string]string{"token:cur_sess:admin:44": "99"}}
	service := NewRevocationService(cache, RevocationConfig{RedisPrefix: "token:"})

	err := service.RevokeCache(context.Background(), Session{ID: 99, UserID: 44, Platform: "admin", AccessTokenHash: "access-hash"})
	if err != nil {
		t.Fatalf("RevokeCache returned error: %v", err)
	}

	if !revocationContains(cache.deletedKeys, "token:access-hash") {
		t.Fatalf("access cache was not deleted: %#v", cache.deletedKeys)
	}
	if !revocationContains(cache.deletedKeys, "token:cur_sess:admin:44") {
		t.Fatalf("matching pointer was not deleted: %#v", cache.deletedKeys)
	}
}

func TestRevocationServiceKeepsNonMatchingPointer(t *testing.T) {
	cache := &fakeRevocationCache{values: map[string]string{"token:cur_sess:admin:44": "100"}}
	service := NewRevocationService(cache, RevocationConfig{RedisPrefix: "token:"})

	err := service.RevokeCache(context.Background(), Session{ID: 99, UserID: 44, Platform: "admin", AccessTokenHash: "access-hash"})
	if err != nil {
		t.Fatalf("RevokeCache returned error: %v", err)
	}

	if !revocationContains(cache.deletedKeys, "token:access-hash") {
		t.Fatalf("access cache was not deleted: %#v", cache.deletedKeys)
	}
	if revocationContains(cache.deletedKeys, "token:cur_sess:admin:44") {
		t.Fatalf("non-matching pointer must not be deleted: %#v", cache.deletedKeys)
	}
}

func revocationContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
