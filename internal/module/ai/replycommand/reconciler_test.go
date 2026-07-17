package replycommand

import (
	"context"
	"testing"
)

type fakeOutcomeRepository struct {
	work       *OutcomeUnknownWork
	resolution ReconcileOutcomeInput
}

func (f *fakeOutcomeRepository) NextOutcomeUnknown(context.Context) (*OutcomeUnknownWork, error) {
	return f.work, nil
}

func (f *fakeOutcomeRepository) ResolveOutcomeUnknown(_ context.Context, input ReconcileOutcomeInput) (bool, error) {
	f.resolution = input
	return true, nil
}

func TestReconcilerUsesLocalAssistantAndNeverResendsUnknownAttempt(t *testing.T) {
	repository := &fakeOutcomeRepository{work: &OutcomeUnknownWork{CommandID: 41, AssistantMessageID: 22, ProviderRequestID: "provider-request-1"}}
	reconciler := NewReconciler(ReconcilerOptions{Repository: repository})
	worked, err := reconciler.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	if repository.resolution.CommandID != 41 || repository.resolution.State != StateSucceeded || repository.resolution.AssistantMessageID != 22 || repository.resolution.Content != "" {
		t.Fatalf("resolution=%+v", repository.resolution)
	}
}

func TestReconcilerFailsUnqueryableAcknowledgedAttemptWithoutRetry(t *testing.T) {
	repository := &fakeOutcomeRepository{work: &OutcomeUnknownWork{CommandID: 42, ProviderRequestID: "provider-request-2"}}
	reconciler := NewReconciler(ReconcilerOptions{Repository: repository})
	worked, err := reconciler.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	if repository.resolution.State != StateFailed || repository.resolution.ErrorCode != "ai.provider_outcome_unknown" {
		t.Fatalf("resolution=%+v", repository.resolution)
	}
}

type fakeProviderLookup struct {
	result ProviderLookupResult
}

func (f fakeProviderLookup) Lookup(context.Context, string) (ProviderLookupResult, error) {
	return f.result, nil
}

func TestReconcilerPersistsProviderLookupResult(t *testing.T) {
	repository := &fakeOutcomeRepository{work: &OutcomeUnknownWork{CommandID: 43, ProviderRequestID: "provider-request-3"}}
	reconciler := NewReconciler(ReconcilerOptions{Repository: repository, Lookup: fakeProviderLookup{result: ProviderLookupResult{Status: ProviderLookupFound, Content: "recovered answer"}}})
	worked, err := reconciler.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	if repository.resolution.State != StateSucceeded || repository.resolution.Content != "recovered answer" {
		t.Fatalf("resolution=%+v", repository.resolution)
	}
}
