package contextengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	infraai "admin_back_go/internal/infra/ai"
)

// TokenCounter is the single token-bound implementation used by indexing and
// retrieval. Keeping the alias here makes the Turn contract independent of a
// particular provider while still reusing the registered counter.
type TokenCounter = infraai.TokenCounter

type ConversationTurn struct {
	ConversationID    uint64
	UserID            uint64
	AgentID           uint64
	UserMessage       TurnMessage
	ToolGroups        []ToolGroup
	AssistantMessage  TurnMessage
	AssistantDelivery string
	SourceSHA256      [32]byte
}

type TurnMessage struct {
	ID            uint64
	Role          string
	Content       string
	ContentSHA256 [32]byte
	Attachments   []TurnAttachment
}

type TurnAttachment struct {
	Index           uint32
	Type            string
	StorageProvider string
	ObjectKey       string
	ETag            string
	Size            int64
	MIMEType        string
	Name            string
}

type ToolGroup struct {
	CallID    string
	Name      string
	Arguments string
	Result    string
}

type ConversationTurnText struct {
	Text            string
	TokenUpperBound int64
}

type ConversationTurnReader interface {
	NewestComplete(context.Context, uint64, uint64, *uint64) (*ConversationTurn, error)
	CompleteByAnchors(context.Context, uint64, uint64, []uint64) ([]ConversationTurn, error)
}

var (
	errTurnInvalid      = errors.New("invalid conversation turn")
	errTurnTextTooSmall = errors.New("conversation turn fixed envelope does not fit")
)

type canonicalTurn struct {
	Schema            string           `json:"schema"`
	ConversationID    uint64           `json:"conversation_id"`
	UserID            uint64           `json:"user_id"`
	AgentID           uint64           `json:"agent_id"`
	UserMessage       canonicalMessage `json:"user_message"`
	ToolGroups        []canonicalTool  `json:"tool_groups,omitempty"`
	AssistantMessage  canonicalMessage `json:"assistant_message"`
	AssistantDelivery string           `json:"assistant_delivery"`
}

type canonicalMessage struct {
	ID            uint64            `json:"id"`
	Role          string            `json:"role"`
	ContentSHA256 string            `json:"content_sha256"`
	Attachments   []canonicalAttach `json:"attachments,omitempty"`
}

type canonicalAttach struct {
	Index           uint32 `json:"index"`
	Type            string `json:"type"`
	StorageProvider string `json:"storage_provider"`
	ObjectKey       string `json:"object_key"`
	ETag            string `json:"etag"`
	Size            int64  `json:"size"`
	MIMEType        string `json:"mime_type"`
	Name            string `json:"name"`
}

type canonicalTool struct {
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Result    string `json:"result"`
}

func (turn *ConversationTurn) ComputeSourceSHA256() error {
	if turn == nil {
		return errTurnInvalid
	}
	if err := validateTurn(turn); err != nil {
		return err
	}
	canonical := canonicalTurn{
		Schema:            "conversation_turn_v1",
		ConversationID:    turn.ConversationID,
		UserID:            turn.UserID,
		AgentID:           turn.AgentID,
		UserMessage:       canonicalMessageFrom(turn.UserMessage),
		AssistantMessage:  canonicalMessageFrom(turn.AssistantMessage),
		AssistantDelivery: turn.AssistantDelivery,
	}
	canonical.ToolGroups = make([]canonicalTool, len(turn.ToolGroups))
	for i, group := range turn.ToolGroups {
		canonical.ToolGroups[i] = canonicalTool{CallID: group.CallID, Name: group.Name, Arguments: group.Arguments, Result: group.Result}
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return fmt.Errorf("hash conversation turn: %w", err)
	}
	turn.SourceSHA256 = sha256.Sum256(raw)
	return nil
}

func ConversationTurnSourceSHA256(turn ConversationTurn) ([32]byte, error) {
	copy := turn
	if err := copy.ComputeSourceSHA256(); err != nil {
		return [32]byte{}, err
	}
	return copy.SourceSHA256, nil
}

func canonicalMessageFrom(message TurnMessage) canonicalMessage {
	attachments := make([]canonicalAttach, len(message.Attachments))
	for i, attachment := range message.Attachments {
		attachments[i] = canonicalAttach{
			Index: attachment.Index, Type: attachment.Type, StorageProvider: attachment.StorageProvider,
			ObjectKey: attachment.ObjectKey, ETag: attachment.ETag, Size: attachment.Size,
			MIMEType: attachment.MIMEType, Name: attachment.Name,
		}
	}
	hash := message.ContentSHA256
	if hash == ([32]byte{}) {
		hash = sha256.Sum256([]byte(message.Content))
	}
	return canonicalMessage{ID: message.ID, Role: message.Role, ContentSHA256: hex.EncodeToString(hash[:]), Attachments: attachments}
}

func validateTurn(turn *ConversationTurn) error {
	if turn.ConversationID == 0 || turn.UserID == 0 || turn.AgentID == 0 {
		return fmt.Errorf("%w: identity", errTurnInvalid)
	}
	if err := validateMessage(turn.UserMessage, "user"); err != nil {
		return err
	}
	if err := validateMessage(turn.AssistantMessage, "assistant"); err != nil {
		return err
	}
	if turn.AssistantDelivery != "completed" && turn.AssistantDelivery != "stopped" {
		return fmt.Errorf("%w: assistant delivery", errTurnInvalid)
	}
	for i, group := range turn.ToolGroups {
		if strings.TrimSpace(group.CallID) == "" || strings.TrimSpace(group.Name) == "" ||
			!utf8.ValidString(group.Arguments) || !utf8.ValidString(group.Result) {
			return fmt.Errorf("%w: tool group %d", errTurnInvalid, i)
		}
	}
	return nil
}

func validateMessage(message TurnMessage, role string) error {
	if message.ID == 0 || message.Role != role || !utf8.ValidString(message.Content) {
		return fmt.Errorf("%w: %s message", errTurnInvalid, role)
	}
	if message.ContentSHA256 != ([32]byte{}) && message.ContentSHA256 != sha256.Sum256([]byte(message.Content)) {
		return fmt.Errorf("%w: %s content hash", errTurnInvalid, role)
	}
	for i, attachment := range message.Attachments {
		if attachment.Index != uint32(i) || strings.TrimSpace(attachment.Type) == "" ||
			strings.TrimSpace(attachment.StorageProvider) == "" || strings.TrimSpace(attachment.ObjectKey) == "" ||
			strings.TrimSpace(attachment.ETag) == "" || attachment.Size <= 0 ||
			strings.TrimSpace(attachment.MIMEType) == "" || strings.TrimSpace(attachment.Name) == "" {
			return fmt.Errorf("%w: %s attachment %d", errTurnInvalid, role, i)
		}
	}
	return nil
}

// BuildConversationTurnText emits the only bounded text representation used
// by retrieval and indexing. Blocks are appended atomically; tool calls and
// results are always emitted as one pair.
func BuildConversationTurnText(turn ConversationTurn, counter TokenCounter, maxTokens int64) (ConversationTurnText, error) {
	if counter == nil || maxTokens <= 0 {
		return ConversationTurnText{}, errTurnTextTooSmall
	}
	if err := validateTurn(&turn); err != nil {
		return ConversationTurnText{}, err
	}
	var out strings.Builder
	appendLine := func(line string) bool {
		candidate := out.String() + line
		count, err := counter.UpperBoundText(candidate)
		if err != nil || count > maxTokens {
			return false
		}
		out.WriteString(line)
		return true
	}
	boundedLine := func(label, value string, budget int64) (string, error) {
		prefix := label
		if !utf8.ValidString(value) {
			return "", errTurnInvalid
		}
		full := prefix + value + "\n"
		if n, err := counter.UpperBoundText(full); err == nil && n <= budget {
			return full, nil
		}
		base, err := counter.UpperBoundText(prefix + "\n")
		if err != nil || base > budget {
			return "", errTurnTextTooSmall
		}
		low, high, best := 0, len(value), ""
		for low <= high {
			mid := (low + high) / 2
			for mid > 0 && !utf8.ValidString(value[:mid]) {
				mid--
			}
			candidate := prefix + value[:mid] + "\n"
			n, countErr := counter.UpperBoundText(candidate)
			if countErr == nil && n <= budget {
				best, low = candidate, mid+1
			} else {
				high = mid - 1
			}
		}
		return best, nil
	}

	userLine, err := boundedLine("User: ", turn.UserMessage.Content, maxTokens)
	if err != nil || !appendLine(userLine) {
		return ConversationTurnText{}, errTurnTextTooSmall
	}
	stopped := false
	for i, attachment := range turn.UserMessage.Attachments {
		used, countErr := counter.UpperBoundText(out.String())
		if countErr != nil {
			return ConversationTurnText{}, countErr
		}
		remaining := maxTokens - used
		if remaining <= 0 {
			stopped = true
			break
		}
		attachmentFacts := fmt.Sprintf(
			"%s etag=%s size=%d mime=%s name=%s",
			attachment.ObjectKey, attachment.ETag, attachment.Size, attachment.MIMEType, attachment.Name,
		)
		line, lineErr := boundedLine(fmt.Sprintf("Attachment[%d]: type=%s provider=%s object_key=", i, attachment.Type, attachment.StorageProvider), attachmentFacts, remaining)
		if lineErr != nil {
			stopped = true
			break
		}
		if !appendLine(line) {
			stopped = true
			break
		}
	}
	for i, group := range turn.ToolGroups {
		if stopped {
			break
		}
		callPrefix := fmt.Sprintf("Tool[%d] Call: id=%s name=%s arguments=", i, group.CallID, group.Name)
		resultPrefix := fmt.Sprintf("Tool[%d] Result: id=%s result=", i, group.CallID)
		remaining := maxTokens
		used, countErr := counter.UpperBoundText(out.String())
		if countErr != nil {
			return ConversationTurnText{}, countErr
		}
		remaining -= used
		if remaining <= 0 {
			break
		}
		pair, pairErr := boundedToolPair(callPrefix, group.Arguments, resultPrefix, group.Result, remaining, counter)
		if pairErr != nil || !appendLine(pair) {
			stopped = true
			break
		}
	}
	remaining := maxTokens
	used, err := counter.UpperBoundText(out.String())
	if err != nil {
		return ConversationTurnText{}, err
	}
	remaining -= used
	if !stopped && remaining > 0 {
		assistant, lineErr := boundedLine("Assistant[delivery="+turn.AssistantDelivery+"]: ", turn.AssistantMessage.Content, remaining)
		if lineErr == nil {
			_ = appendLine(assistant)
		}
	}
	text := out.String()
	count, err := counter.UpperBoundText(text)
	if err != nil {
		return ConversationTurnText{}, err
	}
	return ConversationTurnText{Text: text, TokenUpperBound: count}, nil
}

func boundedToolPair(callPrefix, arguments, resultPrefix, result string, budget int64, counter TokenCounter) (string, error) {
	base, err := counter.UpperBoundText(callPrefix + "\n" + resultPrefix + "\n")
	if err != nil || base > budget {
		return "", errTurnTextTooSmall
	}
	full := callPrefix + arguments + "\n" + resultPrefix + result + "\n"
	if n, countErr := counter.UpperBoundText(full); countErr == nil && n <= budget {
		return full, nil
	}
	low, high, best := 0, len(arguments)+len(result), ""
	for low <= high {
		mid := (low + high) / 2
		argN := min(len(arguments), (mid+1)/2)
		resN := min(len(result), mid-argN)
		left := mid - argN - resN
		if left > 0 {
			add := min(left, len(arguments)-argN)
			argN += add
			left -= add
		}
		if left > 0 {
			resN += min(left, len(result)-resN)
		}
		for argN > 0 && !utf8.ValidString(arguments[:argN]) {
			argN--
		}
		for resN > 0 && !utf8.ValidString(result[:resN]) {
			resN--
		}
		candidate := callPrefix + arguments[:argN] + "\n" + resultPrefix + result[:resN] + "\n"
		n, countErr := counter.UpperBoundText(candidate)
		if countErr == nil && n <= budget {
			best, low = candidate, mid+1
		} else {
			high = mid - 1
		}
	}
	return best, nil
}
