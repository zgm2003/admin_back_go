package contextengine

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type historyAttachmentAvailabilityStub struct{ ready bool }

func (stub historyAttachmentAvailabilityStub) HistoricalAttachmentReady(context.Context, uint64, uint64, uint32) (bool, error) {
	return stub.ready, nil
}

type historyPagerFixture struct {
	turns []ConversationTurn
	calls int
}

func TestAttachmentUnavailableStopsOlderHistoryInsteadOfDroppingFile(t *testing.T) {
	turns := make([]ConversationTurn, 0, 17)
	for id := uint64(17); id > 0; id-- {
		turn := ConversationTurn{ConversationID: 7, UserID: 5, AgentID: 9,
			UserMessage:      TurnMessage{ID: id, Role: "user", Content: "question"},
			AssistantMessage: TurnMessage{ID: id + 100, Role: "assistant", Content: "answer"}, AssistantDelivery: "completed"}
		if id == 1 {
			turn.UserMessage.Attachments = []TurnAttachment{{Index: 0, Type: "file", StorageProvider: "cos", ObjectKey: "ai_chat_attachments/report.pdf", ETag: "v1", Size: 10, MIMEType: "application/pdf", Name: "report.pdf"}}
		}
		if err := turn.ComputeSourceSHA256(); err != nil {
			t.Fatal(err)
		}
		turns = append(turns, turn)
	}
	_, err := runtimeHistoryGroups(t.Context(), &historyPagerFixture{turns: turns}, historyAttachmentAvailabilityStub{}, RuntimeInput{
		CurrentMessageID: 100, ConversationID: 7, UserID: 5,
	}, RuntimeFacts{ModelCapability: ModelCapabilityHashInput{TokenCounterID: "utf8_bytes_v1", ContextWindowTokens: 100000}, Budget: Budget{KnownInputBudget: 100000}})
	if !errors.Is(err, ErrAttachmentUnavailable) {
		t.Fatalf("error=%v", err)
	}
}

func TestAttachmentIndexAddsRecentNativeFileToAtomicTurnGroup(t *testing.T) {
	turn := ConversationTurn{ConversationID: 7, UserID: 5, AgentID: 9,
		UserMessage: TurnMessage{ID: 9, Role: "user", Content: "question", Attachments: []TurnAttachment{{
			Index: 0, Type: "file", StorageProvider: "cos", ObjectKey: "ai_chat_attachments/report.pdf", ETag: "v1", Size: 10, MIMEType: "application/pdf", Name: "report.pdf",
		}}}, AssistantMessage: TurnMessage{ID: 10, Role: "assistant", Content: "answer"}, AssistantDelivery: "completed"}
	if err := turn.ComputeSourceSHA256(); err != nil {
		t.Fatal(err)
	}
	groups, err := runtimeHistoryGroups(t.Context(), &historyPagerFixture{turns: []ConversationTurn{turn}}, nil, RuntimeInput{
		CurrentMessageID: 20, ConversationID: 7, UserID: 5,
	}, RuntimeFacts{ModelCapability: ModelCapabilityHashInput{TokenCounterID: "utf8_bytes_v1", ContextWindowTokens: 1000}, Budget: Budget{KnownInputBudget: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Blocks) != 2 || groups[0].Blocks[1].Block.Kind != BlockHistoryAttachment ||
		groups[0].Blocks[1].Block.AtomicGroupKey != groups[0].Blocks[0].Block.AtomicGroupKey {
		t.Fatalf("groups=%+v", groups)
	}
}

func (fixture *historyPagerFixture) PageCompleteBefore(_ context.Context, _ uint64, _ uint64, before *uint64, size int) (ConversationTurnPage, error) {
	fixture.calls++
	page := ConversationTurnPage{Turns: make([]ConversationTurn, 0, size)}
	for _, turn := range fixture.turns {
		if before != nil && turn.UserMessage.ID >= *before {
			continue
		}
		if len(page.Turns) == size {
			oldest := page.Turns[len(page.Turns)-1].UserMessage.ID
			page.NextBeforeUserMessageID = &oldest
			break
		}
		page.Turns = append(page.Turns, turn)
	}
	return page, nil
}

func TestAutomaticHistoryPagesMoreThanFiftyCompleteTurns(t *testing.T) {
	fixture := &historyPagerFixture{turns: make([]ConversationTurn, 0, 60)}
	for id := uint64(60); id > 0; id-- {
		turn := ConversationTurn{ConversationID: 7, UserID: 5, AgentID: 9,
			UserMessage:       TurnMessage{ID: id, Role: "user", Content: fmt.Sprintf("q%d", id)},
			AssistantMessage:  TurnMessage{ID: id + 100, Role: "assistant", Content: fmt.Sprintf("a%d", id)},
			AssistantDelivery: "completed"}
		if err := turn.ComputeSourceSHA256(); err != nil {
			t.Fatal(err)
		}
		fixture.turns = append(fixture.turns, turn)
	}
	groups, err := runtimeHistoryGroups(context.Background(), fixture, nil, RuntimeInput{
		CurrentMessageID: 100, ConversationID: 7, UserID: 5,
	}, RuntimeFacts{
		ModelCapability: ModelCapabilityHashInput{TokenCounterID: "utf8_bytes_v1", ContextWindowTokens: 100000},
		Budget:          Budget{KnownInputBudget: 100000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 60 || fixture.calls != 4 || groups[0].StableSourceID != "conversation_turn:60" || groups[59].StableSourceID != "conversation_turn:1" {
		t.Fatalf("groups=%d calls=%d first=%q last=%q", len(groups), fixture.calls, groups[0].StableSourceID, groups[len(groups)-1].StableSourceID)
	}
}
