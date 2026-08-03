package contextengine

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
)

type memoryRepositoryFake struct {
	snapshot  MemoryBuildSnapshot
	candidate MemoryCandidate
	commits   int
}

func (fake *memoryRepositoryFake) LoadMemoryBuild(context.Context, ContextMemoryBuildV1) (MemoryBuildSnapshot, error) {
	return fake.snapshot, nil
}
func (fake *memoryRepositoryFake) CommitMemory(_ context.Context, _ ContextMemoryBuildV1, candidate MemoryCandidate) (MemoryRecord, MemoryCommitDisposition, error) {
	fake.candidate, fake.commits = candidate, fake.commits+1
	return MemoryRecord{ID: 1}, MemoryCommitCreated, nil
}

type memorySummarizerFake struct{}

func (memorySummarizerFake) Summarize(context.Context, MemorySummaryRequest) (MemorySummaryResult, error) {
	return MemorySummaryResult{Summary: "stable summary", PromptTokens: 4, CompletionTokens: 3}, nil
}

type failingMemorySummarizer struct{ err error }

func (summarizer failingMemorySummarizer) Summarize(context.Context, MemorySummaryRequest) (MemorySummaryResult, error) {
	return MemorySummaryResult{}, summarizer.err
}

func TestMemoryWatermarkBuildsOnlyAboveHighWatermark(t *testing.T) {
	if window, ok := MemoryWindow(24, 100); ok || window != (MemoryBuildWindow{}) {
		t.Fatalf("below watermark should not build: %#v %v", window, ok)
	}
	window, ok := MemoryWindow(26, 100)
	if !ok || window.HighWatermarkTokens != 25 || window.TargetTokens != 12 {
		t.Fatalf("window=%#v ok=%v", window, ok)
	}
}

func TestMemorySourceHashIncludesParentAndOrderedTurns(t *testing.T) {
	profile := sha256.Sum256([]byte("profile"))
	turn := testConversationTurn()
	first, err := MemorySourceSHA256(MemorySourceInput{ProfileID: 7, ProfileSHA256: profile, ConversationID: turn.ConversationID, Turns: []ConversationTurn{turn}})
	if err != nil {
		t.Fatal(err)
	}
	turn.UserMessage.Content = "changed"
	second, err := MemorySourceSHA256(MemorySourceInput{ProfileID: 7, ProfileSHA256: profile, ConversationID: turn.ConversationID, Turns: []ConversationTurn{turn}})
	if err != nil || first == second {
		t.Fatalf("turn facts must change source hash: %x %x %v", first, second, err)
	}
	parent := uint64(11)
	third, err := MemorySourceSHA256(MemorySourceInput{ProfileID: 7, ProfileSHA256: profile, ConversationID: turn.ConversationID, ParentMemoryID: &parent, ParentSummarySHA256: sha256.Sum256([]byte("summary")), Turns: []ConversationTurn{testConversationTurn()}})
	if err != nil || first == third {
		t.Fatalf("parent identity must change source hash: %x %x %v", first, third, err)
	}
}

func TestMemoryCandidateRejectsPreassignedIDAndInvalidParent(t *testing.T) {
	candidate := MemoryCandidate{ID: 8, ConversationID: 9, ProfileID: 7, FromMessageID: 10, ThroughMessageID: 20, SourceSHA256: sha256.Sum256([]byte("source")), PolicyVersion: MemoryPolicyVersionV1, State: MemoryStateReady, Summary: "ok", SummarySHA256: sha256.Sum256([]byte("ok"))}
	if err := candidate.ValidateForInsert(); !errors.Is(err, ErrMemoryIDMustBeZero) {
		t.Fatalf("preassigned id error=%v", err)
	}
	candidate.ID = 0
	candidate.ParentMemoryID = &candidate.ID
	if err := candidate.ValidateForInsert(); !errors.Is(err, ErrMemorySelfParent) {
		t.Fatalf("self parent error=%v", err)
	}
}

func TestMemoryCandidateFailedHasNoEmptySummary(t *testing.T) {
	candidate := MemoryCandidate{ConversationID: 9, ProfileID: 7, ProfileSHA256: sha256.Sum256([]byte("profile")), FromMessageID: 10, ThroughMessageID: 20, SourceSHA256: sha256.Sum256([]byte("source")), PolicyVersion: MemoryPolicyVersionV1, State: MemoryStateFailed, ErrorCode: "provider_failed"}
	if err := candidate.ValidateForInsert(); err != nil {
		t.Fatal(err)
	}
	candidate.Summary = " "
	if err := candidate.ValidateForInsert(); err == nil {
		t.Fatal("failed memory must not accept an empty summary")
	}
}

func TestMemoryServiceCommitsOnlyAfterSourceHashAndSummaryValidation(t *testing.T) {
	turn := testConversationTurn()
	profile := sha256.Sum256([]byte("profile"))
	source, err := MemorySourceSHA256(MemorySourceInput{ProfileID: 7, ProfileSHA256: profile, ConversationID: turn.ConversationID, Turns: []ConversationTurn{turn}})
	if err != nil {
		t.Fatal(err)
	}
	fake := &memoryRepositoryFake{snapshot: MemoryBuildSnapshot{Turns: []ConversationTurn{turn}, Prompt: "prompt", MemoryProviderModelID: 33, MemoryMaxOutputTokens: 1024}}
	service := NewMemoryService(MemoryServiceDependencies{Repository: fake, Summarizer: memorySummarizerFake{}})
	payload := ContextMemoryBuildV1{ProfileID: 7, ProfileSHA256: profile, ConversationID: turn.ConversationID, FromMessageID: turn.UserMessage.ID,
		ThroughMessageID: turn.UserMessage.ID, SourceSHA256: source, PolicyVersion: MemoryPolicyVersionV1}
	if err := service.BuildMemory(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if fake.commits != 1 || fake.candidate.State != MemoryStateReady || fake.candidate.Summary != "stable summary" {
		t.Fatalf("candidate=%#v commits=%d", fake.candidate, fake.commits)
	}
}

func TestMemoryServicePersistsPermanentFailureButNotTransientFailure(t *testing.T) {
	payload, snapshot := memoryServiceFixture(t)
	permanentRepository := &memoryRepositoryFake{snapshot: snapshot}
	service := NewMemoryService(MemoryServiceDependencies{Repository: permanentRepository, Summarizer: failingMemorySummarizer{err: infraai.ErrUnauthorized}})
	var permanent *MemoryPermanentError
	if err := service.BuildMemory(context.Background(), payload); !errors.As(err, &permanent) {
		t.Fatalf("permanent error=%v", err)
	}
	if permanentRepository.commits != 1 || permanentRepository.candidate.State != MemoryStateFailed {
		t.Fatalf("candidate=%#v commits=%d", permanentRepository.candidate, permanentRepository.commits)
	}

	transientRepository := &memoryRepositoryFake{snapshot: snapshot}
	transient := errors.New("temporary network failure")
	service = NewMemoryService(MemoryServiceDependencies{Repository: transientRepository, Summarizer: failingMemorySummarizer{err: transient}})
	if err := service.BuildMemory(context.Background(), payload); !errors.Is(err, transient) {
		t.Fatalf("transient error=%v", err)
	}
	if transientRepository.commits != 0 {
		t.Fatal("transient failure must remain retryable without a terminal row")
	}
}

func memoryServiceFixture(t *testing.T) (ContextMemoryBuildV1, MemoryBuildSnapshot) {
	t.Helper()
	turn := testConversationTurn()
	profile := sha256.Sum256([]byte("profile"))
	source, err := MemorySourceSHA256(MemorySourceInput{ProfileID: 7, ProfileSHA256: profile, ConversationID: turn.ConversationID, Turns: []ConversationTurn{turn}})
	if err != nil {
		t.Fatal(err)
	}
	return ContextMemoryBuildV1{ProfileID: 7, ProfileSHA256: profile, ConversationID: turn.ConversationID, FromMessageID: turn.UserMessage.ID,
		ThroughMessageID: turn.UserMessage.ID, SourceSHA256: source, PolicyVersion: MemoryPolicyVersionV1}, MemoryBuildSnapshot{Turns: []ConversationTurn{turn}, Prompt: "prompt", MemoryProviderModelID: 33, MemoryMaxOutputTokens: 1024}
}
