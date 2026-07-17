package admincontract

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	aichat "admin_back_go/internal/module/ai/chat"
	notificationtask "admin_back_go/internal/module/notification/task"
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

	wantEvents := []string{
		aichat.EventAIResponseCompleted,
		aichat.EventAIResponseDelta,
		aichat.EventAIResponseFailed,
		aichat.EventAIResponseStart,
		notificationtask.EventNotificationCreatedV1,
		modulerealtime.TypeConnectedV1,
		modulerealtime.TypeErrorV1,
		modulerealtime.TypePingV1,
		modulerealtime.TypePongV1,
		modulerealtime.TypeSubscribeV1,
		modulerealtime.TypeSubscribedV1,
	}
	sort.Strings(wantEvents)
	if !reflect.DeepEqual(envelope.Properties.Type.Enum, wantEvents) {
		t.Fatalf("event enum=%v want=%v", envelope.Properties.Type.Enum, wantEvents)
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
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("payload events=%v want=%v", gotEvents, wantEvents)
	}
}
