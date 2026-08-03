package contextengine

import (
	"context"
	"testing"
)

func TestCutoverPreflightReportsEveryUnsafeFact(t *testing.T) {
	repository := cutoverPreflightRepositoryStub{
		replyCommands: 2,
		attemptIDs:    []uint64{11, 12},
		legacy:        map[string]uint64{"ai_knowledge_chunks": 3},
		agents: []CutoverAgentCapability{
			{AgentID: 21, ProviderID: 0, ProviderModelID: 0},
			{AgentID: 22, ProviderID: 4, ProviderModelID: 5, ModelKind: "embedding", APIProtocol: "responses", ContextWindowTokens: 100, MaxOutputTokens: 10, TokenCounterID: "utf8_bytes_v1"},
			{AgentID: 23, ProviderID: 4, ProviderModelID: 6, ModelKind: "chat", APIProtocol: "unknown", ContextWindowTokens: 0, MaxOutputTokens: -1, TokenCounterID: "not-registered"},
		},
	}
	report, err := RunCutoverPreflight(context.Background(), repository)
	if err != nil {
		t.Fatalf("RunCutoverPreflight: %v", err)
	}
	if report.ReplyCommandCount != 2 || report.ChatAttemptCount != 2 || report.CheckedAgentCount != 3 {
		t.Fatalf("report=%+v", report)
	}
	want := map[string]bool{
		"active_reply_commands": false, "active_chat_attempts": false, "legacy_table_not_empty": false,
		"agent_provider_model_missing": false, "agent_model_kind_invalid": false,
		"agent_api_protocol_invalid": false, "agent_model_limits_invalid": false, "agent_token_counter_invalid": false,
	}
	for _, violation := range report.Violations {
		if _, exists := want[violation.Code]; exists {
			want[violation.Code] = true
		}
	}
	for code, found := range want {
		if !found {
			t.Errorf("missing violation %q: %+v", code, report.Violations)
		}
	}
}

func TestCutoverPreflightValidReportHasNoViolations(t *testing.T) {
	report, err := RunCutoverPreflight(context.Background(), cutoverPreflightRepositoryStub{
		legacy: map[string]uint64{},
		agents: []CutoverAgentCapability{{
			AgentID: 21, ProviderID: 4, ProviderModelID: 5, ModelKind: "chat", APIProtocol: "responses",
			ContextWindowTokens: 128000, MaxOutputTokens: 8192, TokenCounterID: "utf8_bytes_v1",
		}},
	})
	if err != nil || len(report.Violations) != 0 || len(report.LegacyTableCounts) != 6 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

type cutoverPreflightRepositoryStub struct {
	replyCommands uint64
	attemptIDs    []uint64
	legacy        map[string]uint64
	agents        []CutoverAgentCapability
}

func (repository cutoverPreflightRepositoryStub) CountActiveReplyCommands(context.Context) (uint64, error) {
	return repository.replyCommands, nil
}
func (repository cutoverPreflightRepositoryStub) ListActiveChatAttemptIDs(context.Context) ([]uint64, error) {
	return append([]uint64(nil), repository.attemptIDs...), nil
}
func (repository cutoverPreflightRepositoryStub) CountLegacyTable(context.Context, string) (uint64, error) {
	for table, count := range repository.legacy {
		if table == "ai_knowledge_chunks" {
			return count, nil
		}
	}
	return 0, nil
}
func (repository cutoverPreflightRepositoryStub) ListEnabledChatAgents(context.Context) ([]CutoverAgentCapability, error) {
	return append([]CutoverAgentCapability(nil), repository.agents...), nil
}
