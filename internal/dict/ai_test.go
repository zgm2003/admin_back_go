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
}
