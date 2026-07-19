package admincontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/enum"
)

type modelSchemaMode string

const (
	modelSchemaInput  modelSchemaMode = "Input"
	modelSchemaOutput modelSchemaMode = "Output"
)

type modelSchemaKey struct {
	Type reflect.Type
	Mode modelSchemaMode
}

type modelSchemaBuilder struct {
	schemas    map[string]any
	components map[modelSchemaKey]string
	owners     map[string]modelSchemaKey
	building   map[modelSchemaKey]bool
}

func buildModeledOperationContracts(definitions []adminroute.Definition, schemas map[string]any) (map[workflowOperationKey]workflowOperationContract, error) {
	builder := &modelSchemaBuilder{
		schemas:    schemas,
		components: make(map[modelSchemaKey]string),
		owners:     make(map[string]modelSchemaKey),
		building:   make(map[modelSchemaKey]bool),
	}
	contracts := make(map[workflowOperationKey]workflowOperationContract)
	for _, definition := range definitions {
		if definition.Contract == nil {
			continue
		}
		key := workflowKey(definition.Method, definition.Path)
		if _, exists := workflowOperationContracts[key]; exists {
			return nil, fmt.Errorf("%s %s declares both a workflow and runtime-model contract", definition.Method, definition.Path)
		}
		contract, err := builder.operationContract(definition)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", definition.Method, definition.Path, err)
		}
		contracts[key] = contract
	}
	return contracts, nil
}

func (builder *modelSchemaBuilder) operationContract(definition adminroute.Definition) (workflowOperationContract, error) {
	contract := definition.Contract
	if contract == nil || (modelType(contract.Response) == nil && len(contract.ResponseAlternatives) == 0) {
		return workflowOperationContract{}, fmt.Errorf("runtime-model contract response is required")
	}
	if modelType(contract.Response) != nil && len(contract.ResponseAlternatives) > 0 {
		return workflowOperationContract{}, fmt.Errorf("response and response alternatives are mutually exclusive")
	}
	if strings.TrimSpace(definition.OperationID) == "" {
		return workflowOperationContract{}, fmt.Errorf("runtime-model contract operation ID is required")
	}

	var dataSchema map[string]any
	if len(contract.ResponseAlternatives) > 0 {
		alternatives := make([]any, 0, len(contract.ResponseAlternatives))
		for _, model := range contract.ResponseAlternatives {
			schema, err := builder.schemaForModel(model, modelSchemaOutput, false)
			if err != nil {
				return workflowOperationContract{}, fmt.Errorf("response alternative: %w", err)
			}
			alternatives = append(alternatives, schema)
		}
		dataSchema = map[string]any{"oneOf": alternatives}
	} else {
		var err error
		dataSchema, err = builder.schemaForModel(contract.Response, modelSchemaOutput, false)
		if err != nil {
			return workflowOperationContract{}, fmt.Errorf("response model: %w", err)
		}
	}
	responseName := definition.OperationID + "_ResponseEnvelope"
	if err := builder.addSchema(responseName, successEnvelopeWithData(dataSchema), modelSchemaKey{}); err != nil {
		return workflowOperationContract{}, err
	}

	result := workflowOperationContract{
		PathParameters: automaticPathParameterSchemas(definition.Path),
		ParameterRules: append([]string(nil), contract.ParameterRules...),
		ResponseSchema: responseName,
	}
	if modelType(contract.Query) != nil {
		parameters, queryErr := builder.queryParameters(contract.Query)
		if queryErr != nil {
			return workflowOperationContract{}, fmt.Errorf("query model: %w", queryErr)
		}
		result.QueryParameters = parameters
	}
	if modelType(contract.Request) != nil {
		requestSchema, requestErr := builder.schemaForModel(contract.Request, modelSchemaInput, true)
		if requestErr != nil {
			return workflowOperationContract{}, fmt.Errorf("request model: %w", requestErr)
		}
		requestName := definition.OperationID + "_Request"
		if err := builder.addSchema(requestName, requestSchema, modelSchemaKey{}); err != nil {
			return workflowOperationContract{}, err
		}
		result.RequestBody = &workflowRequestBody{
			Schema:    requestName,
			Required:  !contract.RequestOptional,
			MediaType: normalizedContractMediaType(contract.RequestContentType),
		}
	}
	return result, nil
}

func automaticPathParameterSchemas(path string) map[string]map[string]any {
	result := make(map[string]map[string]any)
	for _, segment := range strings.Split(path, "/") {
		if !strings.HasPrefix(segment, ":") {
			continue
		}
		name := strings.TrimPrefix(segment, ":")
		if name == "id" {
			result[name] = positiveIntegerSchema()
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizedContractMediaType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "application/json"
	}
	return value
}

func modelType(model any) reflect.Type {
	if model == nil {
		return nil
	}
	return reflect.TypeOf(model)
}

func (builder *modelSchemaBuilder) schemaForModel(model any, mode modelSchemaMode, inlineRoot bool) (map[string]any, error) {
	typeOf := modelType(model)
	if typeOf == nil {
		return nil, fmt.Errorf("model is nil")
	}
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	return builder.schemaForType(typeOf, mode, inlineRoot)
}

func (builder *modelSchemaBuilder) schemaForType(typeOf reflect.Type, mode modelSchemaMode, inline bool) (map[string]any, error) {
	if typeOf == nil {
		return nil, fmt.Errorf("type is nil")
	}
	if isJSONRawMessage(typeOf) {
		return map[string]any{}, nil
	}
	if typeOf == reflect.TypeOf(adminroute.EmptyListData{}) {
		return map[string]any{"type": "array", "maxItems": 0, "items": map[string]any{}}, nil
	}
	if isTimeType(typeOf) {
		return map[string]any{"type": "string", "format": "date-time"}, nil
	}
	if typeOf.Kind() == reflect.Pointer {
		schema, err := builder.schemaForType(typeOf.Elem(), mode, false)
		if err != nil {
			return nil, err
		}
		return nullableSchema(schema), nil
	}
	if typeOf.Kind() == reflect.Struct && typeOf.Name() != "" && !inline {
		return builder.componentReference(typeOf, mode)
	}

	switch typeOf.Kind() {
	case reflect.Struct:
		return builder.structSchema(typeOf, mode)
	case reflect.Slice, reflect.Array:
		if typeOf.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string", "format": "byte"}, nil
		}
		item, err := builder.schemaForType(typeOf.Elem(), mode, false)
		if err != nil {
			return nil, err
		}
		return arraySchema(item), nil
	case reflect.Map:
		if typeOf.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map key %s is not a string", typeOf.Key())
		}
		value, err := builder.schemaForType(typeOf.Elem(), mode, false)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "object", "additionalProperties": value}, nil
	case reflect.Interface:
		return map[string]any{}, nil
	case reflect.String:
		return stringSchema(), nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	default:
		return nil, fmt.Errorf("unsupported model type %s", typeOf)
	}
}

func (builder *modelSchemaBuilder) componentReference(typeOf reflect.Type, mode modelSchemaMode) (map[string]any, error) {
	key := modelSchemaKey{Type: typeOf, Mode: mode}
	name, exists := builder.components[key]
	if !exists {
		name = modelComponentName(typeOf, mode)
		if owner, collision := builder.owners[name]; collision && owner != key {
			return nil, fmt.Errorf("model component %q collides between %s and %s", name, owner.Type, typeOf)
		}
		builder.components[key] = name
		builder.owners[name] = key
	}
	if _, exists := builder.schemas[name]; !exists && !builder.building[key] {
		builder.building[key] = true
		schema, err := builder.structSchema(typeOf, mode)
		if err != nil {
			delete(builder.building, key)
			return nil, err
		}
		builder.schemas[name] = schema
		delete(builder.building, key)
	}
	return schemaReference(name), nil
}

func modelComponentName(typeOf reflect.Type, mode modelSchemaMode) string {
	identity := strings.TrimPrefix(typeOf.PkgPath(), "admin_back_go/") + "/" + typeOf.Name()
	var builder strings.Builder
	builder.WriteString("Go_")
	lastUnderscore := false
	for _, character := range identity {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_") + "_" + string(mode)
}

func (builder *modelSchemaBuilder) structSchema(typeOf reflect.Type, mode modelSchemaMode) (map[string]any, error) {
	properties := make(map[string]any)
	required := make([]string, 0, typeOf.NumField())
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		name, omitEmpty, skip := modelJSONField(field)
		if skip {
			continue
		}
		if field.Anonymous && name == "" {
			embeddedType := field.Type
			for embeddedType.Kind() == reflect.Pointer {
				embeddedType = embeddedType.Elem()
			}
			if embeddedType.Kind() != reflect.Struct {
				return nil, fmt.Errorf("anonymous field %s is not a struct", field.Name)
			}
			embedded, err := builder.structSchema(embeddedType, mode)
			if err != nil {
				return nil, err
			}
			for propertyName, property := range embedded["properties"].(map[string]any) {
				if _, duplicate := properties[propertyName]; duplicate {
					return nil, fmt.Errorf("duplicate embedded JSON field %q", propertyName)
				}
				properties[propertyName] = property
			}
			if embeddedRequired, ok := embedded["required"].([]string); ok {
				required = append(required, embeddedRequired...)
			}
			continue
		}

		property, err := builder.schemaForType(field.Type, mode, false)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name, err)
		}
		validation := modelValidationTokens(field)
		if err := applyModelValidation(property, field.Type, validation); err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name, err)
		}
		properties[name] = property
		if modelFieldRequired(mode, field, omitEmpty, validation) {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	return closedObjectSchema(required, properties), nil
}

func modelJSONField(field reflect.StructField) (name string, omitEmpty bool, skip bool) {
	tag, tagged := field.Tag.Lookup("json")
	if tagged {
		parts := strings.Split(tag, ",")
		if parts[0] == "-" {
			return "", false, true
		}
		name = parts[0]
		for _, option := range parts[1:] {
			if option == "omitempty" {
				omitEmpty = true
			}
		}
	}
	if name == "" && !field.Anonymous {
		name = field.Name
	}
	return name, omitEmpty, false
}

func modelValidationTokens(field reflect.StructField) []string {
	value := field.Tag.Get("binding")
	if value == "" {
		value = field.Tag.Get("validate")
	}
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

func modelFieldRequired(mode modelSchemaMode, field reflect.StructField, omitEmpty bool, validation []string) bool {
	if mode == modelSchemaOutput {
		return !omitEmpty
	}
	for _, token := range validation {
		if token == "required" {
			return true
		}
	}
	return false
}

func (builder *modelSchemaBuilder) queryParameters(model any) ([]map[string]any, error) {
	typeOf := modelType(model)
	for typeOf != nil && typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	if typeOf == nil || typeOf.Kind() != reflect.Struct {
		return nil, fmt.Errorf("query model must be a struct")
	}
	parameters := make([]map[string]any, 0, typeOf.NumField())
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		if field.PkgPath != "" {
			continue
		}
		form := strings.Split(field.Tag.Get("form"), ",")[0]
		if form == "" || form == "-" {
			continue
		}
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		schema, err := builder.schemaForType(fieldType, modelSchemaInput, false)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name, err)
		}
		validation := modelValidationTokens(field)
		if err := applyModelValidation(schema, fieldType, validation); err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name, err)
		}
		required := false
		for _, token := range validation {
			if token == "required" {
				required = true
				break
			}
		}
		parameters = append(parameters, queryParameter(form, required, schema, ""))
	}
	sort.Slice(parameters, func(left int, right int) bool {
		return parameters[left]["name"].(string) < parameters[right]["name"].(string)
	})
	return parameters, nil
}

func applyModelValidation(schema map[string]any, typeOf reflect.Type, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	diveIndex := -1
	for index, token := range tokens {
		if token == "dive" {
			diveIndex = index
			break
		}
	}
	outer := tokens
	if diveIndex >= 0 {
		outer = tokens[:diveIndex]
	}
	if err := applyModelScalarValidation(schema, typeOf, outer); err != nil {
		return err
	}
	if diveIndex < 0 {
		return nil
	}
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	if typeOf.Kind() != reflect.Slice && typeOf.Kind() != reflect.Array {
		return fmt.Errorf("dive validation requires a slice or array")
	}
	items, ok := schema["items"].(map[string]any)
	if !ok {
		return fmt.Errorf("array schema has no item schema")
	}
	return applyModelScalarValidation(items, typeOf.Elem(), tokens[diveIndex+1:])
}

func applyModelScalarValidation(schema map[string]any, typeOf reflect.Type, tokens []string) error {
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	for _, token := range tokens {
		if token == "" || token == "required" || token == "omitempty" || strings.Contains(token, "_if=") {
			continue
		}
		name, value, hasValue := strings.Cut(token, "=")
		switch name {
		case "min", "max", "len", "gt", "gte", "lt", "lte":
			if !hasValue {
				continue
			}
			number, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("invalid %s validation %q", name, value)
			}
			applyModelBound(schema, typeOf.Kind(), name, number)
		case "oneof":
			if !hasValue {
				continue
			}
			values, err := modelEnumValues(typeOf.Kind(), strings.Fields(value))
			if err != nil {
				return err
			}
			schema["enum"] = values
		case "numeric":
			if typeOf.Kind() == reflect.String {
				schema["pattern"] = `^\d+$`
			}
		case "email":
			schema["format"] = "email"
		case "url":
			schema["format"] = "uri"
		case "platform_code":
			schema["pattern"] = `^[a-z][a-z0-9_]{1,48}$`
		default:
			if values, exists := modelValidationEnums[name]; exists {
				schema["enum"] = append([]any(nil), values...)
			}
		}
	}
	return nil
}

func applyModelBound(schema map[string]any, kind reflect.Kind, name string, value float64) {
	switch kind {
	case reflect.String:
		applyLengthBound(schema, "minLength", "maxLength", name, value)
	case reflect.Slice, reflect.Array:
		applyLengthBound(schema, "minItems", "maxItems", name, value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		minimum, maximum := value, value
		if name == "gt" {
			minimum++
		}
		if name == "lt" {
			maximum--
		}
		switch name {
		case "min", "gte", "gt":
			schema["minimum"] = int64(minimum)
		case "max", "lte", "lt":
			schema["maximum"] = int64(maximum)
		case "len":
			schema["minimum"] = int64(value)
			schema["maximum"] = int64(value)
		}
	case reflect.Float32, reflect.Float64:
		switch name {
		case "min", "gte", "gt":
			schema["minimum"] = value
		case "max", "lte", "lt":
			schema["maximum"] = value
		case "len":
			schema["minimum"] = value
			schema["maximum"] = value
		}
	}
}

func applyLengthBound(schema map[string]any, minimumKey string, maximumKey string, name string, value float64) {
	integer := int(value)
	switch name {
	case "min", "gte", "gt":
		if name == "gt" {
			integer++
		}
		schema[minimumKey] = integer
	case "max", "lte", "lt":
		if name == "lt" {
			integer--
		}
		schema[maximumKey] = integer
	case "len":
		schema[minimumKey] = integer
		schema[maximumKey] = integer
	}
}

func modelEnumValues(kind reflect.Kind, values []string) ([]any, error) {
	result := make([]any, 0, len(values))
	for _, value := range values {
		switch kind {
		case reflect.String:
			result = append(result, value)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid integer enum value %q", value)
			}
			result = append(result, int(parsed))
		case reflect.Float32, reflect.Float64:
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid number enum value %q", value)
			}
			result = append(result, parsed)
		default:
			return nil, fmt.Errorf("oneof is unsupported for %s", kind)
		}
	}
	return result, nil
}

func (builder *modelSchemaBuilder) addSchema(name string, schema map[string]any, owner modelSchemaKey) error {
	if _, duplicate := builder.schemas[name]; duplicate {
		return fmt.Errorf("duplicate OpenAPI schema %s", name)
	}
	if existingOwner, duplicate := builder.owners[name]; duplicate && existingOwner != owner {
		return fmt.Errorf("duplicate model schema %s", name)
	}
	builder.schemas[name] = schema
	return nil
}

func isTimeType(typeOf reflect.Type) bool {
	return typeOf.PkgPath() == "time" && typeOf.Name() == "Time"
}

func isJSONRawMessage(typeOf reflect.Type) bool {
	return typeOf == reflect.TypeOf(json.RawMessage{})
}

var modelValidationEnums = map[string][]any{
	"auth_platform_login_type":   {"email", "phone", "password"},
	"captcha_type":               {"slide"},
	"client_platform":            {"windows-x86_64", "darwin-x86_64"},
	"common_status":              {1, 2},
	"common_yes_no":              {1, 2},
	"log_level":                  {"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"},
	"notification_level":         {1, 2},
	"notification_target_type":   {1, 2, 3},
	"notification_task_platform": {"all", "admin", "app"},
	"notification_task_status":   {1, 2, 3, 4},
	"notification_type":          {1, 2, 3, 4},
	"payment_method":             {"web", "h5"},
	"payment_provider":           {"alipay"},
	"permission_type":            {1, 2, 3},
	"platform_scope":             {"admin", "app", "canvas"},
	"system_setting_value_type":  {1, 2, 3, 4},
	"upload_driver":              {"cos"},
	"upload_file_ext":            stringValues(enum.UploadFileExts),
	"upload_folder":              stringValues(enum.UploadFolders),
	"upload_image_ext":           stringValues(enum.UploadImageExts),
	"user_sex":                   {0, 1, 2},
	"user_verify_type":           {"password", "code"},
	"verify_code_scene":          {"login", "forget", "bind_phone", "bind_email", "change_password"},
}

func stringValues(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}
