package admincontract

import (
	"fmt"
	"sort"

	infrarealtime "admin_back_go/internal/infra/realtime"
	modulerealtime "admin_back_go/internal/module/realtime"
)

const realtimeRequestIDMaxLength = 128

type realtimeEventSchema struct {
	Type    string
	Payload map[string]any
}

func buildRealtimeSchemas() (map[string]any, map[string]any, error) {
	payloads := make(map[string]map[string]any)
	for _, event := range realtimeEventSchemas() {
		if event.Type == "" || event.Payload == nil {
			return nil, nil, fmt.Errorf("realtime payload schema has an empty event type or payload")
		}
		if _, duplicate := payloads[event.Type]; duplicate {
			return nil, nil, fmt.Errorf("duplicate realtime payload schema %q", event.Type)
		}
		payloads[event.Type] = event.Payload
	}

	definitions := modulerealtime.DefaultRegistry().Definitions()
	eventNames := make([]string, 0, len(definitions))
	clientTypes := make([]string, 0)
	serverEphemeralTypes := make([]string, 0)
	serverDurableTypes := make([]string, 0)
	variants := make([]any, 0, len(definitions)+1)
	for _, definition := range definitions {
		payload, ok := payloads[definition.Type]
		if !ok {
			return nil, nil, fmt.Errorf("registered realtime event %q has no contract payload schema", definition.Type)
		}
		delete(payloads, definition.Type)
		eventNames = append(eventNames, definition.Type)

		switch definition.Direction {
		case modulerealtime.DirectionClient:
			clientTypes = append(clientTypes, definition.Type)
			variants = append(variants, clientEventVariant(definition, payload))
		case modulerealtime.DirectionServer:
			if err := appendServerType(definition, &serverEphemeralTypes, &serverDurableTypes); err != nil {
				return nil, nil, err
			}
			variants = append(variants, serverEventVariant(definition, payload))
		case modulerealtime.DirectionBidirectional:
			clientTypes = append(clientTypes, definition.Type)
			if err := appendServerType(definition, &serverEphemeralTypes, &serverDurableTypes); err != nil {
				return nil, nil, err
			}
			variants = append(variants, clientEventVariant(definition, payload), serverEventVariant(definition, payload))
		default:
			return nil, nil, fmt.Errorf("registered realtime event %q has invalid direction %q", definition.Type, definition.Direction)
		}
	}
	if len(payloads) > 0 {
		extra := make([]string, 0, len(payloads))
		for eventType := range payloads {
			extra = append(extra, eventType)
		}
		sort.Strings(extra)
		return nil, nil, fmt.Errorf("realtime contract has unregistered payload schemas: %v", extra)
	}
	sort.Strings(eventNames)
	sort.Strings(clientTypes)
	sort.Strings(serverEphemeralTypes)
	sort.Strings(serverDurableTypes)

	roleVariants := make([]any, 0, 3)
	if len(clientTypes) > 0 {
		roleVariants = append(roleVariants, clientEnvelopeVariant(enumTypeProperty(clientTypes), map[string]any{"type": "object"}, "client"))
	}
	if len(serverEphemeralTypes) > 0 {
		roleVariants = append(roleVariants, serverEnvelopeVariant(enumTypeProperty(serverEphemeralTypes), map[string]any{"type": "object"}, infrarealtime.Ephemeral, "server"))
	}
	if len(serverDurableTypes) > 0 {
		roleVariants = append(roleVariants, serverEnvelopeVariant(enumTypeProperty(serverDurableTypes), map[string]any{"type": "object"}, infrarealtime.Durable, "server"))
	}

	envelope := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://contracts.admin.local/v1/realtime/envelope.schema.json",
		"title":                "Admin realtime envelope",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"type", "data"},
		"properties": map[string]any{
			"event_id":    eventIDProperty(),
			"type":        enumTypeProperty(eventNames),
			"request_id":  stringProperty(realtimeRequestIDMaxLength),
			"sequence":    map[string]any{"type": "integer", "minimum": 0},
			"occurred_at": map[string]any{"type": "string", "format": "date-time"},
			"durability":  map[string]any{"type": "string", "enum": []string{string(infrarealtime.Durable), string(infrarealtime.Ephemeral)}},
			"data":        map[string]any{"type": "object"},
		},
		"allOf": []any{map[string]any{"oneOf": roleVariants}},
	}
	eventDocument := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://contracts.admin.local/v1/realtime/events.schema.json",
		"title":   "Admin realtime events",
		"oneOf":   variants,
	}
	return envelope, eventDocument, nil
}

func appendServerType(definition modulerealtime.EventDefinition, ephemeral *[]string, durable *[]string) error {
	switch definition.Durability {
	case infrarealtime.Ephemeral:
		*ephemeral = append(*ephemeral, definition.Type)
	case infrarealtime.Durable:
		*durable = append(*durable, definition.Type)
	default:
		return fmt.Errorf("registered realtime event %q has invalid durability %q", definition.Type, definition.Durability)
	}
	return nil
}

func clientEventVariant(definition modulerealtime.EventDefinition, payload map[string]any) map[string]any {
	variant := clientEnvelopeVariant(map[string]any{"const": definition.Type}, payload, "client")
	variant["x-direction"] = string(definition.Direction)
	return variant
}

func serverEventVariant(definition modulerealtime.EventDefinition, payload map[string]any) map[string]any {
	variant := serverEnvelopeVariant(map[string]any{"const": definition.Type}, payload, definition.Durability, "server")
	variant["x-direction"] = string(definition.Direction)
	return variant
}

func clientEnvelopeVariant(typeProperty map[string]any, payload map[string]any, role string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"type", "data"},
		"properties": map[string]any{
			"type":       typeProperty,
			"request_id": stringProperty(realtimeRequestIDMaxLength),
			"data":       payload,
		},
		"x-envelope-role": role,
	}
}

func serverEnvelopeVariant(typeProperty map[string]any, payload map[string]any, durability infrarealtime.Durability, role string) map[string]any {
	sequence := map[string]any{"type": "integer"}
	if durability == infrarealtime.Ephemeral {
		sequence["const"] = 0
	} else {
		sequence["minimum"] = 1
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"event_id", "type", "sequence", "occurred_at", "durability", "data"},
		"properties": map[string]any{
			"event_id":    eventIDProperty(),
			"type":        typeProperty,
			"request_id":  stringProperty(realtimeRequestIDMaxLength),
			"sequence":    sequence,
			"occurred_at": map[string]any{"type": "string", "format": "date-time"},
			"durability":  map[string]any{"const": string(durability)},
			"data":        payload,
		},
		"x-envelope-role": role,
		"x-durability":    string(durability),
	}
}

func enumTypeProperty(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func eventIDProperty() map[string]any {
	return map[string]any{
		"type":      "string",
		"minLength": 26,
		"maxLength": 26,
		"pattern":   "^[0-7][0-9A-HJKMNP-TV-Z]{25}$",
	}
}

func stringProperty(maxLength int) map[string]any {
	return map[string]any{"type": "string", "maxLength": maxLength}
}

func nonBlankStringProperty(maxLength int) map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": maxLength, "pattern": ".*\\S.*"}
}

func realtimeEventSchemas() []realtimeEventSchema {
	positiveID := func() map[string]any {
		return map[string]any{"type": "integer", "format": "int64", "minimum": 1}
	}
	topics := map[string]any{
		"type":        "array",
		"minItems":    1,
		"uniqueItems": true,
		"items":       nonBlankStringProperty(128),
	}
	return []realtimeEventSchema{
		{
			Type: modulerealtime.TypeAIResponseStartV1,
			Payload: closedObject(
				[]string{"conversation_id", "request_id", "user_message_id", "agent_id"},
				map[string]any{
					"conversation_id": positiveID(),
					"request_id":      nonBlankStringProperty(realtimeRequestIDMaxLength),
					"user_message_id": positiveID(),
					"agent_id":        positiveID(),
				},
			),
		},
		{
			Type: modulerealtime.TypeAIResponseDeltaV2,
			Payload: closedObject(
				[]string{"conversation_id", "request_id", "delivery_seq", "delta"},
				map[string]any{
					"conversation_id": positiveID(),
					"request_id":      nonBlankStringProperty(realtimeRequestIDMaxLength),
					"delivery_seq":    positiveID(),
					"delta":           map[string]any{"type": "string", "minLength": 1, "maxLength": 16384},
				},
			),
		},
		{
			Type: modulerealtime.TypeAIResponseCompletedV1,
			Payload: closedObject(
				[]string{"conversation_id", "request_id", "assistant_message_id"},
				map[string]any{
					"conversation_id":      positiveID(),
					"request_id":           nonBlankStringProperty(realtimeRequestIDMaxLength),
					"assistant_message_id": positiveID(),
				},
			),
		},
		{
			Type: modulerealtime.TypeAIResponseFailedV1,
			Payload: schemaWith(closedObject(
				[]string{"conversation_id", "request_id", "msg", "error_code", "wallet_path", "recharge_path"},
				map[string]any{
					"conversation_id": positiveID(),
					"request_id":      nonBlankStringProperty(realtimeRequestIDMaxLength),
					"msg":             nonBlankStringProperty(1024),
					"error_code":      nonBlankStringProperty(128),
					"wallet_path":     nullableSchema(nonBlankStringProperty(2048)),
					"recharge_path":   nullableSchema(nonBlankStringProperty(2048)),
				},
			), "allOf", []any{map[string]any{
				"if": map[string]any{
					"properties": map[string]any{"error_code": map[string]any{"const": "ai.billing.insufficient_balance"}},
					"required":   []string{"error_code"},
				},
				"then": map[string]any{"properties": map[string]any{
					"wallet_path":   map[string]any{"type": "string", "const": "/profile/wallet"},
					"recharge_path": map[string]any{"type": "string", "const": "/payment/recharge"},
				}},
				"else": map[string]any{"properties": map[string]any{
					"wallet_path":   map[string]any{"type": "null"},
					"recharge_path": map[string]any{"type": "null"},
				}},
			}}),
		},
		{
			Type: modulerealtime.TypeAIResponseCanceledV2,
			Payload: closedObject(
				[]string{"conversation_id", "request_id", "assistant_message_id"},
				map[string]any{
					"conversation_id":      positiveID(),
					"request_id":           nonBlankStringProperty(realtimeRequestIDMaxLength),
					"assistant_message_id": positiveID(),
				},
			),
		},
		{
			Type: modulerealtime.TypeNotificationCreatedV1,
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
			Type: modulerealtime.TypeConnectedV1,
			Payload: closedObject(
				[]string{"user_id", "platform", "heartbeat_interval_ms"},
				map[string]any{
					"user_id":               positiveID(),
					"platform":              map[string]any{"const": "admin"},
					"heartbeat_interval_ms": map[string]any{"type": "integer", "minimum": 1},
				},
			),
		},
		{Type: modulerealtime.TypePingV1, Payload: closedObject(nil, map[string]any{})},
		{
			Type: modulerealtime.TypePongV1,
			Payload: closedObject(
				[]string{"server_time"},
				map[string]any{"server_time": map[string]any{"type": "string", "format": "date-time"}},
			),
		},
		{
			Type: modulerealtime.TypeSubscribeV1,
			Payload: closedObject(
				[]string{"topics"},
				map[string]any{"topics": topics},
			),
		},
		{
			Type: modulerealtime.TypeSubscribedV1,
			Payload: closedObject(
				[]string{"topics"},
				map[string]any{"topics": topics},
			),
		},
		{
			Type: modulerealtime.TypeResumeV1,
			Payload: closedObject(
				[]string{"after_sequence"},
				map[string]any{"after_sequence": map[string]any{"type": "integer", "minimum": 0}},
			),
		},
		{
			Type: modulerealtime.TypeResyncRequiredV1,
			Payload: closedObject(
				[]string{"latest_sequence"},
				map[string]any{"latest_sequence": map[string]any{"type": "integer", "minimum": 0}},
			),
		},
		{
			Type: modulerealtime.TypeErrorV1,
			Payload: closedObject(
				[]string{"code", "msg"},
				map[string]any{
					"code": map[string]any{"type": "integer", "minimum": 1},
					"msg":  nonBlankStringProperty(1024),
				},
			),
		},
	}
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
