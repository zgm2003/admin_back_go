package user

import (
	"context"
	"errors"
	"testing"
)

func TestNewRedisAddressDictCacheReturnsNilWithoutRedis(t *testing.T) {
	if got := NewRedisAddressDictCache(nil); got != nil {
		t.Fatalf("expected nil cache for nil redis client, got %#v", got)
	}
}

func TestDecodeAddressDictSnapshotRejectsCorruptJSON(t *testing.T) {
	_, ok, err := decodeAddressDictSnapshot([]byte(`{"version":`))
	if ok {
		t.Fatalf("expected corrupt payload not to be a cache hit")
	}
	if !errors.Is(err, ErrAddressDictCacheCorrupt) {
		t.Fatalf("expected ErrAddressDictCacheCorrupt, got %v", err)
	}
}

func TestDecodeAddressDictSnapshotTreatsVersionMismatchAsMiss(t *testing.T) {
	_, ok, err := decodeAddressDictSnapshot([]byte(`{"version":99,"tree":[],"path_by_id":{}}`))
	if err != nil {
		t.Fatalf("expected no error for version mismatch, got %v", err)
	}
	if ok {
		t.Fatalf("expected version mismatch to be a cache miss")
	}
}

func TestEncodeDecodeAddressDictSnapshotRoundTrip(t *testing.T) {
	input := AddressDictSnapshot{
		Version:          addressDictSnapshotVersion,
		GeneratedAt:      "2026-05-11 10:00:00",
		RowCount:         2,
		SourceMaxUpdated: "2026-03-09 10:56:01",
		Tree: []AddressTreeNode{{
			ID:       1,
			ParentID: 0,
			Label:    "中国",
			Value:    1,
			Children: []AddressTreeNode{{
				ID:       2,
				ParentID: 1,
				Label:    "江苏",
				Value:    2,
			}},
		}},
		PathByID: map[int64][]string{
			1: []string{"中国"},
			2: []string{"中国", "江苏"},
		},
	}

	payload, err := encodeAddressDictSnapshot(input)
	if err != nil {
		t.Fatalf("encodeAddressDictSnapshot returned error: %v", err)
	}

	got, ok, err := decodeAddressDictSnapshot(payload)
	if err != nil {
		t.Fatalf("decodeAddressDictSnapshot returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected cache hit after round trip")
	}
	if got.Version != addressDictSnapshotVersion || got.RowCount != 2 {
		t.Fatalf("snapshot metadata mismatch: %#v", got)
	}
	if len(got.Tree) != 1 || got.Tree[0].Children[0].Label != "江苏" {
		t.Fatalf("tree mismatch: %#v", got.Tree)
	}
	if path := got.PathByID[2]; len(path) != 2 || path[0] != "中国" || path[1] != "江苏" {
		t.Fatalf("path mismatch: %#v", got.PathByID)
	}
}

func TestNilRedisAddressDictCacheMethodsAreNoops(t *testing.T) {
	var cache *RedisAddressDictCache
	ctx := context.Background()

	if _, ok, err := cache.Get(ctx); ok || err != nil {
		t.Fatalf("nil cache Get mismatch: ok=%v err=%v", ok, err)
	}
	if err := cache.Set(ctx, AddressDictSnapshot{}); err != nil {
		t.Fatalf("nil cache Set returned error: %v", err)
	}
	if err := cache.Delete(ctx); err != nil {
		t.Fatalf("nil cache Delete returned error: %v", err)
	}
}
