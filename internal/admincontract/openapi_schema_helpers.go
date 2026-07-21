package admincontract

import (
	"fmt"
	"sort"

	"admin_back_go/internal/shared/enum"
)

func workflowOpenAPISchemas() (map[string]any, error) {
	schemas := make(map[string]any)
	for _, group := range []map[string]any{
		commonWorkflowSchemas(),
		identityWorkflowSchemas(),
		aiWorkflowSchemas(),
	} {
		for name, schema := range group {
			if _, duplicate := schemas[name]; duplicate {
				return nil, fmt.Errorf("duplicate workflow OpenAPI schema %s", name)
			}
			schemas[name] = schema
		}
	}
	return schemas, nil
}

func commonWorkflowSchemas() map[string]any {
	return map[string]any{
		"EmptySuccessEnvelope": successEnvelopeWithData(closedObjectSchema(nil, nil)),
		"Page": closedObjectAllProperties(map[string]any{
			"page_size":    positiveIntegerSchema(),
			"current_page": positiveIntegerSchema(),
			"total_page":   nonNegativeIntegerSchema(),
			"total":        nonNegativeIntegerSchema(),
		}),
		"IntOption": closedObjectAllProperties(map[string]any{
			"label": stringSchema(),
			"value": integerSchema(),
		}),
		"StringOption": closedObjectAllProperties(map[string]any{
			"label": stringSchema(),
			"value": stringSchema(),
		}),
		"JSONValue": map[string]any{
			"description": "Any valid JSON value explicitly stored in a json.RawMessage field; invalid or absent stored JSON is normalized to an empty object.",
		},
	}
}

func schemaReference(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func closedObjectSchema(required []string, properties map[string]any) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = append([]string(nil), required...)
	}
	return schema
}

func closedObjectAllProperties(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for name := range properties {
		required = append(required, name)
	}
	sort.Strings(required)
	return closedObjectSchema(required, properties)
}

func arraySchema(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}

func nonEmptyArraySchema(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "minItems": 1, "items": items}
}

func nullableSchema(schema map[string]any) map[string]any {
	return map[string]any{
		"anyOf": []any{
			schema,
			map[string]any{"type": "null"},
		},
	}
}

func stringSchema() map[string]any {
	return map[string]any{"type": "string"}
}

func nonEmptyStringSchema() map[string]any {
	return map[string]any{"type": "string", "minLength": 1}
}

func maxStringSchema(maximum int) map[string]any {
	return map[string]any{"type": "string", "maxLength": maximum}
}

func stringEnumSchema(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": append([]string(nil), values...)}
}

func registeredPlatformSchema() map[string]any {
	return stringEnumSchema(enum.RegisteredPlatforms()...)
}

func integerSchema() map[string]any {
	return map[string]any{"type": "integer", "format": "int64"}
}

func positiveIntegerSchema() map[string]any {
	return map[string]any{"type": "integer", "format": "int64", "minimum": 1}
}

func nonNegativeIntegerSchema() map[string]any {
	return map[string]any{"type": "integer", "format": "int64", "minimum": 0}
}

func integerRangeSchema(minimum int64, maximum int64) map[string]any {
	return map[string]any{"type": "integer", "format": "int64", "minimum": minimum, "maximum": maximum}
}

func integerEnumSchema(values ...int) map[string]any {
	return map[string]any{"type": "integer", "enum": append([]int(nil), values...)}
}

func numberSchema() map[string]any {
	return map[string]any{"type": "number"}
}

func booleanSchema() map[string]any {
	return map[string]any{"type": "boolean"}
}

func schemaWith(schema map[string]any, attributes ...any) map[string]any {
	if len(attributes)%2 != 0 {
		panic("schema attributes must be key/value pairs")
	}
	for index := 0; index < len(attributes); index += 2 {
		key, ok := attributes[index].(string)
		if !ok || key == "" {
			panic("schema attribute key must be a non-empty string")
		}
		schema[key] = attributes[index+1]
	}
	return schema
}

func idListRequestSchema(description string) map[string]any {
	return schemaWith(closedObjectSchema([]string{"ids"}, map[string]any{
		"ids": nonEmptyArraySchema(positiveIntegerSchema()),
	}), "description", description)
}
