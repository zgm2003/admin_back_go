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
		}),
		"AIAttachmentRequest": closedObjectAllProperties(map[string]any{
			"type":       stringEnumSchema("image", "file"),
			"object_key": schemaWith(maxStringSchema(1024), "minLength", 1),
			"mime_type":  schemaWith(maxStringSchema(255), "minLength", 1),
			"url":        schemaWith(maxStringSchema(2048), "minLength", 1),
			"name":       schemaWith(maxStringSchema(255), "minLength", 1),
			"size":       positiveIntegerSchema(),
		}),
		"AIMessageSendRequest": aiMessageSendRequestSchema(),
		"AIMessageMetaAttachment": closedObjectSchema([]string{"type", "url", "name", "size"}, map[string]any{
			"type":       stringEnumSchema("image", "file"),
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
		"AIMessageCitationSource": closedObjectAllProperties(map[string]any{
			"key": stringSchema(), "cited": booleanSchema(), "title": stringSchema(),
			"locator": schemaReference("AIContextLocator"), "document_id": positiveIntegerSchema(),
			"document_version_id": positiveIntegerSchema(),
		}),
		"AIMessageContext": closedObjectAllProperties(map[string]any{
			"plan_id": positiveIntegerSchema(), "outcome": stringEnumSchema("skipped", "no_hit", "hit", "failed"),
			"sources": arraySchema(schemaReference("AIMessageCitationSource")), "invalid_keys": arraySchema(stringSchema()),
		}),
		"AIMessageItem": closedObjectSchema(
			[]string{"id", "role", "content_type", "content", "paired_message_id", "run_id", "liked", "delivery_state", "settlement_pending", "context", "created_at", "updated_at"},
			map[string]any{
				"id":                 positiveIntegerSchema(),
				"role":               integerEnumSchema(1, 2, 3),
				"content_type":       stringSchema(),
				"content":            stringSchema(),
				"meta_json":          schemaReference("AIMessageMeta"),
				"paired_message_id":  nullableSchema(positiveIntegerSchema()),
				"run_id":             nullableSchema(positiveIntegerSchema()),
				"liked":              booleanSchema(),
				"delivery_state":     nullableSchema(stringEnumSchema("completed", "stopped")),
				"settlement_pending": booleanSchema(),
				"context":            nullableSchema(schemaReference("AIMessageContext")),
				"created_at":         stringSchema(),
				"updated_at":         stringSchema(),
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
		"AIMessageRevisionRequest": closedObjectSchema([]string{"content", "request_id"}, map[string]any{
			"content":    schemaWith(maxStringSchema(20000), "minLength", 1, "description", "Trimmed content must be non-empty."),
			"request_id": schemaWith(maxStringSchema(128), "minLength", 1),
			"attachments": schemaWith(
				arraySchema(schemaReference("AIAttachmentRequest")),
				"maxItems", 5,
				"description", "Omit to preserve existing attachments; send an empty array to remove all attachments.",
			),
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
		"AIMessageCancelRequest": closedObjectSchema([]string{"request_id", "delivered_seq"}, map[string]any{
			"request_id":    schemaWith(maxStringSchema(128), "minLength", 1),
			"delivered_seq": nonNegativeIntegerSchema(),
		}),
		"AIMessageCancelResult": closedObjectAllProperties(map[string]any{
			"conversation_id":      positiveIntegerSchema(),
			"request_id":           stringSchema(),
			"status":               stringEnumSchema("stopped", "already_terminal"),
			"assistant_message_id": nullableSchema(positiveIntegerSchema()),
			"settlement_pending":   booleanSchema(),
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
		"AIContextLocator":           aiContextLocatorSchema(),
		"AIContextPlanProfile":       aiContextPlanProfileSchema(),
		"AIContextPlanError":         aiContextPlanErrorSchema(),
		"AIContextPlanBudget":        aiContextPlanBudgetSchema(),
		"AIContextPlanMetrics":       aiContextPlanMetricsSchema(),
		"AIContextPlanItem":          aiContextPlanItemSchema(),
		"AIContextPlan":              aiContextPlanSchema(),
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
	schema["description"] = "request_id is required; additionally, trimmed content must be non-empty or attachments must contain at least one attachment. The cross-field rule is also published on the operation."
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

func aiContextLocatorSchema() map[string]any {
	return closedObjectSchema([]string{"schema", "kind"}, map[string]any{
		"schema": stringEnumSchema("context_locator_v1"), "kind": nonEmptyStringSchema(),
		"page": nullableSchema(nonNegativeIntegerSchema()), "paragraph": nullableSchema(nonNegativeIntegerSchema()),
		"line_start": nullableSchema(nonNegativeIntegerSchema()), "line_end": nullableSchema(nonNegativeIntegerSchema()),
		"row_start": nullableSchema(nonNegativeIntegerSchema()), "row_end": nullableSchema(nonNegativeIntegerSchema()),
		"sheet": nullableSchema(stringSchema()), "cell_start": nullableSchema(stringSchema()), "cell_end": nullableSchema(stringSchema()),
		"heading_path": arraySchema(stringSchema()),
	})
}

func aiContextPlanProfileSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"id": positiveIntegerSchema(), "index_generation": nullableSchema(positiveIntegerSchema()),
	})
}

func aiContextPlanErrorSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"stage": nonEmptyStringSchema(), "code": nonEmptyStringSchema(), "message": nullableSchema(stringSchema()),
	})
}

func aiContextPlanBudgetSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"context_window_tokens": positiveIntegerSchema(), "effective_output_tokens": positiveIntegerSchema(),
		"provider_protocol_upper_bound": nonNegativeIntegerSchema(), "tool_continuation_input_reserve": nonNegativeIntegerSchema(),
		"policy_safety_margin": nonNegativeIntegerSchema(), "known_input_budget": nonNegativeIntegerSchema(),
		"known_input_upper_bound": nonNegativeIntegerSchema(), "proof": stringEnumSchema("exact", "conservative", "opaque_attachment"),
	})
}

func aiContextPlanMetricsSchema() map[string]any {
	return closedObjectSchema([]string{"schema"}, map[string]any{
		"schema":           stringEnumSchema("context_plan_metrics_v1"),
		"authorization_ms": nonNegativeIntegerSchema(), "conversation_ms": nonNegativeIntegerSchema(),
		"query_embedding_ms": nonNegativeIntegerSchema(), "retrieval_ms": nonNegativeIntegerSchema(),
		"rerank_ms": nonNegativeIntegerSchema(), "packing_ms": nonNegativeIntegerSchema(),
		"candidate_count": nonNegativeIntegerSchema(), "query_embedding_request_count": nonNegativeIntegerSchema(),
		"rerank_request_count": nonNegativeIntegerSchema(), "query_input_tokens": nullableSchema(nonNegativeIntegerSchema()),
		"rerank_input_tokens": nullableSchema(nonNegativeIntegerSchema()),
	})
}

func aiContextPlanItemSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"ordinal":     nonNegativeIntegerSchema(),
		"kind":        stringEnumSchema("system_instruction", "current_user_message", "current_attachment", "recent_turn", "recalled_turn", "history_attachment", "conversation_memory", "document_evidence", "tool_definition", "tool_call", "tool_result"),
		"source_type": stringSchema(), "source_ref": stringSchema(), "required": booleanSchema(), "priority": integerSchema(),
		"token_upper_bound": nonNegativeIntegerSchema(), "decision": stringEnumSchema("selected", "excluded"),
		"exclusion_reason": nullableSchema(stringEnumSchema("budget_exceeded", "duplicate_content", "below_relevance_threshold", "superseded_memory", "inactive_source", "permission_changed", "unsupported_attachment")),
		"fusion_score":     nullableSchema(stringSchema()), "rerank_score": nullableSchema(stringSchema()), "citation_key": nullableSchema(stringSchema()),
		"title": stringSchema(), "locator": nullableSchema(schemaReference("AIContextLocator")),
		"document_id": nonNegativeIntegerSchema(), "document_version_id": nonNegativeIntegerSchema(),
		"content_snapshot": stringSchema(), "content_truncated": booleanSchema(),
	})
}

func aiContextPlanSchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"id": positiveIntegerSchema(), "profile": nullableSchema(schemaReference("AIContextPlanProfile")),
		"policy_version": nonEmptyStringSchema(), "api_protocol": stringEnumSchema("chat_completions", "responses"),
		"token_counter_id": nonEmptyStringSchema(), "retrieval_outcome": stringEnumSchema("skipped", "no_hit", "hit", "failed"),
		"state": stringEnumSchema("ready", "failed"), "error": nullableSchema(schemaReference("AIContextPlanError")),
		"budget": schemaReference("AIContextPlanBudget"), "metrics": schemaReference("AIContextPlanMetrics"),
		"items": arraySchema(schemaReference("AIContextPlanItem")),
	})
}

func aiRunDetailSchema() map[string]any {
	properties := aiRunListItemProperties()
	properties["username"] = stringSchema()
	properties["user_message"] = nullableSchema(schemaReference("AIRunMessageSummary"))
	properties["assistant_message"] = nullableSchema(schemaReference("AIRunMessageSummary"))
	properties["events"] = arraySchema(schemaReference("AIRunEvent"))
	properties["context_plan"] = nullableSchema(schemaReference("AIContextPlan"))
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
	for _, field := range []string{"accept_ms", "queue_ms", "prepare_ms", "cos_head_ms", "cos_stream_ms", "ttft_ms", "provider_total_ms", "settlement_ms", "end_to_end_ms"} {
		properties[field] = nullableSchema(nonNegativeIntegerSchema())
	}
	return closedObjectAllProperties(properties)
}

func aiRunRequestSummarySchema() map[string]any {
	return closedObjectAllProperties(map[string]any{
		"provider_attempt_count":     nonNegativeIntegerSchema(),
		"tool_call_count":            nonNegativeIntegerSchema(),
		"prepared_request_bytes":     nonNegativeIntegerSchema(),
		"message_count":              nullableSchema(nonNegativeIntegerSchema()),
		"attachment_count":           nonNegativeIntegerSchema(),
		"native_file_count":          nonNegativeIntegerSchema(),
		"native_file_bytes":          nonNegativeIntegerSchema(),
		"prepared_manifest_bytes":    nonNegativeIntegerSchema(),
		"materialized_request_bytes": nonNegativeIntegerSchema(),
		"api_protocol":               stringEnumSchema("", "chat_completions", "responses"),
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
