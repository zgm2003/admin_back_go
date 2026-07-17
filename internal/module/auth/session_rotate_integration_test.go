package auth

import (
	"context"
	"sync"
	"testing"
	"time"
)

type rotateAttempt struct {
	credentials *CredentialSet
	err         string
}

func TestConcurrentRotateHasOneWinner(t *testing.T) {
	resources := openSessionIntegrationResources(t)
	userID := createSessionIntegrationUser(t, resources)
	prefix := "p04:rotate:" + time.Now().Format("150405.000000000") + ":"
	lifecycle := newIntegrationSessionLifecycle(resources, prefix, &AuthPolicy{
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
	})

	issued, appErr := lifecycle.Issue(context.Background(), IssueCommand{
		UserID:    userID,
		Platform:  "admin",
		DeviceID:  "rotate-device",
		ClientIP:  "127.0.0.1",
		UserAgent: "p04-rotate-integration",
	})
	if appErr != nil {
		t.Fatalf("issue initial credentials: %v", appErr)
	}
	before := loadIntegrationSessions(t, resources, userID)
	if len(before) != 1 {
		t.Fatalf("issued sessions = %d, want 1", len(before))
	}

	const attempts = 20
	start := make(chan struct{})
	results := make(chan rotateAttempt, attempts)
	var wait sync.WaitGroup
	for index := 0; index < attempts; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			credentials, rotateErr := lifecycle.Rotate(context.Background(), RotateCommand{
				RefreshToken: issued.RefreshToken,
				ClientIP:     "127.0.0.1",
				UserAgent:    "p04-rotate-racer",
			})
			result := rotateAttempt{credentials: credentials}
			if rotateErr != nil {
				result.err = rotateErr.Code
			}
			results <- result
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	var winner *CredentialSet
	reused := 0
	for result := range results {
		switch {
		case result.credentials != nil && result.err == "":
			if winner != nil {
				t.Fatalf("more than one refresh winner")
			}
			winner = result.credentials
		case result.err == "auth.refresh_reused":
			reused++
		default:
			t.Fatalf("unexpected rotate result: credentials=%#v code=%q", result.credentials, result.err)
		}
	}
	if winner == nil || reused != attempts-1 {
		t.Fatalf("winner=%v reused=%d, want one winner and %d reused", winner != nil, reused, attempts-1)
	}

	after := loadIntegrationSessions(t, resources, userID)
	if len(after) != 1 {
		t.Fatalf("rotated sessions = %d, want 1", len(after))
	}
	wantHash, err := HashToken(winner.RefreshToken, integrationTokenPepper)
	if err != nil {
		t.Fatalf("hash winning refresh credential: %v", err)
	}
	if after[0].RefreshTokenHash != wantHash {
		t.Fatalf("stored refresh hash does not belong to the winner")
	}
	if !after[0].RefreshExpiresAt.Equal(before[0].RefreshExpiresAt) {
		t.Fatalf("refresh expiry changed: before=%s after=%s", before[0].RefreshExpiresAt, after[0].RefreshExpiresAt)
	}

	credentials, reusedErr := lifecycle.Rotate(context.Background(), RotateCommand{RefreshToken: issued.RefreshToken})
	if credentials != nil || reusedErr == nil || reusedErr.Code != "auth.refresh_reused" {
		t.Fatalf("original credential reused result = %#v, %#v", credentials, reusedErr)
	}
}
