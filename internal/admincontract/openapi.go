package admincontract

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"admin_back_go/internal/server/adminroute"
)

func buildOpenAPI(definitions []adminroute.Definition) (map[string]any, error) {
	paths := make(map[string]any)
	for _, definition := range definitions {
		path, parameters, err := openAPIPath(definition.Path)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", definition.Method, definition.Path, err)
		}
		method := strings.ToLower(definition.Method)
		pathItem, _ := paths[path].(map[string]any)
		if pathItem == nil {
			pathItem = make(map[string]any)
			paths[path] = pathItem
		}
		if _, duplicate := pathItem[method]; duplicate {
			return nil, fmt.Errorf("duplicate OpenAPI operation %s %s", definition.Method, path)
		}

		tags := append([]string(nil), definition.Tags...)
		if len(tags) == 0 {
			tags = []string{operationTag(definition.Path)}
		}
		operation := map[string]any{
			"operationId":    definition.OperationID,
			"tags":           tags,
			"summary":        definition.Method + " " + definition.Path,
			"x-runtime-path": definition.Path,
			"x-admin-access": definition.Access,
			"x-admin-audit":  definition.Audit,
			"responses":      operationResponses(definition),
		}
		if len(parameters) > 0 {
			operation["parameters"] = parameters
		}
		if definition.Access.Kind == adminroute.AccessPublic {
			operation["security"] = []any{}
		} else {
			operation["security"] = []any{map[string]any{"bearerAuth": []string{}}}
		}
		if requestBody := operationRequestBody(definition); requestBody != nil {
			operation["requestBody"] = requestBody
		}
		pathItem[method] = operation
	}

	return map[string]any{
		"openapi": OpenAPIVersion,
		"info": map[string]any{
			"title":       "Admin API",
			"version":     BundleVersion,
			"description": "Deterministic active Admin HTTP contract generated from the runtime route-policy registry.",
		},
		"paths": paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
				},
			},
			"schemas": openAPISchemas(),
		},
	}, nil
}

func openAPIPath(runtimePath string) (string, []map[string]any, error) {
	if runtimePath == "" || !strings.HasPrefix(runtimePath, "/") {
		return "", nil, fmt.Errorf("invalid runtime path")
	}
	segments := strings.Split(runtimePath, "/")
	parameters := make([]map[string]any, 0)
	for index, segment := range segments {
		if segment == "" {
			continue
		}
		catchAll := strings.HasPrefix(segment, "*")
		if !catchAll && !strings.HasPrefix(segment, ":") {
			continue
		}
		name := strings.TrimLeft(segment, ":*")
		if name == "" {
			return "", nil, fmt.Errorf("empty path parameter")
		}
		segments[index] = "{" + name + "}"
		parameter := map[string]any{
			"name":     name,
			"in":       "path",
			"required": true,
			"schema":   map[string]any{"type": "string", "minLength": 1},
		}
		if catchAll {
			parameter["x-gin-catch-all"] = true
		}
		parameters = append(parameters, parameter)
	}
	return strings.Join(segments, "/"), parameters, nil
}

func operationTag(path string) string {
	if path == "/health" || path == "/ready" {
		return "system"
	}
	if strings.HasPrefix(path, "/api/payment/callbacks/") {
		return "payment-callbacks"
	}
	const prefix = "/api/admin/v1/"
	remainder := strings.TrimPrefix(path, prefix)
	if remainder == path || remainder == "" {
		return "admin"
	}
	if index := strings.IndexByte(remainder, '/'); index >= 0 {
		remainder = remainder[:index]
	}
	return strings.Trim(remainder, ":*")
}

func operationResponses(definition adminroute.Definition) map[string]any {
	if definition.Path == "/api/admin/v1/realtime/ws" {
		return map[string]any{
			"101":     map[string]any{"description": "WebSocket protocol switch"},
			"default": errorResponse(),
		}
	}
	status := definition.SuccessStatus
	if status == 0 {
		status = http.StatusOK
	}
	return map[string]any{
		strconv.Itoa(status): map[string]any{
			"description": "Successful response",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": responseSchema(definition.ResponseSchema),
				},
			},
		},
		"default": errorResponse(),
	}
}

func responseSchema(name string) map[string]any {
	if strings.TrimSpace(name) == "" {
		return map[string]any{"$ref": "#/components/schemas/SuccessEnvelope"}
	}
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func errorResponse() map[string]any {
	return map[string]any{
		"description": "Classified safe error response",
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"$ref": "#/components/schemas/ErrorEnvelope"},
			},
		},
	}
}

func operationRequestBody(definition adminroute.Definition) map[string]any {
	switch definition.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return nil
	}
	mediaType := "application/json"
	if strings.HasPrefix(definition.Path, "/api/payment/callbacks/") {
		mediaType = "application/x-www-form-urlencoded"
	}
	schema := map[string]any{"$ref": "#/components/schemas/GenericObject"}
	if definition.RequestSchema != "" {
		schema = map[string]any{"$ref": "#/components/schemas/" + definition.RequestSchema}
	}
	return map[string]any{
		"required": false,
		"content": map[string]any{
			mediaType: map[string]any{"schema": schema},
		},
	}
}

func openAPISchemas() map[string]any {
	categories := []string{
		"authentication",
		"authorization",
		"canceled",
		"conflict",
		"dependency",
		"internal",
		"not_found",
		"rate_limit",
		"timeout",
		"validation",
	}
	sort.Strings(categories)
	errorMetadata := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"code", "category", "retryable"},
		"properties": map[string]any{
			"code":       map[string]any{"type": "string", "minLength": 1},
			"category":   map[string]any{"type": "string", "enum": categories},
			"retryable":  map[string]any{"type": "boolean"},
			"request_id": map[string]any{"type": "string"},
			"trace_id":   map[string]any{"type": "string"},
		},
	}
	return map[string]any{
		"GenericObject": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"SuccessEnvelope": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"code", "data", "msg"},
			"properties": map[string]any{
				"code": map[string]any{"type": "integer", "const": 0},
				"data": map[string]any{},
				"msg":  map[string]any{"type": "string"},
			},
		},
		"ErrorEnvelope": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"code", "data", "msg", "error"},
			"properties": map[string]any{
				"code":  map[string]any{"type": "integer"},
				"data":  map[string]any{},
				"msg":   map[string]any{"type": "string"},
				"error": errorMetadata,
			},
		},
	}
}
