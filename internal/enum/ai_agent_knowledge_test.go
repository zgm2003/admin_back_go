package enum

import "testing"

func TestAIAgentKnowledgeEnumsAreStable(t *testing.T) {
	if !IsAIMode(AIModeChat) || !IsAIMode(AIModeRAG) || !IsAIMode(AIModeTool) || !IsAIMode(AIModeWorkflow) || IsAIMode("goods_script") {
		t.Fatalf("AI mode enum mismatch")
	}
	if !IsAICapability(AICapabilityTools) || !IsAICapability(AICapabilityRAG) || !IsAICapability(AICapabilityWorkflow) || IsAICapability(AICapabilityChat) {
		t.Fatalf("AI capability enum mismatch")
	}
	for _, scene := range RetiredAIScenes {
		if !IsRetiredAIScene(scene) {
			t.Fatalf("retired scene %q was not recognized", scene)
		}
	}
	if IsRetiredAIScene("normal_chat") {
		t.Fatalf("normal chat must not be treated as retired scene")
	}
	if !IsAIKnowledgeVisibility(AIKnowledgeVisibilityPrivate) || !IsAIKnowledgeVisibility(AIKnowledgeVisibilityTeam) || !IsAIKnowledgeVisibility(AIKnowledgeVisibilityPublic) || IsAIKnowledgeVisibility("org") {
		t.Fatalf("knowledge visibility enum mismatch")
	}
	if !IsAIKnowledgeSourceType(AIKnowledgeSourceManual) || !IsAIKnowledgeSourceType(AIKnowledgeSourceText) || IsAIKnowledgeSourceType("file") || IsAIKnowledgeSourceType("url") {
		t.Fatalf("knowledge source type enum mismatch")
	}
	if !IsAIKnowledgeIndexStatus(AIKnowledgeIndexIndexed) || !IsAIKnowledgeIndexStatus(AIKnowledgeIndexFailed) || IsAIKnowledgeIndexStatus(9) {
		t.Fatalf("knowledge index status enum mismatch")
	}
}
