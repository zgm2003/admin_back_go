package admincontract

func identityWorkflowSchemas() map[string]any {
	return map[string]any{
		"AddressTreeNode": closedObjectSchema(
			[]string{"id", "parent_id", "label", "value"},
			map[string]any{
				"id":        positiveIntegerSchema(),
				"parent_id": nonNegativeIntegerSchema(),
				"label":     stringSchema(),
				"value":     positiveIntegerSchema(),
				"children":  arraySchema(schemaReference("AddressTreeNode")),
			},
		),
		"UserPageInitDict": closedObjectAllProperties(map[string]any{
			"roleArr":           arraySchema(schemaReference("IntOption")),
			"auth_address_tree": arraySchema(schemaReference("AddressTreeNode")),
			"sexArr":            arraySchema(schemaReference("IntOption")),
			"platformArr":       arraySchema(schemaReference("StringOption")),
		}),
		"UserPageInit": closedObjectAllProperties(map[string]any{
			"dict": schemaReference("UserPageInitDict"),
		}),
		"UserPageInitSuccessEnvelope": successEnvelopeWithData(schemaReference("UserPageInit")),
		"UserListItem": closedObjectAllProperties(map[string]any{
			"id":             positiveIntegerSchema(),
			"username":       stringSchema(),
			"email":          stringSchema(),
			"phone":          stringSchema(),
			"avatar":         nullableSchema(stringSchema()),
			"sex":            integerEnumSchema(0, 1, 2),
			"sex_show":       stringSchema(),
			"role_id":        positiveIntegerSchema(),
			"role_name":      stringSchema(),
			"bio":            stringSchema(),
			"address_show":   stringSchema(),
			"address_id":     nonNegativeIntegerSchema(),
			"detail_address": stringSchema(),
			"status":         integerEnumSchema(1, 2),
			"created_at":     stringSchema(),
		}),
		"UserListResult": closedObjectAllProperties(map[string]any{
			"list": arraySchema(schemaReference("UserListItem")),
			"page": schemaReference("Page"),
		}),
		"UserListSuccessEnvelope": successEnvelopeWithData(schemaReference("UserListResult")),
		"UserUpdateRequest": closedObjectSchema(
			[]string{"username", "role_id", "address_id"},
			map[string]any{
				"username":       schemaWith(maxStringSchema(64), "minLength", 1),
				"avatar":         maxStringSchema(255),
				"role_id":        positiveIntegerSchema(),
				"sex":            integerEnumSchema(0, 1, 2),
				"address_id":     nonNegativeIntegerSchema(),
				"detail_address": maxStringSchema(255),
				"bio":            maxStringSchema(1000),
			},
		),
		"UserStatusRequest": closedObjectSchema([]string{"status"}, map[string]any{
			"status": integerEnumSchema(1, 2),
		}),
		"UserBatchProfileRequest": userBatchProfileRequestSchema(),
		"UserBatchDeleteRequest":  idListRequestSchema("Positive user IDs to delete."),
		"UserExportRequest":       idListRequestSchema("Positive user IDs included in this export task."),
		"UserExportResult": closedObjectAllProperties(map[string]any{
			"id":      positiveIntegerSchema(),
			"message": stringSchema(),
		}),
		"UserExportSuccessEnvelope": successEnvelopeWithData(schemaReference("UserExportResult")),

		"NotificationPageInitDict": closedObjectAllProperties(map[string]any{
			"notification_type_arr":        arraySchema(schemaReference("IntOption")),
			"notification_level_arr":       arraySchema(schemaReference("IntOption")),
			"notification_read_status_arr": arraySchema(schemaReference("IntOption")),
		}),
		"NotificationPageInit": closedObjectAllProperties(map[string]any{
			"dict": schemaReference("NotificationPageInitDict"),
		}),
		"NotificationPageInitSuccessEnvelope": successEnvelopeWithData(schemaReference("NotificationPageInit")),
		"NotificationItem": closedObjectAllProperties(map[string]any{
			"id":         positiveIntegerSchema(),
			"title":      stringSchema(),
			"content":    stringSchema(),
			"type":       integerEnumSchema(1, 2, 3, 4),
			"type_text":  stringSchema(),
			"level":      integerEnumSchema(1, 2),
			"level_text": stringSchema(),
			"link":       stringSchema(),
			"is_read":    integerEnumSchema(1, 2),
			"created_at": stringSchema(),
		}),
		"NotificationListResult": closedObjectAllProperties(map[string]any{
			"list":    arraySchema(schemaReference("NotificationItem")),
			"page":    schemaReference("Page"),
			"next_id": nonNegativeIntegerSchema(),
		}),
		"NotificationListSuccessEnvelope": successEnvelopeWithData(schemaReference("NotificationListResult")),
		"NotificationUnreadCountResult": closedObjectAllProperties(map[string]any{
			"count": nonNegativeIntegerSchema(),
		}),
		"NotificationUnreadCountSuccessEnvelope": successEnvelopeWithData(schemaReference("NotificationUnreadCountResult")),
		"NotificationReadRequest": closedObjectSchema(nil, map[string]any{
			"ids": arraySchema(positiveIntegerSchema()),
		}),
		"NotificationDeleteBatchRequest": idListRequestSchema("Positive notification IDs owned by the current user."),

		"ExportTaskStatusCountItem": closedObjectAllProperties(map[string]any{
			"label": stringSchema(),
			"value": integerEnumSchema(1, 2, 3),
			"num":   nonNegativeIntegerSchema(),
		}),
		"ExportTaskStatusCountSuccessEnvelope": successEnvelopeWithData(arraySchema(schemaReference("ExportTaskStatusCountItem"))),
		"ExportTaskItem": closedObjectAllProperties(map[string]any{
			"id":             positiveIntegerSchema(),
			"kind":           stringSchema(),
			"kind_text":      stringSchema(),
			"title":          stringSchema(),
			"file_name":      nullableSchema(stringSchema()),
			"file_url":       nullableSchema(stringSchema()),
			"file_size_text": stringSchema(),
			"row_count":      nullableSchema(nonNegativeIntegerSchema()),
			"status":         integerEnumSchema(1, 2, 3),
			"status_text":    stringSchema(),
			"error_msg":      nullableSchema(stringSchema()),
			"expire_at":      nullableSchema(stringSchema()),
			"created_at":     stringSchema(),
		}),
		"ExportTaskListResult": closedObjectAllProperties(map[string]any{
			"list":    arraySchema(schemaReference("ExportTaskItem")),
			"page":    schemaReference("Page"),
			"next_id": nonNegativeIntegerSchema(),
		}),
		"ExportTaskListSuccessEnvelope": successEnvelopeWithData(schemaReference("ExportTaskListResult")),
		"ExportTaskDeleteBatchRequest":  idListRequestSchema("Positive export-task IDs owned by the current user."),
	}
}

func userBatchProfileRequestSchema() map[string]any {
	ids := nonEmptyArraySchema(positiveIntegerSchema())
	return map[string]any{
		"description": "Exactly one field-specific variant is accepted.",
		"oneOf": []any{
			closedObjectSchema([]string{"ids", "field", "sex"}, map[string]any{
				"ids":   ids,
				"field": map[string]any{"type": "string", "const": "sex"},
				"sex":   integerEnumSchema(0, 1, 2),
			}),
			closedObjectSchema([]string{"ids", "field", "address_id"}, map[string]any{
				"ids":        nonEmptyArraySchema(positiveIntegerSchema()),
				"field":      map[string]any{"type": "string", "const": "address_id"},
				"address_id": positiveIntegerSchema(),
			}),
			closedObjectSchema([]string{"ids", "field"}, map[string]any{
				"ids":            nonEmptyArraySchema(positiveIntegerSchema()),
				"field":          map[string]any{"type": "string", "const": "detail_address"},
				"detail_address": maxStringSchema(255),
			}),
		},
	}
}
