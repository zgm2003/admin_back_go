package architecture

import "testing"

func TestAIConversationRuntimeOwnedByAIModule(t *testing.T) {
	root := backendRoot(t)
	for _, rel := range []string{
		"internal/module/aiconversation",
		"internal/module/aimessage",
		"internal/module/aichat",
		"internal/module/airun",
	} {
		mustNotExist(t, root, rel)
	}
	for _, rel := range []string{
		"internal/module/ai/conversation/transport/admin/route.go",
		"internal/module/ai/message/transport/admin/route.go",
		"internal/module/ai/chat/transport/admin/route.go",
		"internal/module/ai/chat/jobs.go",
		"internal/module/ai/chat/events.go",
		"internal/module/ai/run/transport/admin/route.go",
	} {
		mustExist(t, root, rel)
	}
}
