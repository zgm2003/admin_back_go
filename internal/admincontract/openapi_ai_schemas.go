package admincontract

func aiWorkflowSchemas() map[string]any {
	return map[string]any{
		"AIConversationItem": closedObjectAllProperties(map[string]any{
			"id":              positiveIntegerSchema(),
			"agent_id":        positiveIntegerSchema(),
			"agent_name":      stringSchema(),
			"title":           stringSchema(),
			"unread_count":    nonNegativeIntegerSchema(),
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
			"max_history": integerRangeSchema(1, 50),
		}),
		"AIAttachmentRequest": closedObjectSchema([]string{"type", "object_key"}, map[string]any{
			"type":       map[string]any{"type": "string", "const": "image"},
			"object_key": nonEmptyStringSchema(),
			"name":       stringSchema(),
		}),
		"AIMessageSendRequest": aiMessageSendRequestSchema(),
		"AIMessageMetaAttachment": closedObjectSchema([]string{"type", "url", "name", "size"}, map[string]any{
			"type":       map[string]any{"type": "string", "const": "image"},
			"object_key": nonEmptyStringSchema(),
			"mime_type":  nonEmptyStringSchema(),
			"url":        nonEmptyStringSchema(),
			"name":       stringSchema(),
			"size":       nonNegativeIntegerSchema(),
		}),
		"AIMessageMeta": closedObjectSchema(nil, map[string]any{
			"attachments":    arraySchema(schemaReference("AIMessageMetaAttachment")),
			"runtime_params": schemaReference("AIRuntimeParams"),
		}),
		"AIMessageItem": closedObjectSchema(
			[]string{"id", "role", "content_type", "content", "paired_message_id", "run_id", "liked", "created_at", "updated_at"},
			map[string]any{
				"id":                positiveIntegerSchema(),
				"role":              integerEnumSchema(1, 2, 3),
				"content_type":      stringSchema(),
				"content":           stringSchema(),
				"meta_json":         schemaReference("AIMessageMeta"),
				"paired_message_id": nullableSchema(positiveIntegerSchema()),
				"run_id":            nullableSchema(positiveIntegerSchema()),
				"liked":             booleanSchema(),
				"created_at":        stringSchema(),
				"updated_at":        stringSchema(),
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
		"AIMessageRevisionRequest": closedObjectAllProperties(map[string]any{
			"content":    schemaWith(maxStringSchema(20000), "minLength", 1, "description", "Trimmed content must be non-empty."),
			"request_id": schemaWith(maxStringSchema(128), "minLength", 1),
		}),
		"AIMessageRegenerationRequest": closedObjectAllProperties(map[string]any{
			"request_id": schemaWith(maxStringSchema(128), "minLength", 1),
		}),
		"AIMessageDeleteRequest": closedObjectAllProperties(map[string]any{
			"ids": schemaWith(nonEmptyArraySchema(positiveIntegerSchema()), "uniqueItems", true),
		}),
		"AIMessageDeleteResult": closedObjectAllProperties(map[string]any{
			"deleted_ids": schemaWith(
				nonEmptyArraySchema(positiveIntegerSchema()),
				"uniqueItems", true,
				"description", "The submitted message IDs normalized to unique ascending order.",
			),
		}),
		"AIMessageDeleteSuccessEnvelope": successEnvelopeWithData(schemaReference("AIMessageDeleteResult")),
		"AIMessageCancelRequest": closedObjectSchema([]string{"request_id"}, map[string]any{
			"request_id": schemaWith(maxStringSchema(128), "minLength", 1),
		}),
		"AIMessageCancelResult": closedObjectAllProperties(map[string]any{
			"conversation_id": positiveIntegerSchema(),
			"request_id":      stringSchema(),
			"status":          map[string]any{"type": "string", "const": "stopping"},
		}),
		"AIMessageCancelSuccessEnvelope": successEnvelopeWithData(schemaReference("AIMessageCancelResult")),
		"AIConversationReadCursorRequest": closedObjectAllProperties(map[string]any{
			"message_id": positiveIntegerSchema(),
		}),
		"AIConversationReadCursorResult": closedObjectAllProperties(map[string]any{
			"conversation_id":      positiveIntegerSchema(),
			"last_read_message_id": positiveIntegerSchema(),
			"unread_count":         nonNegativeIntegerSchema(),
		}),
		"AIConversationReadCursorSuccessEnvelope": successEnvelopeWithData(schemaReference("AIConversationReadCursorResult")),

		"AIRunPageInitModelOption": closedObjectAllProperties(map[string]any{
			"label":      stringSchema(),
			"value":      stringSchema(),
			"historical": booleanSchema(),
		}),
		"AIRunPageInitDict": closedObjectAllProperties(map[string]any{
			"status_arr":         arraySchema(schemaReference("StringOption")),
			"platform_arr":       arraySchema(schemaReference("StringOption")),
			"providerArr":        arraySchema(schemaReference("IntOption")),
			"agentArr":           arraySchema(schemaReference("IntOption")),
			"model_arr":          arraySchema(schemaReference("AIRunPageInitModelOption")),
			"billing_status_arr": arraySchema(schemaReference("StringOption")),
			"billing_reason_arr": arraySchema(schemaReference("StringOption")),
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
		"AIRunLatencyBreakdown":      aiRunLatencyBreakdownSchema(),
		"AIRunRequestSummary":        aiRunRequestSummarySchema(),
		"AIRunDetail":                aiRunDetailSchema(),
		"AIRunDetailSuccessEnvelope": successEnvelopeWithData(schemaReference("AIRunDetail")),
		"AIRunUserFeedbackRequest": closedObjectAllProperties(map[string]any{
			"liked": booleanSchema(),
		}),
		"AIRunUserFeedbackResult": closedObjectAllProperties(map[string]any{
			"id":       positiveIntegerSchema(),
			"liked":    booleanSchema(),
			"liked_at": nullableSchema(stringSchema()),
		}),
		"AIRunUserFeedbackSuccessEnvelope": successEnvelopeWithData(schemaReference("AIRunUserFeedbackResult")),
		"AIRunDashboardDateRange":          aiRunDashboardDateRangeSchema(),
		"AIRunDashboardSummary":            aiRunDashboardSummarySchema(),
		"AIRunDashboardPercentile":         aiRunDashboardPercentileSchema(),
		"AIRunDashboardPerformance":        aiRunDashboardPerformanceSchema(),
		"AIRunDashboardBilling":            aiRunDashboardBillingSchema(),
		"AIRunDashboardAnomalyItem":        aiRunDashboardAnomalyItemSchema(),
		"AIRunDashboardAnomalies":          aiRunDashboardAnomaliesSchema(),
		"AIRunDashboardTrendItem":          aiRunDashboardTrendItemSchema(),
		"AIRunDashboardAttributionMetrics": aiRunDashboardAttributionMetricsSchema(),
		"AIRunDashboardModelBreakdown":     aiRunDashboardModelBreakdownSchema(),
		"AIRunDashboardProviderBreakdown":  aiRunDashboardProviderBreakdownSchema(),
		"AIRunDashboardAgentBreakdown":     aiRunDashboardAgentBreakdownSchema(),
		"AIRunDashboardUserBreakdown":      aiRunDashboardUserBreakdownSchema(),
		"AIRunDashboardErrorBreakdown":     aiRunDashboardErrorBreakdownSchema(),
		"AIRunDashboardToolBreakdown":      aiRunDashboardToolBreakdownSchema(),
		"AIRunDashboardBreakdowns":         aiRunDashboardBreakdownsSchema(),
		"AIRunDashboardResult":             aiRunDashboardResultSchema(),
		"AIRunDashboardSuccessEnvelope":    successEnvelopeWithData(schemaReference("AIRunDashboardResult")),
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
			"running", "success", "failed", "canceled", "timeout", "outcome_unknown",
		),
		"status_name":        stringSchema(),
		"model_id":           stringSchema(),
		"model_display_name": stringSchema(),
		"billing_status":     stringEnumSchema("pending", "held", "settled", "released", "unbilled"),
		"billing_reason": stringEnumSchema(
			"pending", "held", "settled_complete_usage", "released_before_dispatch", "released_insufficient_balance",
			"released_provider_failed", "released_outcome_unknown", "unbilled_usage_incomplete", "unbilled_over_hold", "legacy_unpriced",
		),
		"error_code":        stringSchema(),
		"liked":             booleanSchema(),
		"liked_at":          nullableSchema(stringSchema()),
		"prompt_tokens":     nonNegativeIntegerSchema(),
		"completion_tokens": nonNegativeIntegerSchema(),
		"total_tokens":      nonNegativeIntegerSchema(),
		"duration_ms":       nullableSchema(nonNegativeIntegerSchema()),
		"duration_text":     stringSchema(),
		"error_message":     stringSchema(),
		"created_at":        stringSchema(),
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
		"id":  positiveIntegerSchema(),
		"seq": nonNegativeIntegerSchema(),
		"event_type": stringEnumSchema(
			"start", "completed", "failed", "canceled", "timeout", "retry_scheduled",
			"usage_recorded", "outcome_unknown", "settled", "released", "unbilled",
		),
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
	properties["latency"] = schemaReference("AIRunLatencyBreakdown")
	properties["request_summary"] = schemaReference("AIRunRequestSummary")
	properties["liked"] = booleanSchema()
	properties["liked_at"] = nullableSchema(stringSchema())
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

func aiRunLatencyBreakdownSchema() map[string]any {
	properties := map[string]any{
		"claim_source": stringEnumSchema("", "wake", "poll", "recovery"),
	}
	for _, field := range []string{"accept_ms", "queue_ms", "prepare_ms", "ttft_ms", "provider_total_ms", "settlement_ms", "end_to_end_ms"} {
		properties[field] = nullableSchema(nonNegativeIntegerSchema())
	}
	return closedObjectAllProperties(properties)
}

func aiRunRequestSummarySchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"provider_attempt_count": nonNegativeIntegerSchema(),
		"tool_call_count":        nonNegativeIntegerSchema(),
		"prepared_request_bytes": nonNegativeIntegerSchema(),
		"message_count":          nullableSchema(nonNegativeIntegerSchema()),
	})
}

func canonicalRMBAmountSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": `^(0|[1-9][0-9]*)(\.[0-9]{0,7}[1-9])?$`}
}

func aiRunDashboardDateRangeSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"start_at":      schemaWith(stringSchema(), "format", "date-time"),
		"end_exclusive": schemaWith(stringSchema(), "format", "date-time"),
	})
}

func aiRunDashboardSummarySchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"total_runs":           nonNegativeIntegerSchema(),
		"terminal_runs":        nonNegativeIntegerSchema(),
		"in_progress_runs":     nonNegativeIntegerSchema(),
		"success_runs":         nonNegativeIntegerSchema(),
		"failed_runs":          nonNegativeIntegerSchema(),
		"timeout_runs":         nonNegativeIntegerSchema(),
		"outcome_unknown_runs": nonNegativeIntegerSchema(),
		"canceled_runs":        nonNegativeIntegerSchema(),
		"success_denominator":  nonNegativeIntegerSchema(),
		"success_rate":         schemaWith(numberSchema(), "minimum", 0, "maximum", 100),
		"prompt_tokens":        nonNegativeIntegerSchema(),
		"completion_tokens":    nonNegativeIntegerSchema(),
		"total_tokens":         nonNegativeIntegerSchema(),
	})
}

func aiRunDashboardPercentileSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"sample_count":        nonNegativeIntegerSchema(),
		"insufficient_sample": booleanSchema(),
		"p50_ms":              nonNegativeIntegerSchema(),
		"p95_ms":              nonNegativeIntegerSchema(),
	})
}

func aiRunDashboardPerformanceSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"ttft":       schemaReference("AIRunDashboardPercentile"),
		"end_to_end": schemaReference("AIRunDashboardPercentile"),
	})
}

func aiRunDashboardBillingSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"settled_runs":    nonNegativeIntegerSchema(),
		"actual_amount":   canonicalRMBAmountSchema(),
		"released_runs":   nonNegativeIntegerSchema(),
		"released_amount": canonicalRMBAmountSchema(),
		"unbilled_runs":   nonNegativeIntegerSchema(),
	})
}

func aiRunDashboardAnomalyItemSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"code":  stringSchema(),
		"count": nonNegativeIntegerSchema(),
	})
}

func aiRunDashboardAnomaliesSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"run_total":     nonNegativeIntegerSchema(),
		"billing_total": nonNegativeIntegerSchema(),
		"run_items":     arraySchema(schemaReference("AIRunDashboardAnomalyItem")),
		"billing_items": arraySchema(schemaReference("AIRunDashboardAnomalyItem")),
	})
}

func aiRunDashboardAttributionMetricProperties() map[string]any {
	return map[string]any{
		"total_runs":            nonNegativeIntegerSchema(),
		"success_runs":          nonNegativeIntegerSchema(),
		"success_denominator":   nonNegativeIntegerSchema(),
		"success_rate":          schemaWith(numberSchema(), "minimum", 0, "maximum", 100),
		"total_tokens":          nonNegativeIntegerSchema(),
		"actual_amount":         canonicalRMBAmountSchema(),
		"run_anomaly_count":     nonNegativeIntegerSchema(),
		"billing_anomaly_count": nonNegativeIntegerSchema(),
	}
}

func aiRunDashboardAttributionMetricsSchema() map[string]any {
	return closedObjectAllProperties(aiRunDashboardAttributionMetricProperties())
}

func aiRunDashboardModelBreakdownSchema() map[string]any {
	properties := aiRunDashboardAttributionMetricProperties()
	properties["model_id"] = stringSchema()
	properties["model_display_name"] = stringSchema()
	properties["historical"] = booleanSchema()
	return closedObjectAllProperties(properties)
}

func aiRunDashboardProviderBreakdownSchema() map[string]any {
	properties := aiRunDashboardAttributionMetricProperties()
	properties["provider_id"] = positiveIntegerSchema()
	properties["provider_name"] = stringSchema()
	return closedObjectAllProperties(properties)
}

func aiRunDashboardAgentBreakdownSchema() map[string]any {
	properties := aiRunDashboardAttributionMetricProperties()
	properties["agent_id"] = positiveIntegerSchema()
	properties["agent_name"] = stringSchema()
	return closedObjectAllProperties(properties)
}

func aiRunDashboardUserBreakdownSchema() map[string]any {
	properties := aiRunDashboardAttributionMetricProperties()
	properties["user_id"] = positiveIntegerSchema()
	properties["username"] = stringSchema()
	return closedObjectAllProperties(properties)
}

func aiRunDashboardErrorBreakdownSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"error_code": stringSchema(),
		"count":      nonNegativeIntegerSchema(),
	})
}

func aiRunDashboardToolBreakdownSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"tool_code":           stringSchema(),
		"tool_name":           stringSchema(),
		"total_calls":         nonNegativeIntegerSchema(),
		"success_calls":       nonNegativeIntegerSchema(),
		"failed_calls":        nonNegativeIntegerSchema(),
		"timeout_calls":       nonNegativeIntegerSchema(),
		"success_denominator": nonNegativeIntegerSchema(),
		"success_rate":        schemaWith(numberSchema(), "minimum", 0, "maximum", 100),
		"duration":            schemaReference("AIRunDashboardPercentile"),
	})
}

func aiRunDashboardTrendItemSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"date":                 schemaWith(stringSchema(), "format", "date"),
		"total_runs":           nonNegativeIntegerSchema(),
		"in_progress_runs":     nonNegativeIntegerSchema(),
		"success_runs":         nonNegativeIntegerSchema(),
		"failed_runs":          nonNegativeIntegerSchema(),
		"canceled_runs":        nonNegativeIntegerSchema(),
		"timeout_runs":         nonNegativeIntegerSchema(),
		"outcome_unknown_runs": nonNegativeIntegerSchema(),
		"success_denominator":  nonNegativeIntegerSchema(),
		"success_rate":         schemaWith(numberSchema(), "minimum", 0, "maximum", 100),
		"actual_amount":        canonicalRMBAmountSchema(),
		"ttft":                 schemaReference("AIRunDashboardPercentile"),
		"end_to_end":           schemaReference("AIRunDashboardPercentile"),
	})
}

func aiRunDashboardBreakdownsSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"models":    arraySchema(schemaReference("AIRunDashboardModelBreakdown")),
		"providers": arraySchema(schemaReference("AIRunDashboardProviderBreakdown")),
		"agents":    arraySchema(schemaReference("AIRunDashboardAgentBreakdown")),
		"users":     arraySchema(schemaReference("AIRunDashboardUserBreakdown")),
		"errors":    arraySchema(schemaReference("AIRunDashboardErrorBreakdown")),
		"tools":     arraySchema(schemaReference("AIRunDashboardToolBreakdown")),
	})
}

func aiRunDashboardResultSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"generated_at": schemaWith(stringSchema(), "format", "date-time"),
		"timezone":     stringSchema(),
		"date_range":   schemaReference("AIRunDashboardDateRange"),
		"summary":      schemaReference("AIRunDashboardSummary"),
		"performance":  schemaReference("AIRunDashboardPerformance"),
		"billing":      schemaReference("AIRunDashboardBilling"),
		"anomalies":    schemaReference("AIRunDashboardAnomalies"),
		"trend":        arraySchema(schemaReference("AIRunDashboardTrendItem")),
		"breakdowns":   schemaReference("AIRunDashboardBreakdowns"),
	})
}
