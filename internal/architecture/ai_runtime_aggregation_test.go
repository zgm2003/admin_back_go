package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestAIConversationRuntimeDoesNotReferenceOldActivePaths(t *testing.T) {
	root := backendRoot(t)
	oldPaths := []string{
		"internal/module/aiconversation",
		"internal/module/aimessage",
		"internal/module/aichat",
		"internal/module/airun",
		"admin_back_go/internal/module/aiconversation",
		"admin_back_go/internal/module/aimessage",
		"admin_back_go/internal/module/aichat",
		"admin_back_go/internal/module/airun",
	}
	for _, rel := range []string{
		"docs/architecture.md",
		"database/legacy-migrations/20260510_ai_knowledge_rag.sql",
	} {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := string(body)
		for _, oldPath := range oldPaths {
			if strings.Contains(text, oldPath) {
				t.Errorf("%s still references old AI runtime path %q", rel, oldPath)
			}
		}
	}
}

func TestBackendArchitectureDocumentsRetainedAIRuntime(t *testing.T) {
	root := backendRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "docs", "architecture.md"))
	if err != nil {
		t.Fatalf("read docs/architecture.md: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"chat, agent_generate, text_generate, and image_generate",
		"AI audio and video generation are retired",
		"ai_runs",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("backend architecture must document retained AI runtime fact %q", want)
		}
	}
	for _, retired := range []string{
		"internal/module/ai/audio",
		"internal/module/ai/video",
		"canvas_audio_generate",
		"canvas_video_generate",
		"POST /api/canvas/v1/ai/audios",
		"POST /api/canvas/v1/ai/videos",
	} {
		if strings.Contains(text, retired) {
			t.Fatalf("backend architecture still documents retired AI runtime fact %q", retired)
		}
	}
}
