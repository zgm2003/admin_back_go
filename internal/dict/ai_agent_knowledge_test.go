package dict

import (
	"testing"

	"admin_back_go/internal/enum"
)

func TestAIAgentKnowledgeOptionsUseStableEnums(t *testing.T) {
	modes := AIModeOptions()
	if len(modes) != 4 || modes[0].Value != enum.AIModeChat || modes[0].Label != "对话" {
		t.Fatalf("unexpected mode options: %#v", modes)
	}
	capabilities := AICapabilityOptions()
	if len(capabilities) != 3 || capabilities[0].Value != enum.AICapabilityTools || capabilities[0].Label != "工具调用" {
		t.Fatalf("unexpected capability options: %#v", capabilities)
	}
	for _, item := range capabilities {
		if item.Value == enum.AICapabilityChat {
			t.Fatalf("chat is implicit and must not be a capability switch: %#v", capabilities)
		}
	}
	visibility := AIKnowledgeVisibilityOptions()
	if len(visibility) != 3 || visibility[0].Value != enum.AIKnowledgeVisibilityPrivate {
		t.Fatalf("unexpected visibility options: %#v", visibility)
	}
	sources := AIKnowledgeSourceTypeOptions()
	if len(sources) != 2 || sources[0].Value != enum.AIKnowledgeSourceManual || sources[1].Value != enum.AIKnowledgeSourceText {
		t.Fatalf("unexpected source options: %#v", sources)
	}
	indexStatuses := AIKnowledgeIndexStatusOptions()
	if len(indexStatuses) != 2 || indexStatuses[0].Value != enum.AIKnowledgeIndexIndexed || indexStatuses[1].Value != enum.AIKnowledgeIndexFailed {
		t.Fatalf("unexpected index options: %#v", indexStatuses)
	}
}
