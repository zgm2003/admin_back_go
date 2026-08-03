package contextengine

import "testing"

func TestToolGroupRequiresPairedResult(t *testing.T) {
	callID := "call-1"
	if _, complete := turnToolGroups([]conversationToolRow{{
		ID: 1, RunID: 2, CallID: &callID, ToolCode: "lookup", Status: "success", ArgumentsJSON: `{}`, ResultJSON: nil,
	}}); complete {
		t.Fatal("missing tool result was accepted")
	}
	result := `{"ok":true}`
	groups, complete := turnToolGroups([]conversationToolRow{{
		ID: 1, RunID: 2, CallID: &callID, ToolCode: "lookup", Status: "success", ArgumentsJSON: `{}`, ResultJSON: &result,
	}})
	if !complete || len(groups) != 1 || groups[0].CallID != callID {
		t.Fatalf("complete group=%+v complete=%v", groups, complete)
	}
}
