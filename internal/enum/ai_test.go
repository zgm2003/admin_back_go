package enum

import "testing"

func TestAIDriverEnums(t *testing.T) {
	if !IsAIDriver(AIDriverOpenAI) || !IsAIDriver(AIDriverQwen) || IsAIDriver("goods") {
		t.Fatalf("unexpected AI driver validation")
	}
}
