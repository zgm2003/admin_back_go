package dict

import (
	"testing"

	"admin_back_go/internal/enum"
)

func TestAIOptionsUseEnumOrder(t *testing.T) {
	drivers := AIDriverOptions()
	if len(drivers) != len(enum.AIDrivers) || drivers[0].Value != enum.AIDriverOpenAI || drivers[0].Label != "OpenAI" {
		t.Fatalf("unexpected driver options: %#v", drivers)
	}
	executors := AIExecutorTypeOptions()
	if len(executors) != 3 || executors[0].Value != enum.AIExecutorInternal || executors[0].Label != "内置函数" {
		t.Fatalf("unexpected executor options: %#v", executors)
	}
}
