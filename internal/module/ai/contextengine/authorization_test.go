package contextengine

import (
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/module/ai/replycommand"
	airun "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/enum"
)

func TestPlanAuthoritySnapshotRejectsFingerprintFactsThatDisagreeWithHash(t *testing.T) {
	input := validBuildPlanInput()
	modelHash, err := HashModelCapability(input.ModelCapability)
	if err != nil {
		t.Fatal(err)
	}
	input.Fingerprint.ModelCapabilitySHA256 = modelHash
	fingerprintHash, err := HashInputFingerprint(input.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := PlanAuthoritySnapshot{InputFingerprintSHA256: fingerprintHash, Fingerprint: input.Fingerprint}
	if _, err := HashPlanAuthoritySnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Fingerprint.ModelID = "changed-model"
	if _, err := HashPlanAuthoritySnapshot(snapshot); !errors.Is(err, ErrInvalidPlanCommitToken) {
		t.Fatalf("mismatched fingerprint error=%v", err)
	}
}

func TestLockedPlanAuthorityRejectsLostLeaseCancelAndTerminalRun(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	owner := "worker-a"
	expires := now.Add(time.Minute)
	run := airun.Run{ID: 44, RequestID: "request-1", UserID: 7, AgentID: 5, ProviderID: 7, ModelID: "gpt-5.6", Status: enum.AIRunStatusRunning, ConversationID: int64Pointer(3), UserMessageID: int64Pointer(9)}
	command := replycommand.Command{ID: 77, RequestID: "request-1", UserID: 7, ConversationID: 3, UserMessageID: 9, State: replycommand.StateRunning, LeaseOwner: &owner, LeaseToken: 3, LeaseExpiresAt: &expires}
	token := PlanCommitToken{RunID: 44, ReplyCommandID: 77, LeaseOwner: owner, LeaseToken: 3, InputFingerprintSHA256: testSHA256("input"), AuthoritySnapshotSHA256: testSHA256("authority")}
	if err := validateLockedPlanAuthority(run, command, token, now); err != nil {
		t.Fatal(err)
	}
	command.LeaseToken++
	if err := validateLockedPlanAuthority(run, command, token, now); !errors.Is(err, ErrPlanCommitAborted) {
		t.Fatalf("lease error=%v", err)
	}
	command.LeaseToken = token.LeaseToken
	command.CancelRequestedAt = &now
	if err := validateLockedPlanAuthority(run, command, token, now); !errors.Is(err, ErrPlanCommitAborted) {
		t.Fatalf("cancel error=%v", err)
	}
	command.CancelRequestedAt = nil
	run.Status = enum.AIRunStatusFailed
	if err := validateLockedPlanAuthority(run, command, token, now); !errors.Is(err, ErrPlanCommitAborted) {
		t.Fatalf("run error=%v", err)
	}
}

func int64Pointer(value int64) *int64 { return &value }
