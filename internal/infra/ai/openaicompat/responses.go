package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"sort"
	"strings"

	infraai "admin_back_go/internal/infra/ai"
)

type responsesRequest struct {
	Model           string          `json:"model"`
	Instructions    string          `json:"instructions,omitempty"`
	Input           []any           `json:"input"`
	Include         []string        `json:"include,omitempty"`
	Stream          bool            `json:"stream"`
	Store           bool            `json:"store"`
	Tools           []responsesTool `json:"tools,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	MaxOutputTokens *int            `json:"max_output_tokens,omitempty"`
}

type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
	Strict      bool           `json:"strict"`
}

type responsesMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type responsesFunctionCall struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type responsesFunctionCallOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

func normalizeAPIProtocol(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return infraai.APIProtocolChatCompletions
	}
	return value
}

func prepareResponsesRequest(input infraai.ChatInput, model string, messages []chatMessage) (responsesRequest, error) {
	items, instructions, err := responsesInput(messages)
	if err != nil {
		return responsesRequest{}, err
	}
	items, err = applyResponsesContinuation(items, input.Continuation)
	if err != nil {
		return responsesRequest{}, err
	}
	request := responsesRequest{
		Model: model, Instructions: instructions, Input: items,
		Include: []string{"reasoning.encrypted_content"}, Stream: true, Store: false,
		Tools: responsesTools(input.Tools),
	}
	request.Temperature = input.Temperature
	if input.EffectiveMaxOutputTokens < 0 {
		return responsesRequest{}, fmt.Errorf("%w: effective max output tokens must not be negative", infraai.ErrInvalidConfig)
	}
	if input.EffectiveMaxOutputTokens > 0 {
		request.MaxOutputTokens = &input.EffectiveMaxOutputTokens
	}
	return request, nil
}

func applyResponsesContinuation(items []any, continuation *infraai.ChatContinuation) ([]any, error) {
	if continuation == nil {
		return items, nil
	}
	if strings.TrimSpace(continuation.Protocol) != infraai.APIProtocolResponses {
		return nil, fmt.Errorf("%w: Responses continuation protocol is invalid", infraai.ErrInvalidConfig)
	}
	if len(continuation.Items) == 0 || len(continuation.Items) > 1<<20 {
		return nil, fmt.Errorf("%w: Responses continuation items are invalid", infraai.ErrInvalidConfig)
	}
	var continuationItems []json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(continuation.Items))
	if err := decoder.Decode(&continuationItems); err != nil || len(continuationItems) == 0 {
		return nil, fmt.Errorf("%w: Responses continuation items are invalid", infraai.ErrInvalidConfig)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return nil, fmt.Errorf("%w: Responses continuation items are invalid", infraai.ErrInvalidConfig)
	}
	callIDs := make(map[string]struct{})
	for _, raw := range continuationItems {
		duplicate, duplicateErr := hasDuplicateJSONKey(raw)
		if duplicateErr != nil || duplicate {
			return nil, fmt.Errorf("%w: Responses continuation item is invalid", infraai.ErrInvalidConfig)
		}
		var item struct {
			Type   string `json:"type"`
			CallID string `json:"call_id"`
		}
		if json.Unmarshal(raw, &item) != nil || strings.TrimSpace(item.Type) == "" {
			return nil, fmt.Errorf("%w: Responses continuation item is invalid", infraai.ErrInvalidConfig)
		}
		if item.Type == "function_call" {
			callID := strings.TrimSpace(item.CallID)
			if callID == "" {
				return nil, fmt.Errorf("%w: Responses continuation function call is invalid", infraai.ErrInvalidConfig)
			}
			callIDs[callID] = struct{}{}
		}
	}
	if len(callIDs) == 0 {
		return nil, fmt.Errorf("%w: Responses continuation has no function call", infraai.ErrInvalidConfig)
	}
	normal := make([]any, 0, len(items)+len(continuationItems))
	outputs := make([]any, 0)
	for _, item := range items {
		kind, err := responsesItemType(item)
		if err != nil {
			return nil, err
		}
		switch kind {
		case "function_call":
			continue
		case "function_call_output":
			encoded, encodeErr := json.Marshal(item)
			if encodeErr != nil {
				return nil, fmt.Errorf("encode Responses function output: %w", encodeErr)
			}
			var output responsesFunctionCallOutput
			if json.Unmarshal(encoded, &output) != nil {
				return nil, fmt.Errorf("%w: Responses function output is invalid", infraai.ErrInvalidConfig)
			}
			if _, ok := callIDs[strings.TrimSpace(output.CallID)]; !ok {
				return nil, fmt.Errorf("%w: Responses function output does not match continuation", infraai.ErrInvalidConfig)
			}
			outputs = append(outputs, item)
		default:
			normal = append(normal, item)
		}
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("%w: Responses continuation requires a function output", infraai.ErrInvalidConfig)
	}
	for _, item := range continuationItems {
		normal = append(normal, item)
	}
	normal = append(normal, outputs...)
	return normal, nil
}

func responsesItemType(item any) (string, error) {
	encoded, err := json.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("encode Responses input item: %w", err)
	}
	var value struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(encoded, &value); err != nil {
		return "", fmt.Errorf("decode Responses input item: %w", err)
	}
	return strings.TrimSpace(value.Type), nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON content")
		}
		return err
	}
	return nil
}

func responsesInput(messages []chatMessage) ([]any, string, error) {
	items := make([]any, 0, len(messages))
	instructions := ""
	for _, message := range messages {
		switch message.Role {
		case "system":
			text, ok := message.Content.(string)
			if !ok {
				return nil, "", fmt.Errorf("%w: Responses instructions must be text", infraai.ErrInvalidConfig)
			}
			if strings.TrimSpace(text) != "" {
				if instructions != "" {
					instructions += "\n\n"
				}
				instructions += strings.TrimSpace(text)
			}
		case "tool":
			callID := strings.TrimSpace(message.ToolCallID)
			if callID == "" {
				return nil, "", fmt.Errorf("%w: Responses function output call ID is required", infraai.ErrInvalidConfig)
			}
			output, _ := message.Content.(string)
			items = append(items, responsesFunctionCallOutput{Type: "function_call_output", CallID: callID, Output: output})
		default:
			if len(message.ToolCalls) > 0 {
				for _, call := range message.ToolCalls {
					items = append(items, responsesFunctionCall{
						Type: "function_call", CallID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments,
					})
				}
				continue
			}
			content, err := responsesMessageContent(message.Role, message.Content)
			if err != nil {
				return nil, "", err
			}
			items = append(items, responsesMessage{Role: message.Role, Content: content})
		}
	}
	return items, instructions, nil
}

func responsesMessageContent(role string, content any) (any, error) {
	if text, ok := content.(string); ok {
		if role == "assistant" {
			return text, nil
		}
		return []any{map[string]any{"type": "input_text", "text": text}}, nil
	}
	parts, ok := content.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: Responses message content is invalid", infraai.ErrInvalidConfig)
	}
	converted := make([]any, 0, len(parts))
	for _, rawPart := range parts {
		encoded, err := json.Marshal(rawPart)
		if err != nil {
			return nil, fmt.Errorf("encode Responses content part: %w", err)
		}
		var part map[string]any
		if err := json.Unmarshal(encoded, &part); err != nil {
			return nil, fmt.Errorf("decode Responses content part: %w", err)
		}
		switch strings.TrimSpace(stringFromAny(part["type"])) {
		case "text":
			converted = append(converted, map[string]any{"type": "input_text", "text": stringFromAny(part["text"])})
		case "image_url":
			image, ok := part["image_url"].(map[string]any)
			if !ok || stringFromAny(image["url"]) == "" {
				return nil, fmt.Errorf("%w: Responses image URL is invalid", infraai.ErrInvalidConfig)
			}
			converted = append(converted, map[string]any{"type": "input_image", "image_url": stringFromAny(image["url"])})
		case "file_ref":
			ref := stringFromAny(part["ref"])
			if ref == "" {
				return nil, fmt.Errorf("%w: Responses file ref is invalid", infraai.ErrInvalidConfig)
			}
			converted = append(converted, struct {
				Type string `json:"type"`
				Ref  string `json:"ref"`
			}{Type: "file_ref", Ref: ref})
		default:
			return nil, fmt.Errorf("%w: unsupported Responses content part", infraai.ErrInvalidConfig)
		}
	}
	return converted, nil
}

func responsesTools(tools []infraai.ToolDefinition) []responsesTool {
	out := make([]responsesTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		parameters := tool.Parameters
		if parameters == nil {
			parameters = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
		}
		out = append(out, responsesTool{
			Type: "function", Name: name, Description: strings.TrimSpace(tool.Description), Parameters: parameters, Strict: false,
		})
	}
	return out
}

type responsesStreamEvent struct {
	Type        string            `json:"type"`
	Delta       string            `json:"delta"`
	Arguments   string            `json:"arguments"`
	ItemID      string            `json:"item_id"`
	OutputIndex int               `json:"output_index"`
	Item        json.RawMessage   `json:"item"`
	Response    responsesResponse `json:"response"`
	Error       *responsesError   `json:"error"`
	Code        string            `json:"code"`
	Message     string            `json:"message"`
}

type responsesResponse struct {
	ID                string                      `json:"id"`
	Status            string                      `json:"status"`
	Output            []json.RawMessage           `json:"output"`
	Usage             *responsesUsage             `json:"usage"`
	Error             *responsesError             `json:"error"`
	IncompleteDetails *responsesIncompleteDetails `json:"incomplete_details"`
}

type responsesOutputItem struct {
	ID        string                   `json:"id"`
	Type      string                   `json:"type"`
	CallID    string                   `json:"call_id"`
	Name      string                   `json:"name"`
	Arguments string                   `json:"arguments"`
	Content   []responsesOutputContent `json:"content"`
}

type responsesOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesIncompleteDetails struct {
	Reason string `json:"reason"`
}

type responsesError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type responsesUsage struct {
	InputTokens  *int                   `json:"input_tokens"`
	OutputTokens *int                   `json:"output_tokens"`
	TotalTokens  *int                   `json:"total_tokens"`
	InputDetails *responsesInputDetails `json:"input_tokens_details"`
	hasDuplicate bool
}

type responsesInputDetails struct {
	CachedTokens     *int `json:"cached_tokens"`
	CacheWriteTokens *int `json:"cache_write_tokens"`
}

func (usage *responsesUsage) UnmarshalJSON(data []byte) error {
	duplicate, err := hasDuplicateJSONKey(data)
	if err != nil {
		return err
	}
	type plainUsage responsesUsage
	if err := json.Unmarshal(data, (*plainUsage)(usage)); err != nil {
		return err
	}
	usage.hasDuplicate = duplicate
	return nil
}

type responsesToolCallState struct {
	byItemID      map[string]int
	byOutputIndex map[int]int
	outputItems   map[int]json.RawMessage
}

type responsesTerminalStreamError struct{ cause error }

func (err *responsesTerminalStreamError) Error() string {
	if err == nil || err.cause == nil {
		return "OpenAI Responses request failed"
	}
	return err.cause.Error()
}

func (err *responsesTerminalStreamError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func isResponsesTerminalStreamError(err error) bool {
	var terminal *responsesTerminalStreamError
	return errors.As(err, &terminal)
}

func (c *Client) readResponsesStream(ctx context.Context, body io.Reader, sink infraai.EventSink, touch func()) (*infraai.ChatResult, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var answer strings.Builder
	result := &infraai.ChatResult{}
	streamDigest := sha256.New()
	hasStreamData := false
	deliveryEnabled := sink != nil
	toolState := responsesToolCallState{
		byItemID: make(map[string]int), byOutputIndex: make(map[int]int), outputItems: make(map[int]json.RawMessage),
	}
	for scanner.Scan() {
		if touch != nil {
			touch()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		_, _ = io.WriteString(streamDigest, data+"\n")
		hasStreamData = true
		if data == "[DONE]" {
			return nil, fmt.Errorf("OpenAI Responses stream ended without a terminal response event")
		}
		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, fmt.Errorf("decode OpenAI Responses stream event: %w", err)
		}
		switch event.Type {
		case "response.created":
			if id := strings.TrimSpace(event.Response.ID); id != "" {
				result.EngineTaskID = id
			}
		case "response.output_text.delta":
			if event.Delta == "" {
				continue
			}
			answer.WriteString(event.Delta)
			result.Answer = strings.TrimSpace(answer.String())
			if deliveryEnabled {
				if err := sink.Emit(ctx, infraai.Event{Type: "delta", DeltaText: event.Delta, Payload: map[string]any{"delta": event.Delta}}); err != nil {
					if infraai.IsFatalEventSinkError(err) {
						return nil, err
					}
					deliveryEnabled = false
				}
			}
		case "response.output_item.added", "response.output_item.done":
			item, itemErr := decodeResponsesOutputItem(event.Item)
			if itemErr != nil {
				return nil, itemErr
			}
			if event.Type == "response.output_item.done" {
				toolState.outputItems[event.OutputIndex] = append(json.RawMessage(nil), event.Item...)
			}
			if item.Type != "function_call" {
				continue
			}
			index, mergeErr := mergeResponsesToolItem(result, &toolState, event.OutputIndex, item, event.Type == "response.output_item.done")
			if mergeErr != nil {
				return nil, mergeErr
			}
			if deliveryEnabled && event.Type == "response.output_item.added" {
				call := result.ToolCalls[index]
				if err := emitResponsesToolDelta(ctx, sink, call.ID, call.Name, call.Arguments); err != nil {
					if infraai.IsFatalEventSinkError(err) {
						return nil, err
					}
					deliveryEnabled = false
				}
			}
		case "response.function_call_arguments.delta":
			index, ok := toolState.byItemID[event.ItemID]
			if !ok {
				index, ok = toolState.byOutputIndex[event.OutputIndex]
			}
			if !ok || index < 0 || index >= len(result.ToolCalls) {
				return nil, fmt.Errorf("OpenAI Responses function arguments arrived before the function call")
			}
			result.ToolCalls[index].Arguments += event.Delta
			if deliveryEnabled {
				call := result.ToolCalls[index]
				if err := emitResponsesToolDelta(ctx, sink, call.ID, "", event.Delta); err != nil {
					if infraai.IsFatalEventSinkError(err) {
						return nil, err
					}
					deliveryEnabled = false
				}
			}
		case "response.function_call_arguments.done":
			index, ok := responsesToolCallIndex(&toolState, event.ItemID, event.OutputIndex)
			if !ok || index < 0 || index >= len(result.ToolCalls) {
				return nil, fmt.Errorf("OpenAI Responses function arguments completed before the function call")
			}
			arguments := event.Arguments
			if current := result.ToolCalls[index].Arguments; current != "" && arguments != "" && current != arguments {
				return nil, fmt.Errorf("OpenAI Responses function arguments conflict with streamed deltas")
			}
			if arguments != "" {
				result.ToolCalls[index].Arguments = arguments
			}
		case "response.completed":
			applyResponsesUsage(result, event.Response.Usage, []byte(data))
			if id := strings.TrimSpace(event.Response.ID); id != "" {
				result.EngineTaskID = id
			}
			if strings.TrimSpace(event.Response.Status) != "completed" {
				finalizeResponsesStreamResult(result, streamDigest, hasStreamData)
				return result, &responsesTerminalStreamError{cause: fmt.Errorf("%w: OpenAI Responses completed event has status %q", infraai.ErrUpstreamFailed, event.Response.Status)}
			}
			if mergeErr := mergeResponsesFinalOutput(result, &toolState, event.Response.Output, answer.String()); mergeErr != nil {
				finalizeResponsesStreamResult(result, streamDigest, hasStreamData)
				return result, &responsesTerminalStreamError{cause: mergeErr}
			}
			finalizeResponsesStreamResult(result, streamDigest, hasStreamData)
			return result, nil
		case "response.incomplete":
			applyResponsesUsage(result, event.Response.Usage, []byte(data))
			if id := strings.TrimSpace(event.Response.ID); id != "" {
				result.EngineTaskID = id
			}
			if mergeErr := mergeResponsesFinalOutput(result, &toolState, event.Response.Output, answer.String()); mergeErr != nil {
				finalizeResponsesStreamResult(result, streamDigest, hasStreamData)
				return result, &responsesTerminalStreamError{cause: mergeErr}
			}
			finalizeResponsesStreamResult(result, streamDigest, hasStreamData)
			return result, nil
		case "response.failed":
			applyResponsesUsage(result, event.Response.Usage, []byte(data))
			if id := strings.TrimSpace(event.Response.ID); id != "" {
				result.EngineTaskID = id
			}
			c.logResponsesStreamFailure(ctx, event.Type, event.Response.Error)
			finalizeResponsesStreamResult(result, streamDigest, hasStreamData)
			return result, &responsesTerminalStreamError{cause: responsesStreamError(event.Response.Error)}
		case "error":
			streamError := event.Error
			if streamError == nil {
				streamError = &responsesError{Code: event.Code, Message: event.Message}
			}
			c.logResponsesStreamFailure(ctx, event.Type, streamError)
			finalizeResponsesStreamResult(result, streamDigest, hasStreamData)
			return result, &responsesTerminalStreamError{cause: responsesStreamError(streamError)}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read OpenAI Responses stream: %w", err)
	}
	return nil, fmt.Errorf("OpenAI Responses stream ended without a terminal response event")
}

func (c *Client) logResponsesStreamFailure(ctx context.Context, eventType string, value *responsesError) {
	if c == nil || c.logger == nil {
		return
	}
	code, message := "", ""
	if value != nil {
		code = sanitizeBody([]byte(value.Code), c.apiKey)
		message = sanitizeBody([]byte(value.Message), c.apiKey)
	}
	c.logger.WarnContext(ctx, "AI provider stream failed",
		"event_type", strings.TrimSpace(eventType),
		"error_code", code,
		"message", message,
	)
}

func decodeResponsesOutputItem(raw json.RawMessage) (responsesOutputItem, error) {
	if len(raw) == 0 {
		return responsesOutputItem{}, fmt.Errorf("OpenAI Responses output item is missing")
	}
	duplicate, err := hasDuplicateJSONKey(raw)
	if err != nil || duplicate {
		return responsesOutputItem{}, fmt.Errorf("OpenAI Responses output item is invalid")
	}
	var item responsesOutputItem
	if err := json.Unmarshal(raw, &item); err != nil || strings.TrimSpace(item.Type) == "" {
		return responsesOutputItem{}, fmt.Errorf("OpenAI Responses output item is invalid")
	}
	return item, nil
}

func responsesToolCallIndex(state *responsesToolCallState, itemID string, outputIndex int) (int, bool) {
	if state == nil {
		return 0, false
	}
	if id := strings.TrimSpace(itemID); id != "" {
		if index, ok := state.byItemID[id]; ok {
			return index, true
		}
	}
	index, ok := state.byOutputIndex[outputIndex]
	return index, ok
}

func mergeResponsesToolItem(result *infraai.ChatResult, state *responsesToolCallState, outputIndex int, item responsesOutputItem, replaceArguments bool) (int, error) {
	if result == nil || state == nil {
		return 0, fmt.Errorf("OpenAI Responses function call state is missing")
	}
	itemID := strings.TrimSpace(item.ID)
	callID := strings.TrimSpace(item.CallID)
	name := strings.TrimSpace(item.Name)
	if callID == "" || name == "" {
		return 0, fmt.Errorf("OpenAI Responses function call identity is incomplete")
	}
	index, exists := responsesToolCallIndex(state, itemID, outputIndex)
	if exists {
		if index < 0 || index >= len(result.ToolCalls) {
			return 0, fmt.Errorf("OpenAI Responses function call index is invalid")
		}
		call := &result.ToolCalls[index]
		if current := strings.TrimSpace(call.ID); current != "" && current != callID {
			return 0, fmt.Errorf("OpenAI Responses function call ID changed during the stream")
		}
		if current := strings.TrimSpace(call.Name); current != "" && current != name {
			return 0, fmt.Errorf("OpenAI Responses function name changed during the stream")
		}
		if replaceArguments && call.Arguments != "" && item.Arguments != "" && call.Arguments != item.Arguments {
			return 0, fmt.Errorf("OpenAI Responses function arguments conflict with streamed deltas")
		}
		call.ID = callID
		call.Name = name
		if replaceArguments && item.Arguments != "" {
			call.Arguments = item.Arguments
		}
		if itemID != "" {
			state.byItemID[itemID] = index
		}
		state.byOutputIndex[outputIndex] = index
		return index, nil
	}
	index = len(result.ToolCalls)
	result.ToolCalls = append(result.ToolCalls, infraai.ToolCall{ID: callID, Name: name, Arguments: item.Arguments})
	if itemID != "" {
		state.byItemID[itemID] = index
	}
	state.byOutputIndex[outputIndex] = index
	return index, nil
}

func mergeResponsesFinalOutput(result *infraai.ChatResult, state *responsesToolCallState, output []json.RawMessage, streamedText string) error {
	if len(output) > 0 {
		state.outputItems = make(map[int]json.RawMessage, len(output))
		for index, raw := range output {
			state.outputItems[index] = append(json.RawMessage(nil), raw...)
		}
	}
	var finalText strings.Builder
	for _, index := range sortedResponsesOutputIndexes(state.outputItems) {
		raw := state.outputItems[index]
		item, err := decodeResponsesOutputItem(raw)
		if err != nil {
			return err
		}
		switch item.Type {
		case "function_call":
			if _, err := mergeResponsesToolItem(result, state, index, item, true); err != nil {
				return err
			}
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" {
					finalText.WriteString(content.Text)
				}
			}
		}
	}
	if final := finalText.String(); final != "" {
		if streamedText != "" && streamedText != final {
			return fmt.Errorf("OpenAI Responses final text conflicts with streamed deltas")
		}
		result.Answer = strings.TrimSpace(final)
	}
	if len(result.ToolCalls) == 0 {
		return nil
	}
	for _, call := range result.ToolCalls {
		if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" || !json.Valid([]byte(call.Arguments)) {
			return fmt.Errorf("OpenAI Responses function call is incomplete")
		}
	}
	items := make([]json.RawMessage, 0, len(state.outputItems))
	for _, index := range sortedResponsesOutputIndexes(state.outputItems) {
		items = append(items, state.outputItems[index])
	}
	if len(items) == 0 {
		return fmt.Errorf("OpenAI Responses tool result is missing continuation items")
	}
	encoded, err := json.Marshal(items)
	if err != nil || len(encoded) > 1<<20 {
		return fmt.Errorf("OpenAI Responses continuation items are invalid")
	}
	result.Continuation = &infraai.ChatContinuation{Protocol: infraai.APIProtocolResponses, Items: encoded}
	return nil
}

func sortedResponsesOutputIndexes(items map[int]json.RawMessage) []int {
	indexes := make([]int, 0, len(items))
	for index := range items {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	return indexes
}

func emitResponsesToolDelta(ctx context.Context, sink infraai.EventSink, callID, name, arguments string) error {
	if sink == nil || (strings.TrimSpace(callID) == "" && strings.TrimSpace(name) == "" && arguments == "") {
		return nil
	}
	return sink.Emit(ctx, infraai.Event{Type: "tool_delta", Payload: map[string]any{
		"tool_call_id": strings.TrimSpace(callID), "name": strings.TrimSpace(name), "arguments_delta": arguments,
	}})
}

func applyResponsesUsage(result *infraai.ChatResult, usage *responsesUsage, raw []byte) {
	if result == nil || usage == nil {
		return
	}
	result.PromptTokens = usageInt(usage.InputTokens)
	result.CompletionTokens = usageInt(usage.OutputTokens)
	result.TotalTokens = usageInt(usage.TotalTokens)
	result.Usage = responsesUsageSnapshot(usage)
	result.Usage.RawProviderJSON = append([]byte(nil), raw...)
	result.Usage.ResponseSHA256 = sha256.Sum256(raw)
	result.ResponseSHA256 = result.Usage.ResponseSHA256
	if result.Usage.Complete() {
		result.UsageStatus = infraai.UsageStatusReported
	} else {
		result.UsageStatus = infraai.UsageStatusUnavailable
	}
}

func responsesUsageSnapshot(usage *responsesUsage) infraai.UsageSnapshot {
	if usage == nil || usage.hasDuplicate {
		return infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
	}
	var details *promptTokenDetails
	if usage.InputDetails != nil {
		details = &promptTokenDetails{
			CachedTokens: usage.InputDetails.CachedTokens, CacheCreationInputTokens: usage.InputDetails.CacheWriteTokens,
		}
	}
	return tokenUsageSnapshot(usage.InputTokens, usage.OutputTokens, usage.TotalTokens, details)
}

func responsesStreamError(value *responsesError) error {
	if value == nil {
		return fmt.Errorf("%w: OpenAI Responses request failed", infraai.ErrUpstreamFailed)
	}
	message := strings.TrimSpace(value.Message)
	if message == "" {
		message = "OpenAI Responses request failed"
	}
	if code := strings.TrimSpace(value.Code); code != "" {
		return fmt.Errorf("%w: %s (%s)", infraai.ErrUpstreamFailed, message, code)
	}
	return fmt.Errorf("%w: %s", infraai.ErrUpstreamFailed, message)
}

func finalizeResponsesStreamResult(result *infraai.ChatResult, digest hash.Hash, hasData bool) {
	if result.UsageStatus == "" {
		result.UsageStatus = infraai.UsageStatusUnavailable
		result.Usage = infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable}
	}
	setStreamResponseHash(result, digest, hasData)
}

func preparedRequestAPIProtocol(body []byte) (string, error) {
	schema, err := infraai.DetectPreparedChatSchema(body)
	if err != nil {
		return "", err
	}
	switch schema {
	case infraai.PreparedChatSchemaInlineV1:
		return infraai.APIProtocolChatCompletions, nil
	case infraai.PreparedChatSchemaResponsesInlineV1:
		envelope, parseErr := infraai.ParsePreparedChatInlineEnvelope(body)
		if parseErr != nil {
			return "", parseErr
		}
		return envelope.APIProtocol, nil
	case infraai.PreparedChatSchemaFileManifestV1, infraai.PreparedChatSchemaResponsesFileManifestV1:
		manifest, err := infraai.ParsePreparedChatFileManifest(body)
		if err != nil {
			return "", err
		}
		return manifest.Protocol(), nil
	default:
		return "", fmt.Errorf("%w: unsupported prepared request API protocol schema", infraai.ErrInvalidConfig)
	}
}
