package contextengine

import (
	"math"
	"reflect"
	"testing"
)

func TestPackSelectsWholeGroupsInStableOrderAndAssignsCitations(t *testing.T) {
	core := testPackGroup("message:9", true, 1, "1", 9,
		testPackBlock(BlockCurrentUserMessage, "message:9", 4, "question"),
	)
	documentHigh := testPackGroup("document:20", false, 3, "0.9", 20,
		testPackBlock(BlockDocumentEvidence, "chunk:20", 3, "evidence-a"),
		testPackBlock(BlockDocumentEvidence, "chunk:21", 3, "evidence-b"),
	)
	documentLow := testPackGroup("document:30", false, 3, "0.8", 30,
		testPackBlock(BlockDocumentEvidence, "chunk:30", 6, "evidence-c"),
	)
	memory := testPackGroup("memory:4", false, 4, "1", 40,
		testPackBlock(BlockConversationMemory, "memory:4", 4, "memory"),
	)

	packed, appErr := Pack(PackInput{
		KnownInputBudget: 10,
		Candidates:       []PackGroup{memory, documentLow, core, documentHigh},
	})
	if appErr != nil {
		t.Fatal(appErr)
	}
	if packed.KnownInputUpperBound != 10 || len(packed.Items) != 5 {
		t.Fatalf("packed = %#v", packed)
	}
	wantRefs := []string{"message:9", "chunk:20", "chunk:21", "chunk:30", "memory:4"}
	for index, item := range packed.Items {
		if item.Ordinal != uint32(index+1) || item.Block.SourceRef != wantRefs[index] {
			t.Fatalf("item %d = %#v", index, item)
		}
	}
	if packed.Items[0].Decision != DecisionSelected || packed.Items[1].Decision != DecisionSelected || packed.Items[2].Decision != DecisionSelected {
		t.Fatalf("required and highest-ranked group were not selected: %#v", packed.Items)
	}
	if packed.Items[3].Decision != DecisionExcluded || packed.Items[4].Decision != DecisionExcluded {
		t.Fatalf("overflow groups were not excluded: %#v", packed.Items)
	}
	if packed.Items[1].CitationKey == nil || *packed.Items[1].CitationKey != "C1" ||
		packed.Items[2].CitationKey == nil || *packed.Items[2].CitationKey != "C2" || packed.Items[3].CitationKey != nil {
		t.Fatalf("citations = %#v", packed.Items)
	}
	for _, item := range packed.Items[3:] {
		if item.ExclusionReason == nil || *item.ExclusionReason != ExclusionBudgetExceeded || item.Block.ContentSnapshot != nil {
			t.Fatalf("excluded item retained invalid facts: %#v", item)
		}
	}

	reordered, appErr := Pack(PackInput{
		KnownInputBudget: 10,
		Candidates:       []PackGroup{documentHigh, core, memory, documentLow},
	})
	if appErr != nil || !reflect.DeepEqual(reordered, packed) {
		t.Fatalf("candidate input order changed packing: %#v, %v", reordered, appErr)
	}
}

func TestPackRejectsRequiredAndToolContinuationOverflow(t *testing.T) {
	required := testPackGroup("message:9", true, 1, "1", 9,
		testPackBlock(BlockCurrentUserMessage, "message:9", 11, "question"),
	)
	if _, appErr := Pack(PackInput{KnownInputBudget: 10, Candidates: []PackGroup{required}}); appErr == nil || appErr.Code != string(ErrCodeRequiredOverflow) {
		t.Fatalf("required overflow error = %#v", appErr)
	}
	if _, appErr := Pack(PackInput{
		KnownInputBudget:                10,
		ToolContinuationInputReserve:    8,
		ToolContinuationInputUpperBound: 9,
		Candidates:                      []PackGroup{testPackGroup("message:9", true, 1, "1", 9, testPackBlock(BlockCurrentUserMessage, "message:9", 1, "q"))},
	}); appErr == nil || appErr.Code != string(ErrCodeToolContinuationOverflow) {
		t.Fatalf("tool continuation overflow error = %#v", appErr)
	}
}

func TestPackDoesNotInventUnregisteredContextErrorCodes(t *testing.T) {
	_, appErr := Pack(PackInput{})
	if appErr == nil || appErr.Code != "internal.unknown" {
		t.Fatalf("invalid pack input error = %#v", appErr)
	}
}

func TestPackClassifiesRequiredGroupSumOverflowWithoutWrapping(t *testing.T) {
	required := testPackGroup("turn:9", true, 1, "1", 9,
		testPackBlock(BlockRecentTurn, "message:9", math.MaxInt64, "user"),
		testPackBlock(BlockRecentTurn, "message:10", 1, "assistant"),
	)
	if _, appErr := Pack(PackInput{KnownInputBudget: math.MaxInt64, Candidates: []PackGroup{required}}); appErr == nil || appErr.Code != string(ErrCodeRequiredOverflow) {
		t.Fatalf("required group sum overflow error = %#v", appErr)
	}

	core := testPackGroup("message:1", true, 1, "1", 1,
		testPackBlock(BlockCurrentUserMessage, "message:1", 1, "q"),
	)
	optional := required
	optional.Required = false
	optional.Priority = 2
	packed, appErr := Pack(PackInput{KnownInputBudget: math.MaxInt64, Candidates: []PackGroup{optional, core}})
	if appErr != nil {
		t.Fatal(appErr)
	}
	if packed.KnownInputUpperBound != 1 || packed.Items[1].Decision != DecisionExcluded || packed.Items[2].Decision != DecisionExcluded {
		t.Fatalf("overflowing optional group was not excluded: %#v", packed)
	}
}

func FuzzPackerNeverSplitsAtomicGroups(f *testing.F) {
	f.Add(uint16(400), uint8(8))
	f.Fuzz(func(t *testing.T, budget uint16, count uint8) {
		if count > 32 {
			count = 32
		}
		groups := make([]PackGroup, 0, int(count)+1)
		groups = append(groups, testPackGroup("required", true, 1, "1", 1,
			testPackBlock(BlockCurrentUserMessage, "message:1", 1, "q"),
		))
		for index := 0; index < int(count); index++ {
			groups = append(groups, testPackGroup(
				"turn:"+testInt(index+1), false, 5, "0.5", int64(index+1),
				testPackBlock(BlockRecentTurn, "message:"+testInt(index*2+2), int64(index%7+1), "user"),
				testPackBlock(BlockRecentTurn, "message:"+testInt(index*2+3), int64(index%5+1), "assistant"),
			))
		}
		packed, appErr := Pack(PackInput{KnownInputBudget: int64(budget), Candidates: groups})
		if appErr != nil {
			return
		}
		if packed.KnownInputUpperBound > int64(budget) {
			t.Fatalf("upper bound %d exceeds budget %d", packed.KnownInputUpperBound, budget)
		}
		decisions := make(map[string]Decision)
		for _, item := range packed.Items {
			if previous, exists := decisions[item.Block.AtomicGroupKey]; exists && previous != item.Decision {
				t.Fatalf("atomic group %q was split", item.Block.AtomicGroupKey)
			}
			decisions[item.Block.AtomicGroupKey] = item.Decision
		}
		if decisions["required"] != DecisionSelected {
			t.Fatal("required group was not selected")
		}
	})
}

func testPackGroup(id string, required bool, priority int32, relevance string, sourceOrder int64, blocks ...PackBlock) PackGroup {
	score, err := ParseFixedScore(relevance)
	if err != nil {
		panic(err)
	}
	return PackGroup{
		Required: required, Priority: priority, Relevance: &score,
		SourceOrder: sourceOrder, StableSourceID: id, Blocks: blocks,
	}
}

func testPackBlock(kind BlockKind, sourceRef string, tokens int64, content string) PackBlock {
	return PackBlock{Block: ContextBlock{
		Kind: kind, SourceType: "test", SourceRef: sourceRef,
		SourceSHA256: testSHA256(sourceRef), AtomicGroupKey: "unset",
		TokenUpperBound: tokens, ContentSnapshot: stringPointer(content),
		Metadata: ContextBlockMetadataV1{Schema: ContextBlockMetadataSchemaV1},
	}}
}

func testInt(value int) string {
	const digits = "0123456789"
	if value < 10 {
		return string(digits[value])
	}
	return testInt(value/10) + string(digits[value%10])
}
