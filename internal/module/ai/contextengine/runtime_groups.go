package contextengine

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	infraai "admin_back_go/internal/infra/ai"
)

func runtimeCoreGroups(messageID, agentID uint64, systemPrompt string, agentHash [32]byte, content string, attachments []runtimeAttachment, tools []runtimeToolRow, counterID string) ([]PackGroup, error) {
	counter, err := infraai.ResolveTokenCounter(counterID)
	if err != nil {
		return nil, err
	}
	groups := make([]PackGroup, 0, 3+len(tools))
	if strings.TrimSpace(systemPrompt) != "" {
		bound, countErr := counter.UpperBoundText(systemPrompt)
		if countErr != nil {
			return nil, countErr
		}
		groups = append(groups, PackGroup{Required: true, Priority: 1, SourceOrder: 0, StableSourceID: fmt.Sprintf("agent:%d", agentID), Blocks: []PackBlock{{Block: ContextBlock{Kind: BlockSystemInstruction, SourceType: "agent", SourceRef: fmt.Sprintf("agent:%d", agentID), SourceSHA256: agentHash, AtomicGroupKey: fmt.Sprintf("agent:%d", agentID), Required: true, Priority: 1, TokenUpperBound: bound, ContentSnapshot: &systemPrompt, Metadata: emptyBlockMetadata()}}}})
	}
	messageHash := sha256.Sum256([]byte(content))
	messageBlocks := make([]PackBlock, 0, 1+len(attachments))
	if content != "" {
		bound, countErr := counter.UpperBoundText(content)
		if countErr != nil {
			return nil, countErr
		}
		messageBlocks = append(messageBlocks, PackBlock{Block: ContextBlock{Kind: BlockCurrentUserMessage, SourceType: "message", SourceRef: fmt.Sprintf("message:%d", messageID), SourceSHA256: messageHash, AtomicGroupKey: fmt.Sprintf("message:%d", messageID), Required: true, Priority: 1, TokenUpperBound: bound, ContentSnapshot: &content, Metadata: emptyBlockMetadata()}})
	}
	for index, attachment := range attachments {
		hash, hashErr := hashRuntimeFacts(attachment)
		if hashErr != nil {
			return nil, hashErr
		}
		metadata := emptyBlockMetadata()
		metadata.Attachment = &ContextAttachmentV1{Kind: attachment.Kind, URL: attachment.URL, ObjectKey: attachment.ObjectKey, ETag: attachment.ETag, Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Filename}
		messageBlocks = append(messageBlocks, PackBlock{Block: ContextBlock{Kind: BlockCurrentAttachment, SourceType: "attachment", SourceRef: fmt.Sprintf("message:%d/attachment:%d", messageID, index), SourceSHA256: hash, AtomicGroupKey: fmt.Sprintf("message:%d", messageID), Required: true, Priority: 1, TokenUpperBound: 0, Metadata: metadata}})
	}
	if len(messageBlocks) == 0 {
		return nil, ErrInvalidContextPlan
	}
	groups = append(groups, PackGroup{Required: true, Priority: 2, SourceOrder: int64(messageID), StableSourceID: fmt.Sprintf("message:%d", messageID), Blocks: messageBlocks})
	for index, tool := range tools {
		contentJSON := tool.ParametersJSON
		bound, countErr := counter.UpperBoundText(contentJSON)
		if countErr != nil {
			return nil, countErr
		}
		hash, hashErr := hashRuntimeFacts(struct {
			ID                            uint64
			Code, Description, Parameters string
		}{tool.ToolID, tool.Code, tool.Description, tool.ParametersJSON})
		if hashErr != nil {
			return nil, hashErr
		}
		groups = append(groups, PackGroup{Required: true, Priority: 3, SourceOrder: int64(index), StableSourceID: fmt.Sprintf("tool:%d", tool.ToolID), Blocks: []PackBlock{{Block: ContextBlock{Kind: BlockToolDefinition, SourceType: "tool", SourceRef: fmt.Sprintf("tool:%d", tool.ToolID), SourceSHA256: hash, AtomicGroupKey: fmt.Sprintf("tool:%d", tool.ToolID), Required: true, Priority: 3, TokenUpperBound: bound, ContentSnapshot: &contentJSON, Metadata: emptyBlockMetadata()}}}})
	}
	return groups, nil
}

type runtimeAttachment struct {
	Kind      AttachmentKind
	URL       string
	ObjectKey string
	ETag      string
	Size      int64
	MIMEType  string
	Filename  string
}

func runtimeAttachments(raw *string) ([]runtimeAttachment, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	var metadata struct {
		Attachments []struct {
			Type      string `json:"type"`
			URL       string `json:"url"`
			ObjectKey string `json:"object_key"`
			ETag      string `json:"etag"`
			Size      int64  `json:"size"`
			MIMEType  string `json:"mime_type"`
			Name      string `json:"name"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(*raw), &metadata); err != nil {
		return nil, err
	}
	attachments := make([]runtimeAttachment, len(metadata.Attachments))
	for index, item := range metadata.Attachments {
		attachment := runtimeAttachment{Kind: AttachmentKind(strings.TrimSpace(item.Type)), URL: strings.TrimSpace(item.URL),
			ObjectKey: strings.TrimSpace(item.ObjectKey), ETag: strings.TrimSpace(item.ETag), Size: item.Size,
			MIMEType: strings.ToLower(strings.TrimSpace(item.MIMEType)), Filename: strings.TrimSpace(item.Name)}
		if err := (ContextAttachmentV1{Kind: attachment.Kind, URL: attachment.URL, ObjectKey: attachment.ObjectKey, ETag: attachment.ETag,
			Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Filename}).Validate(); err != nil {
			return nil, err
		}
		attachments[index] = attachment
	}
	return attachments, nil
}

func sameToolDefinitions(input, authoritative []infraai.ToolDefinition) bool {
	if len(input) != len(authoritative) {
		return false
	}
	for index := range input {
		left, leftErr := json.Marshal(input[index])
		right, rightErr := json.Marshal(authoritative[index])
		if leftErr != nil || rightErr != nil || string(left) != string(right) {
			return false
		}
	}
	return true
}

func hashRuntimeFacts(value any) ([sha256.Size]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

func bindingSpaceIDs(bindings []runtimeBindingRow) []uint64 {
	ids := make([]uint64, len(bindings))
	for index, binding := range bindings {
		ids[index] = binding.SpaceID
	}
	return ids
}

func optionalUnixMilli(value *time.Time) int64 {
	if value == nil {
		return 0
	}
	return value.UnixMilli()
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func emptyBlockMetadata() ContextBlockMetadataV1 {
	return ContextBlockMetadataV1{Schema: ContextBlockMetadataSchemaV1}
}
