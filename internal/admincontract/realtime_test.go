package admincontract

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	infrarealtime "admin_back_go/internal/infra/realtime"
	modulerealtime "admin_back_go/internal/module/realtime"
)

func TestRealtimeSchemasCloseEventNamesAndPayloads(t *testing.T) {
	bundle := mustBuildBundle(t)
	var envelope struct {
		Properties struct {
			Type struct {
				Enum []string `json:"enum"`
			} `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(bundle.Artifacts["realtime/envelope.schema.json"], &envelope); err != nil {
		t.Fatalf("decode envelope schema: %v", err)
	}

	definitions := modulerealtime.DefaultRegistry().Definitions()
	wantEvents := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		wantEvents = append(wantEvents, definition.Type)
	}
	sort.Strings(wantEvents)
	if !reflect.DeepEqual(envelope.Properties.Type.Enum, wantEvents) {
		t.Fatalf("event enum=%v want registry=%v", envelope.Properties.Type.Enum, wantEvents)
	}

	var events struct {
		OneOf []struct {
			Properties struct {
				Type struct {
					Const string `json:"const"`
				} `json:"type"`
				Data map[string]any `json:"data"`
			} `json:"properties"`
		} `json:"oneOf"`
	}
	if err := json.Unmarshal(bundle.Artifacts["realtime/events.schema.json"], &events); err != nil {
		t.Fatalf("decode event schema: %v", err)
	}
	gotEvents := make([]string, 0, len(events.OneOf))
	for _, event := range events.OneOf {
		if event.Properties.Type.Const == "" {
			t.Fatal("event schema has a free-form type")
		}
		if event.Properties.Data["additionalProperties"] != false {
			t.Fatalf("event %s payload is not closed", event.Properties.Type.Const)
		}
		gotEvents = append(gotEvents, event.Properties.Type.Const)
	}
	sort.Strings(gotEvents)
	gotEvents = uniqueStrings(gotEvents)
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("payload events=%v want registry=%v", gotEvents, wantEvents)
	}

	for _, definition := range definitions {
		assertRealtimeEventEnvelopeShape(t, bundle.Artifacts["realtime/events.schema.json"], definition)
		assertRealtimePayloadFieldsMatchCodec(t, bundle.Artifacts["realtime/events.schema.json"], definition)
	}
}

func TestRealtimeSchemasPreserveNonBlankPayloadConstraints(t *testing.T) {
	bundle := mustBuildBundle(t)
	for _, check := range []struct {
		eventType string
		field     string
	}{
		{modulerealtime.TypeAIResponseStartV1, "request_id"},
		{modulerealtime.TypeAIResponseCanceledV1, "request_id"},
		{modulerealtime.TypeAIResponseFailedV1, "msg"},
		{modulerealtime.TypeErrorV1, "msg"},
	} {
		property := realtimePayloadProperty(t, bundle.Artifacts["realtime/events.schema.json"], check.eventType, check.field)
		if property["minLength"] != float64(1) || property["pattern"] == "" {
			t.Fatalf("event %s field %s does not preserve non-blank validation: %v", check.eventType, check.field, property)
		}
	}
	topics := realtimePayloadProperty(t, bundle.Artifacts["realtime/events.schema.json"], modulerealtime.TypeSubscribeV1, "topics")
	items, _ := topics["items"].(map[string]any)
	if items["minLength"] != float64(1) || items["pattern"] == "" {
		t.Fatalf("subscribe topic does not preserve non-blank validation: %v", items)
	}
}

func TestAIResponseFailedContractRequiresMachineCodeAndExplicitConditionalPaths(t *testing.T) {
	bundle := mustBuildBundle(t)
	artifact := bundle.Artifacts["realtime/events.schema.json"]
	for _, field := range []string{"conversation_id", "request_id", "msg", "error_code", "wallet_path", "recharge_path"} {
		_ = realtimePayloadProperty(t, artifact, modulerealtime.TypeAIResponseFailedV1, field)
	}
	errorCode := realtimePayloadProperty(t, artifact, modulerealtime.TypeAIResponseFailedV1, "error_code")
	if errorCode["minLength"] != float64(1) || errorCode["pattern"] == "" {
		t.Fatalf("error_code must reject blank strings: %#v", errorCode)
	}

	payload := realtimePayloadSchema(t, artifact, modulerealtime.TypeAIResponseFailedV1)
	allOf, ok := payload["allOf"].([]any)
	if !ok || len(allOf) != 1 {
		t.Fatalf("failed payload conditional schema=%#v", payload["allOf"])
	}
	conditional := allOf[0].(map[string]any)
	thenProperties := conditional["then"].(map[string]any)["properties"].(map[string]any)
	if thenProperties["wallet_path"].(map[string]any)["const"] != "/profile/wallet" || thenProperties["recharge_path"].(map[string]any)["const"] != "/payment/recharge" {
		t.Fatalf("billing paths are not canonical: %#v", thenProperties)
	}
	elseProperties := conditional["else"].(map[string]any)["properties"].(map[string]any)
	if elseProperties["wallet_path"].(map[string]any)["type"] != "null" || elseProperties["recharge_path"].(map[string]any)["type"] != "null" {
		t.Fatalf("non-billing paths must be explicit null: %#v", elseProperties)
	}
}

func realtimePayloadSchema(t *testing.T, artifact []byte, eventType string) map[string]any {
	t.Helper()
	var document struct {
		OneOf []map[string]any `json:"oneOf"`
	}
	if err := json.Unmarshal(artifact, &document); err != nil {
		t.Fatalf("decode realtime event document: %v", err)
	}
	for _, variant := range document.OneOf {
		properties, _ := variant["properties"].(map[string]any)
		typeSchema, _ := properties["type"].(map[string]any)
		if typeSchema["const"] == eventType {
			return properties["data"].(map[string]any)
		}
	}
	t.Fatalf("event %s has no contract variant", eventType)
	return nil
}

func realtimePayloadProperty(t *testing.T, artifact []byte, eventType string, field string) map[string]any {
	t.Helper()
	var document struct {
		OneOf []map[string]any `json:"oneOf"`
	}
	if err := json.Unmarshal(artifact, &document); err != nil {
		t.Fatalf("decode realtime event document: %v", err)
	}
	for _, variant := range document.OneOf {
		properties, _ := variant["properties"].(map[string]any)
		typeSchema, _ := properties["type"].(map[string]any)
		if typeSchema["const"] != eventType {
			continue
		}
		data, _ := properties["data"].(map[string]any)
		payloadProperties, _ := data["properties"].(map[string]any)
		property, _ := payloadProperties[field].(map[string]any)
		if property == nil {
			t.Fatalf("event %s has no field %s", eventType, field)
		}
		return property
	}
	t.Fatalf("event %s has no contract variant", eventType)
	return nil
}

func assertRealtimePayloadFieldsMatchCodec(t *testing.T, artifact []byte, definition modulerealtime.EventDefinition) {
	t.Helper()
	var document struct {
		OneOf []map[string]any `json:"oneOf"`
	}
	if err := json.Unmarshal(artifact, &document); err != nil {
		t.Fatalf("decode realtime event document: %v", err)
	}
	want := payloadJSONFields(t, definition)
	for _, variant := range document.OneOf {
		properties, _ := variant["properties"].(map[string]any)
		typeSchema, _ := properties["type"].(map[string]any)
		if typeSchema["const"] != definition.Type {
			continue
		}
		data, _ := properties["data"].(map[string]any)
		payloadProperties, _ := data["properties"].(map[string]any)
		got := make([]string, 0, len(payloadProperties))
		for field := range payloadProperties {
			got = append(got, field)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("event %s contract fields=%v codec fields=%v", definition.Type, got, want)
		}
		required := stringSet(data["required"])
		if len(required) != len(want) {
			t.Fatalf("event %s required fields=%v codec fields=%v", definition.Type, required, want)
		}
		for _, field := range want {
			if _, ok := required[field]; !ok {
				t.Fatalf("event %s codec field %s is not required by contract", definition.Type, field)
			}
		}
		return
	}
	t.Fatalf("event %s has no payload contract variant", definition.Type)
}

func payloadJSONFields(t *testing.T, definition modulerealtime.EventDefinition) []string {
	t.Helper()
	if definition.NewPayload == nil {
		t.Fatalf("event %s has no payload codec", definition.Type)
	}
	payloadType := reflect.TypeOf(definition.NewPayload())
	if payloadType.Kind() == reflect.Pointer {
		payloadType = payloadType.Elem()
	}
	if payloadType.Kind() != reflect.Struct {
		t.Fatalf("event %s payload codec type=%s", definition.Type, payloadType)
	}
	fields := make([]string, 0, payloadType.NumField())
	for index := 0; index < payloadType.NumField(); index++ {
		name := strings.Split(payloadType.Field(index).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			fields = append(fields, name)
		}
	}
	sort.Strings(fields)
	return fields
}

func assertRealtimeEventEnvelopeShape(t *testing.T, artifact []byte, definition modulerealtime.EventDefinition) {
	t.Helper()
	var document struct {
		OneOf []map[string]any `json:"oneOf"`
	}
	if err := json.Unmarshal(artifact, &document); err != nil {
		t.Fatalf("decode realtime event document: %v", err)
	}
	matchedClient := false
	matchedServer := false
	for _, variant := range document.OneOf {
		properties, _ := variant["properties"].(map[string]any)
		typeSchema, _ := properties["type"].(map[string]any)
		if typeSchema["const"] != definition.Type {
			continue
		}
		required := stringSet(variant["required"])
		_, hasEventID := properties["event_id"]
		if hasEventID {
			matchedServer = true
			for _, field := range []string{"event_id", "type", "sequence", "occurred_at", "durability", "data"} {
				if _, ok := required[field]; !ok {
					t.Fatalf("server event %s does not require %s", definition.Type, field)
				}
			}
			durability, _ := properties["durability"].(map[string]any)
			if durability["const"] != string(definition.Durability) {
				t.Fatalf("server event %s durability=%v want=%s", definition.Type, durability["const"], definition.Durability)
			}
			sequence, _ := properties["sequence"].(map[string]any)
			if definition.Durability == infrarealtime.Ephemeral && sequence["const"] != float64(0) {
				t.Fatalf("ephemeral event %s sequence schema=%v", definition.Type, sequence)
			}
			if definition.Durability == infrarealtime.Durable && sequence["minimum"] != float64(1) {
				t.Fatalf("durable event %s sequence schema=%v", definition.Type, sequence)
			}
		} else {
			matchedClient = true
			for _, forbidden := range []string{"event_id", "sequence", "occurred_at", "durability"} {
				if _, ok := properties[forbidden]; ok {
					t.Fatalf("client event %s exposes server-owned %s", definition.Type, forbidden)
				}
			}
		}
	}
	wantClient := definition.Direction == modulerealtime.DirectionClient || definition.Direction == modulerealtime.DirectionBidirectional
	wantServer := definition.Direction == modulerealtime.DirectionServer || definition.Direction == modulerealtime.DirectionBidirectional
	if matchedClient != wantClient || matchedServer != wantServer {
		t.Fatalf("event %s variants client=%v server=%v want client=%v server=%v", definition.Type, matchedClient, matchedServer, wantClient, wantServer)
	}
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func stringSet(raw any) map[string]struct{} {
	result := map[string]struct{}{}
	for _, value := range raw.([]any) {
		result[value.(string)] = struct{}{}
	}
	return result
}
