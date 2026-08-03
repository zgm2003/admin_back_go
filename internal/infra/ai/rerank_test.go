package ai

import (
	"errors"
	"testing"
)

func TestRerankInputRequiresOneBoundedComparableRequest(t *testing.T) {
	input := RerankInput{
		ModelID: "rerank-v1",
		Query:   "refund timing",
		Documents: []RerankDocument{
			{CandidateID: "chunk:1", Text: "three business days"},
			{CandidateID: "chunk:2", Text: "contact support"},
		},
		Capabilities: RerankCapabilities{MaxDocuments: 2, MaxInputTokens: 128, TokenCounterID: TokenCounterUTF8BytesV1},
	}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
	input.Documents[1].CandidateID = input.Documents[0].CandidateID
	if err := input.Validate(); !errors.Is(err, ErrRerankFailed) {
		t.Fatalf("duplicate candidate error=%v", err)
	}
}

func TestRerankResultRequiresExactCandidateSet(t *testing.T) {
	input := RerankInput{
		ModelID: "rerank-v1", Query: "refund",
		Documents:    []RerankDocument{{CandidateID: "a", Text: "A"}, {CandidateID: "b", Text: "B"}},
		Capabilities: RerankCapabilities{MaxDocuments: 2, MaxInputTokens: 128, TokenCounterID: TokenCounterUTF8BytesV1},
	}
	result := RerankResult{ModelID: "rerank-v1", Scores: []RerankScore{{CandidateID: "a", Score: 0.8}, {CandidateID: "b", Score: 0.7}}}
	if err := result.Validate(input); err != nil {
		t.Fatal(err)
	}
	result.Scores[1].CandidateID = "a"
	if err := result.Validate(input); !errors.Is(err, ErrRerankFailed) {
		t.Fatalf("duplicate response candidate error=%v", err)
	}
}
