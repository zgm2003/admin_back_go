package enum

import "testing"

func TestAIDriverAndExecutorEnums(t *testing.T) {
	if !IsAIDriver(AIDriverOpenAI) || !IsAIDriver(AIDriverQwen) || IsAIDriver("goods") {
		t.Fatalf("unexpected AI driver validation")
	}
	if !IsAIExecutorType(AIExecutorInternal) || !IsAIExecutorType(AIExecutorHTTPWhitelist) || !IsAIExecutorType(AIExecutorSQLReadonly) || IsAIExecutorType(9) {
		t.Fatalf("unexpected AI executor validation")
	}
}
