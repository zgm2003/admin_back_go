package admincontract

func aiWorkflowSchemas() map[string]any {
	return map[string]any{
		"AIConversationItem": closedObjectAllProperties(map[string]any{
			"id":              positiveIntegerSchema(),
			"agent_id":        positiveIntegerSchema(),
			"agent_name":      stringSchema(),
			"title":           stringSchema(),
			"last_message_at": stringSchema(),
			"updated_at":      stringSchema(),
		}),
		"AIConversationListResult": closedObjectAllProperties(map[string]any{
			"list":      arraySchema(schemaReference("AIConversationItem")),
			"next_time": stringSchema(),
			"next_id":   nonNegativeIntegerSchema(),
			"has_more":  booleanSchema(),
		}),
		"AIConversationListSuccessEnvelope": successEnvelopeWithData(schemaReference("AIConversationListResult")),
		"AIConversationCreateRequest": closedObjectSchema([]string{"agent_id"}, map[string]any{
			"agent_id": positiveIntegerSchema(),
			"title":    maxStringSchema(100),
		}),
		"AIConversationCreateResult": closedObjectAllProperties(map[string]any{
			"id": positiveIntegerSchema(),
		}),
		"AIConversationCreateSuccessEnvelope": successEnvelopeWithData(schemaReference("AIConversationCreateResult")),
		"AIConversationDetail": closedObjectAllProperties(map[string]any{
			"id":              positiveIntegerSchema(),
			"agent_id":        positiveIntegerSchema(),
			"agent_name":      stringSchema(),
			"title":           stringSchema(),
			"last_message_at": stringSchema(),
			"created_at":      stringSchema(),
			"updated_at":      stringSchema(),
		}),
		"AIConversationDetailSuccessEnvelope": successEnvelopeWithData(schemaReference("AIConversationDetail")),
		"AIConversationUpdateRequest": closedObjectSchema([]string{"title"}, map[string]any{
			"title": schemaWith(maxStringSchema(100), "minLength", 1),
		}),

		"AIRuntimeParams": closedObjectSchema(nil, map[string]any{
			"temperature": schemaWith(numberSchema(), "minimum", 0, "maximum", 2),
			"max_tokens":  integerRangeSchema(1, 200000),
			"max_history": integerRangeSchema(1, 50),
		}),
		"AIAttachmentRequest": closedObjectSchema([]string{"type", "url"}, map[string]any{
			"type": map[string]any{"type": "string", "const": "image"},
			"url":  nonEmptyStringSchema(),
			"name": stringSchema(),
			"size": nonNegativeIntegerSchema(),
		}),
		"AIMessageSendRequest": aiMessageSendRequestSchema(),
		"AIMessageMetaAttachment": closedObjectAllProperties(map[string]any{
			"type": map[string]any{"type": "string", "const": "image"},
			"url":  stringSchema(),
			"name": stringSchema(),
			"size": nonNegativeIntegerSchema(),
		}),
		"AIMessageMeta": closedObjectSchema(nil, map[string]any{
			"attachments":    arraySchema(schemaReference("AIMessageMetaAttachment")),
			"runtime_params": schemaReference("AIRuntimeParams"),
		}),
		"AIMessageItem": closedObjectSchema(
			[]string{"id", "role", "content_type", "content", "created_at", "updated_at"},
			map[string]any{
				"id":           positiveIntegerSchema(),
				"role":         integerEnumSchema(1, 2, 3),
				"content_type": stringSchema(),
				"content":      stringSchema(),
				"meta_json":    schemaReference("AIMessageMeta"),
				"created_at":   stringSchema(),
				"updated_at":   stringSchema(),
			},
		),
		"AIMessageListResult": closedObjectAllProperties(map[string]any{
			"list":     arraySchema(schemaReference("AIMessageItem")),
			"next_id":  nonNegativeIntegerSchema(),
			"has_more": booleanSchema(),
		}),
		"AIMessageListSuccessEnvelope": successEnvelopeWithData(schemaReference("AIMessageListResult")),
		"AIMessageSendResult": closedObjectAllProperties(map[string]any{
			"conversation_id": positiveIntegerSchema(),
			"user_message_id": positiveIntegerSchema(),
			"command_id":      positiveIntegerSchema(),
			"request_id":      stringSchema(),
			"state": stringEnumSchema(
				"pending", "claimed", "running", "succeeded", "failed", "canceled", "outcome_unknown", "timed_out",
			),
		}),
		"AIMessageSendSuccessEnvelope": successEnvelopeWithData(schemaReference("AIMessageSendResult")),
		"AIMessageCancelRequest": closedObjectSchema([]string{"request_id"}, map[string]any{
			"request_id": schemaWith(maxStringSchema(128), "minLength", 1),
		}),
		"AIMessageCancelResult": closedObjectAllProperties(map[string]any{
			"conversation_id": positiveIntegerSchema(),
			"request_id":      stringSchema(),
			"status":          map[string]any{"type": "string", "const": "stopping"},
		}),
		"AIMessageCancelSuccessEnvelope": successEnvelopeWithData(schemaReference("AIMessageCancelResult")),

		"AIRunPageInitDict": closedObjectAllProperties(map[string]any{
			"status_arr":   arraySchema(schemaReference("StringOption")),
			"platform_arr": arraySchema(schemaReference("StringOption")),
			"providerArr":  arraySchema(schemaReference("IntOption")),
			"agentArr":     arraySchema(schemaReference("IntOption")),
		}),
		"AIRunPageInit": closedObjectAllProperties(map[string]any{
			"dict": schemaReference("AIRunPageInitDict"),
		}),
		"AIRunPageInitSuccessEnvelope": successEnvelopeWithData(schemaReference("AIRunPageInit")),
		"AIRunListItem":                aiRunListItemSchema(),
		"AIRunListResult": closedObjectAllProperties(map[string]any{
			"list": arraySchema(schemaReference("AIRunListItem")),
			"page": schemaReference("Page"),
		}),
		"AIRunListSuccessEnvelope":   successEnvelopeWithData(schemaReference("AIRunListResult")),
		"AIRunMessageSummary":        aiRunMessageSummarySchema(),
		"AIRunEvent":                 aiRunEventSchema(),
		"AIRunToolCall":              aiRunToolCallSchema(),
		"AIRunKnowledgeHit":          aiRunKnowledgeHitSchema(),
		"AIRunKnowledgeRetrieval":    aiRunKnowledgeRetrievalSchema(),
		"AIRunPricingRate":           aiRunPricingRateSchema(),
		"AIRunPricing":               aiRunPricingSchema(),
		"AIRunUsageItem":             aiRunUsageItemSchema(),
		"AIRunProviderAttempt":       aiRunProviderAttemptSchema(),
		"AIRunDetail":                aiRunDetailSchema(),
		"AIRunDetailSuccessEnvelope": successEnvelopeWithData(schemaReference("AIRunDetail")),
		"AIRunStatsSummary":          aiRunStatsSummarySchema(),
		"AIRunStatsResult": closedObjectAllProperties(map[string]any{
			"date_range": schemaReference("AIRunStatsDateRange"),
			"summary":    schemaReference("AIRunStatsSummary"),
		}),
		"AIRunStatsDateRange": closedObjectAllProperties(map[string]any{
			"start": nullableSchema(stringSchema()),
			"end":   nullableSchema(stringSchema()),
		}),
		"AIRunStatsSuccessEnvelope": successEnvelopeWithData(schemaReference("AIRunStatsResult")),
		"AIRunStatsMetric":          aiRunStatsMetricSchema(),
		"AIRunStatsByDateItem":      aiRunStatsByDateItemSchema(),
		"AIRunStatsByAgentItem":     aiRunStatsByAgentItemSchema(),
		"AIRunStatsByUserItem":      aiRunStatsByUserItemSchema(),
		"AIRunStatsByDateResult": closedObjectAllProperties(map[string]any{
			"list": arraySchema(schemaReference("AIRunStatsByDateItem")),
			"page": schemaReference("Page"),
		}),
		"AIRunStatsByAgentResult": closedObjectAllProperties(map[string]any{
			"list": arraySchema(schemaReference("AIRunStatsByAgentItem")),
			"page": schemaReference("Page"),
		}),
		"AIRunStatsByUserResult": closedObjectAllProperties(map[string]any{
			"list": arraySchema(schemaReference("AIRunStatsByUserItem")),
			"page": schemaReference("Page"),
		}),
		"AIRunStatsByDateSuccessEnvelope":  successEnvelopeWithData(schemaReference("AIRunStatsByDateResult")),
		"AIRunStatsByAgentSuccessEnvelope": successEnvelopeWithData(schemaReference("AIRunStatsByAgentResult")),
		"AIRunStatsByUserSuccessEnvelope":  successEnvelopeWithData(schemaReference("AIRunStatsByUserResult")),
	}
}

func aiMessageSendRequestSchema() map[string]any {
	schema := closedObjectSchema([]string{"request_id"}, map[string]any{
		"content":        schemaWith(maxStringSchema(20000), "description", "Trimmed content must be non-empty when attachments is absent or empty."),
		"request_id":     schemaWith(maxStringSchema(128), "minLength", 1),
		"attachments":    schemaWith(arraySchema(schemaReference("AIAttachmentRequest")), "maxItems", 5),
		"runtime_params": schemaReference("AIRuntimeParams"),
	})
	schema["description"] = "request_id is required; additionally, trimmed content must be non-empty or attachments must contain at least one image. The cross-field rule is also published on the operation."
	return schema
}

func aiRunListItemProperties() map[string]any {
	return map[string]any{
		"id":                 positiveIntegerSchema(),
		"request_id":         stringSchema(),
		"user_id":            positiveIntegerSchema(),
		"agent_id":           positiveIntegerSchema(),
		"agent_name":         stringSchema(),
		"provider_id":        positiveIntegerSchema(),
		"provider_name":      stringSchema(),
		"platform":           registeredPlatformSchema(),
		"input_snapshot":     stringSchema(),
		"conversation_id":    nullableSchema(positiveIntegerSchema()),
		"conversation_title": stringSchema(),
		"status": stringEnumSchema(
			"running", "success", "failed", "canceled", "timeout",
		),
		"status_name":        stringSchema(),
		"model_id":           stringSchema(),
		"model_display_name": stringSchema(),
		"prompt_tokens":      nonNegativeIntegerSchema(),
		"completion_tokens":  nonNegativeIntegerSchema(),
		"total_tokens":       nonNegativeIntegerSchema(),
		"duration_ms":        nullableSchema(nonNegativeIntegerSchema()),
		"duration_text":      stringSchema(),
		"error_message":      stringSchema(),
		"created_at":         stringSchema(),
	}
}

func aiRunListItemSchema() map[string]any {
	return closedObjectAllProperties(aiRunListItemProperties())
}

func aiRunMessageSummarySchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"id":           positiveIntegerSchema(),
		"role":         integerEnumSchema(1, 2, 3),
		"content_type": stringSchema(),
		"content":      stringSchema(),
		"meta_json":    schemaReference("JSONValue"),
		"created_at":   stringSchema(),
	})
}

func aiRunEventSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"id":              positiveIntegerSchema(),
		"seq":             nonNegativeIntegerSchema(),
		"event_type":      stringEnumSchema("start", "completed", "failed", "canceled", "timeout"),
		"event_type_name": stringSchema(),
		"message":         stringSchema(),
		"elapsed_ms":      nullableSchema(nonNegativeIntegerSchema()),
		"elapsed_text":    stringSchema(),
		"created_at":      stringSchema(),
	})
}

func aiRunToolCallSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"id":             positiveIntegerSchema(),
		"tool_id":        positiveIntegerSchema(),
		"tool_code":      stringSchema(),
		"tool_name":      stringSchema(),
		"call_id":        nullableSchema(stringSchema()),
		"status":         stringEnumSchema("running", "success", "failed", "timeout"),
		"arguments_json": schemaReference("JSONValue"),
		"result_json":    schemaReference("JSONValue"),
		"error_message":  stringSchema(),
		"duration_ms":    nullableSchema(nonNegativeIntegerSchema()),
		"started_at":     stringSchema(),
		"finished_at":    stringSchema(),
	})
}

func aiRunKnowledgeHitSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"id":                  positiveIntegerSchema(),
		"knowledge_base_id":   positiveIntegerSchema(),
		"knowledge_base_name": stringSchema(),
		"document_id":         positiveIntegerSchema(),
		"document_title":      stringSchema(),
		"chunk_id":            positiveIntegerSchema(),
		"chunk_index":         nonNegativeIntegerSchema(),
		"score":               numberSchema(),
		"rank_no":             nonNegativeIntegerSchema(),
		"content_snapshot":    stringSchema(),
		"status":              integerEnumSchema(1, 2),
		"status_name":         stringSchema(),
		"skip_reason":         stringSchema(),
		"created_at":          stringSchema(),
	})
}

func aiRunKnowledgeRetrievalSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"id":            positiveIntegerSchema(),
		"run_id":        positiveIntegerSchema(),
		"query":         stringSchema(),
		"status":        stringEnumSchema("success", "failed", "skipped"),
		"status_name":   stringSchema(),
		"total_hits":    nonNegativeIntegerSchema(),
		"selected_hits": nonNegativeIntegerSchema(),
		"duration_ms":   nullableSchema(nonNegativeIntegerSchema()),
		"duration_text": stringSchema(),
		"error_message": stringSchema(),
		"created_at":    stringSchema(),
		"hits":          arraySchema(schemaReference("AIRunKnowledgeHit")),
	})
}

func aiRunDetailSchema() map[string]any {
	properties := aiRunListItemProperties()
	properties["username"] = stringSchema()
	properties["user_message"] = nullableSchema(schemaReference("AIRunMessageSummary"))
	properties["assistant_message"] = nullableSchema(schemaReference("AIRunMessageSummary"))
	properties["events"] = arraySchema(schemaReference("AIRunEvent"))
	properties["knowledge_retrievals"] = arraySchema(schemaReference("AIRunKnowledgeRetrieval"))
	properties["tool_calls"] = arraySchema(schemaReference("AIRunToolCall"))
	properties["billing_status"] = stringEnumSchema("pending", "held", "settled", "released", "unbilled")
	properties["billing_reason"] = stringEnumSchema(
		"pending", "held", "settled_complete_usage", "released_before_dispatch", "released_insufficient_balance",
		"released_provider_failed", "released_outcome_unknown", "unbilled_usage_incomplete", "unbilled_over_hold", "legacy_unpriced",
	)
	properties["held_amount"] = canonicalRMBAmountSchema()
	properties["actual_amount"] = canonicalRMBAmountSchema()
	properties["pricing"] = nullableSchema(schemaReference("AIRunPricing"))
	properties["usage_items"] = arraySchema(schemaReference("AIRunUsageItem"))
	properties["provider_attempts"] = arraySchema(schemaReference("AIRunProviderAttempt"))
	properties["started_at"] = stringSchema()
	properties["finished_at"] = stringSchema()
	properties["updated_at"] = stringSchema()
	return closedObjectAllProperties(properties)
}

func aiRunPricingRateSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"category":   stringEnumSchema("input", "output", "cache_read", "cache_write", "media"),
		"tier_key":   stringSchema(),
		"unit":       nonEmptyStringSchema(),
		"price":      canonicalRMBAmountSchema(),
		"unit_scale": positiveIntegerSchema(),
	})
}

func aiRunPricingSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"version":            nonEmptyStringSchema(),
		"catalog_vendor":     nonEmptyStringSchema(),
		"transport_engine":   nonEmptyStringSchema(),
		"model_id":           nonEmptyStringSchema(),
		"resolved_alias":     stringSchema(),
		"billing_multiplier": map[string]any{"type": "string", "pattern": `^(0\.[0-9]{0,5}[1-9]|[1-9][0-9]*(\.[0-9]{0,5}[1-9])?)$`},
		"max_output_tokens":  positiveIntegerSchema(),
		"rates":              nonEmptyArraySchema(schemaReference("AIRunPricingRate")),
	})
}

func aiRunUsageItemSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"attempt_no": positiveIntegerSchema(),
		"category":   stringEnumSchema("input", "output", "cache_read", "cache_write", "media"),
		"tier_key":   stringSchema(),
		"quantity":   nonNegativeIntegerSchema(),
		"unit":       nonEmptyStringSchema(),
		"unit_price": canonicalRMBAmountSchema(),
		"unit_scale": positiveIntegerSchema(),
		"amount":     canonicalRMBAmountSchema(),
		"billable":   booleanSchema(),
	})
}

func aiRunProviderAttemptSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"attempt_no":          positiveIntegerSchema(),
		"state":               stringEnumSchema("prepared", "dispatched", "succeeded", "failed", "canceled", "outcome_unknown"),
		"provider_request_id": nullableSchema(nonEmptyStringSchema()),
		"usage_status":        stringEnumSchema("complete", "unavailable"),
	})
}

func canonicalRMBAmountSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": `^(0|[1-9][0-9]*)(\.[0-9]{0,7}[1-9])?$`}
}

func aiRunStatsSummarySchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"total_runs":              nonNegativeIntegerSchema(),
		"success_rate":            schemaWith(numberSchema(), "minimum", 0, "maximum", 100),
		"fail_runs":               nonNegativeIntegerSchema(),
		"total_tokens":            nonNegativeIntegerSchema(),
		"total_prompt_tokens":     nonNegativeIntegerSchema(),
		"total_completion_tokens": nonNegativeIntegerSchema(),
		"avg_duration_ms":         nonNegativeIntegerSchema(),
	})
}

func aiRunStatsMetricProperties() map[string]any {
	return map[string]any{
		"total_runs":              nonNegativeIntegerSchema(),
		"total_tokens":            nonNegativeIntegerSchema(),
		"total_prompt_tokens":     nonNegativeIntegerSchema(),
		"total_completion_tokens": nonNegativeIntegerSchema(),
		"avg_duration_ms":         nonNegativeIntegerSchema(),
	}
}

func aiRunStatsMetricSchema() map[string]any {
	return closedObjectAllProperties(aiRunStatsMetricProperties())
}

func aiRunStatsByDateItemSchema() map[string]any {
	properties := aiRunStatsMetricProperties()
	properties["date"] = stringSchema()
	return closedObjectAllProperties(properties)
}

func aiRunStatsByAgentItemSchema() map[string]any {
	properties := aiRunStatsMetricProperties()
	properties["agent_id"] = positiveIntegerSchema()
	properties["agent_name"] = stringSchema()
	return closedObjectAllProperties(properties)
}

func aiRunStatsByUserItemSchema() map[string]any {
	properties := aiRunStatsMetricProperties()
	properties["username"] = stringSchema()
	return closedObjectAllProperties(properties)
}
