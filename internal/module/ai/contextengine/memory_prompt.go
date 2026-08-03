package contextengine

import (
	"fmt"
	"strings"
)

func BuildMemoryPrompt(parentSummary string, turns []ConversationTurn) (string, error) {
	if len(turns) == 0 {
		return "", ErrMemoryInvalid
	}
	var builder strings.Builder
	builder.WriteString("Create a faithful rolling conversation memory. Keep separate sections for user claims, assistant answers, confirmed facts, unresolved matters, and attachment references. Never promote assistant speculation to a user claim.\n")
	if strings.TrimSpace(parentSummary) != "" {
		builder.WriteString("\nPREVIOUS MEMORY:\n")
		builder.WriteString(parentSummary)
	}
	for _, turn := range turns {
		if _, err := ConversationTurnSourceSHA256(turn); err != nil {
			return "", err
		}
		builder.WriteString(fmt.Sprintf("\n\nTURN %d\nUSER:\n%s\nASSISTANT:\n%s", turn.UserMessage.ID, turn.UserMessage.Content, turn.AssistantMessage.Content))
		for _, attachment := range turn.UserMessage.Attachments {
			builder.WriteString(fmt.Sprintf("\nATTACHMENT[%d]: %s (%s, %d bytes, etag=%s)", attachment.Index, attachment.Name, attachment.MIMEType, attachment.Size, attachment.ETag))
		}
		for _, tool := range turn.ToolGroups {
			builder.WriteString(fmt.Sprintf("\nTOOL %s %s\nARGS: %s\nRESULT: %s", tool.CallID, tool.Name, tool.Arguments, tool.Result))
		}
	}
	return builder.String(), nil
}
