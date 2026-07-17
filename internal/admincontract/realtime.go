package admincontract

import (
	"sort"

	aichat "admin_back_go/internal/module/ai/chat"
	notificationtask "admin_back_go/internal/module/notification/task"
	modulerealtime "admin_back_go/internal/module/realtime"
)

type realtimeEventSchema struct {
	Type      string
	Direction string
	Payload   map[string]any
}

func buildRealtimeSchemas() (map[string]any, map[string]any) {
	events := realtimeEventSchemas()
	eventNames := make([]string, 0, len(events))
	variants := make([]any, 0, len(events))
	for _, event := range events {
		eventNames = append(eventNames, event.Type)
		variants = append(variants, map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"type", "data"},
			"properties": map[string]any{
				"type":       map[string]any{"const": event.Type},
				"request_id": map[string]any{"type": "string", "maxLength": 128},
				"data":       event.Payload,
			},
			"x-direction": event.Direction,
		})
	}

	envelope := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://contracts.admin.local/v1/realtime/envelope.schema.json",
		"title":                "Admin realtime envelope",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"type", "data"},
		"properties": map[string]any{
			"type":       map[string]any{"type": "string", "enum": eventNames},
			"request_id": map[string]any{"type": "string", "maxLength": 128},
			"data":       map[string]any{"type": "object"},
		},
	}
	eventDocument := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://contracts.admin.local/v1/realtime/events.schema.json",
		"title":   "Admin realtime events",
		"oneOf":   variants,
	}
	return envelope, eventDocument
}

func realtimeEventSchemas() []realtimeEventSchema {
	stringProperty := func(maxLength int) map[string]any {
		return map[string]any{"type": "string", "maxLength": maxLength}
	}
	positiveID := func() map[string]any {
		return map[string]any{"type": "integer", "format": "int64", "minimum": 1}
	}
	topics := map[string]any{
		"type":        "array",
		"minItems":    1,
		"uniqueItems": true,
		"items":       map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
	}
	events := []realtimeEventSchema{
		{
			Type:      aichat.EventAIResponseStart,
			Direction: "server",
			Payload: closedObject(
				[]string{"conversation_id", "request_id", "user_message_id", "agent_id"},
				map[string]any{
					"conversation_id": positiveID(),
					"request_id":      stringProperty(128),
					"user_message_id": positiveID(),
					"agent_id":        positiveID(),
				},
			),
		},
		{
			Type:      aichat.EventAIResponseDelta,
			Direction: "server",
			Payload: closedObject(
				[]string{"conversation_id", "request_id", "delta"},
				map[string]any{
					"conversation_id": positiveID(),
					"request_id":      stringProperty(128),
					"delta":           stringProperty(65536),
				},
			),
		},
		{
			Type:      aichat.EventAIResponseCompleted,
			Direction: "server",
			Payload: closedObject(
				[]string{"conversation_id", "request_id", "assistant_message_id"},
				map[string]any{
					"conversation_id":      positiveID(),
					"request_id":           stringProperty(128),
					"assistant_message_id": positiveID(),
				},
			),
		},
		{
			Type:      aichat.EventAIResponseFailed,
			Direction: "server",
			Payload: closedObject(
				[]string{"conversation_id", "request_id", "msg"},
				map[string]any{
					"conversation_id": positiveID(),
					"request_id":      stringProperty(128),
					"msg":             stringProperty(1024),
				},
			),
		},
		{
			Type:      notificationtask.EventNotificationCreatedV1,
			Direction: "server",
			Payload: closedObject(
				[]string{"task_id", "title", "content", "link", "level", "notification_type"},
				map[string]any{
					"task_id":           positiveID(),
					"title":             stringProperty(255),
					"content":           stringProperty(65536),
					"link":              stringProperty(2048),
					"level":             map[string]any{"type": "string", "enum": []string{"normal", "urgent"}},
					"notification_type": map[string]any{"type": "string", "enum": []string{"error", "info", "success", "warning"}},
				},
			),
		},
		{
			Type:      modulerealtime.TypeConnectedV1,
			Direction: "server",
			Payload: closedObject(
				[]string{"user_id", "platform", "heartbeat_interval_ms"},
				map[string]any{
					"user_id":               positiveID(),
					"platform":              map[string]any{"const": "admin"},
					"heartbeat_interval_ms": map[string]any{"type": "integer", "minimum": 1},
				},
			),
		},
		{
			Type:      modulerealtime.TypePingV1,
			Direction: "bidirectional",
			Payload:   closedObject(nil, map[string]any{}),
		},
		{
			Type:      modulerealtime.TypePongV1,
			Direction: "server",
			Payload: closedObject(
				[]string{"server_time"},
				map[string]any{"server_time": map[string]any{"type": "string", "format": "date-time"}},
			),
		},
		{
			Type:      modulerealtime.TypeSubscribeV1,
			Direction: "client",
			Payload: closedObject(
				[]string{"topics"},
				map[string]any{"topics": topics},
			),
		},
		{
			Type:      modulerealtime.TypeSubscribedV1,
			Direction: "server",
			Payload: closedObject(
				[]string{"topics"},
				map[string]any{"topics": topics},
			),
		},
		{
			Type:      modulerealtime.TypeErrorV1,
			Direction: "server",
			Payload: closedObject(
				[]string{"code", "msg"},
				map[string]any{
					"code": map[string]any{"type": "integer"},
					"msg":  stringProperty(1024),
				},
			),
		},
	}
	sort.Slice(events, func(left int, right int) bool {
		return events[left].Type < events[right].Type
	})
	return events
}

func closedObject(required []string, properties map[string]any) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           properties,
	}
}
