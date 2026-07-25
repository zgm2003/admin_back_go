package admin

import (
	"reflect"
	"strings"
	"testing"
)

func TestGraphValidateRejectsMissingRequiredCapability(t *testing.T) {
	graph := Graph{}
	err := graph.Validate()
	if err == nil || !strings.Contains(err.Error(), "identity.auth") {
		t.Fatalf("err=%v", err)
	}
}

func TestGraphValidateRequiresRedeemCodeCapability(t *testing.T) {
	field, ok := reflect.TypeOf(CommerceGraph{}).FieldByName("RedeemCodes")
	if !ok || field.Type.Kind() != reflect.Interface {
		t.Fatalf("CommerceGraph.RedeemCodes capability missing or wrong type: %+v", field)
	}
}
