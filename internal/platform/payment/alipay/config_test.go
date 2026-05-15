package alipay

import (
	"context"
	"testing"
)

func TestGopayGatewayTestConfigRejectsMissingAppIDWithoutRemoteCall(t *testing.T) {
	gateway := NewGopayGateway(0)
	err := gateway.TestConfig(context.Background(), ChannelConfig{})
	if err == nil {
		t.Fatal("expected missing config error")
	}
}
