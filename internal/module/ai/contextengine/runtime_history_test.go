package contextengine

import (
	"context"
	"fmt"
	"testing"
)

type historyPagerFixture struct {
	turns []ConversationTurn
	calls int
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
	groups, err := runtimeHistoryGroups(context.Background(), fixture, RuntimeInput{
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
