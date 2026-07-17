package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeBrowserGrantStore struct {
	mu     sync.Mutex
	values map[string]string
	err    error
}

func (f *fakeBrowserGrantStore) Put(_ context.Context, key string, value string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if f.values == nil {
		f.values = make(map[string]string)
	}
	f.values[key] = value
	return nil
}

func (f *fakeBrowserGrantStore) Consume(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	value := f.values[key]
	delete(f.values, key)
	return value, nil
}

func (f *fakeBrowserGrantStore) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	return f.values[key], nil
}

func TestRealtimeTicketIsOpaqueBoundAndSingleUse(t *testing.T) {
	store := &fakeBrowserGrantStore{}
	service := NewBrowserGrantService(store, BrowserGrantConfig{
		RedisPrefix:    "test:",
		TokenGenerator: (&sequenceTokenGenerator{values: []string{"opaque-ticket"}}).MakeToken,
	})
	subject := GrantSubject{SessionID: 42, UserID: 7, Platform: "admin"}

	grant, appErr := service.IssueRealtimeTicket(context.Background(), subject)
	if appErr != nil {
		t.Fatalf("issue realtime ticket: %v", appErr)
	}
	if grant.Credential != "opaque-ticket" || grant.ExpiresIn != 30 {
		t.Fatalf("unexpected realtime ticket: %#v", grant)
	}
	consumed, appErr := service.ConsumeRealtimeTicket(context.Background(), grant.Credential)
	if appErr != nil || consumed != subject {
		t.Fatalf("consume realtime ticket: subject=%#v err=%v", consumed, appErr)
	}
	if _, appErr = service.ConsumeRealtimeTicket(context.Background(), grant.Credential); appErr == nil || appErr.Category != "authentication" {
		t.Fatalf("second consume error=%#v, want authentication rejection", appErr)
	}
}

func TestQueueMonitorGrantCanValidateForItsShortLifetime(t *testing.T) {
	store := &fakeBrowserGrantStore{}
	service := NewBrowserGrantService(store, BrowserGrantConfig{
		RedisPrefix:    "test:",
		TokenGenerator: (&sequenceTokenGenerator{values: []string{"queue-grant"}}).MakeToken,
	})
	subject := GrantSubject{SessionID: 84, UserID: 9, Platform: "admin"}

	grant, appErr := service.IssueQueueMonitorGrant(context.Background(), subject)
	if appErr != nil {
		t.Fatalf("issue queue grant: %v", appErr)
	}
	if grant.Credential != "queue-grant" || grant.ExpiresIn != 60 {
		t.Fatalf("unexpected queue grant: %#v", grant)
	}
	for index := 0; index < 2; index++ {
		validated, validateErr := service.ValidateQueueMonitorGrant(context.Background(), grant.Credential)
		if validateErr != nil || validated != subject {
			t.Fatalf("validate queue grant %d: subject=%#v err=%v", index, validated, validateErr)
		}
	}
}

func TestBrowserGrantStoreFailureFailsClosed(t *testing.T) {
	service := NewBrowserGrantService(&fakeBrowserGrantStore{err: errors.New("redis down")}, BrowserGrantConfig{
		TokenGenerator: (&sequenceTokenGenerator{values: []string{"ticket"}}).MakeToken,
	})
	if _, appErr := service.IssueRealtimeTicket(context.Background(), GrantSubject{SessionID: 1, UserID: 2, Platform: "admin"}); appErr == nil {
		t.Fatal("expected Redis failure to reject grant issue")
	}
}
