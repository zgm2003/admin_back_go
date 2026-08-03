package contextengine

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEvaluationRequestIsClosedAndValidated(t *testing.T) {
	typeOf := reflect.TypeOf(EvaluationRequest{})
	if typeOf.NumField() != 2 {
		t.Fatalf("EvaluationRequest fields=%d, want 2", typeOf.NumField())
	}
	if field, ok := typeOf.FieldByName("AgentID"); !ok || field.Tag.Get("json") != "agent_id" || field.Tag.Get("binding") != "required,gt=0" {
		t.Fatalf("AgentID contract=%+v", field)
	}
	if field, ok := typeOf.FieldByName("Query"); !ok || field.Tag.Get("json") != "query" || field.Tag.Get("binding") != "required,min=1,max=20000" {
		t.Fatalf("Query contract=%+v", field)
	}
	raw, err := json.Marshal(EvaluationRequest{AgentID: 7, Query: "find it"})
	if err != nil || string(raw) != `{"agent_id":7,"query":"find it"}` {
		t.Fatalf("request JSON=%s err=%v", raw, err)
	}
}

func TestContextStateEnumsRejectUnknownValues(t *testing.T) {
	if err := ValidateContextAdminState("profile", "retired"); err != nil {
		t.Fatalf("profile retired: %v", err)
	}
	if err := ValidateContextAdminState("space", "disabled"); err != nil {
		t.Fatalf("space disabled: %v", err)
	}
	if err := ValidateContextAdminState("document_version", "ready"); err != nil {
		t.Fatalf("document ready: %v", err)
	}
	if err := ValidateContextAdminState("space", "paused"); err == nil {
		t.Fatal("unknown state accepted")
	}
}
