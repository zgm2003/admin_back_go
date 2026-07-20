package architecture

import "testing"

func TestImageGenerationOwnedBySingleAICapability(t *testing.T) {
	root := backendRoot(t)
	mustExist(t, root, "internal/module/ai/image/service.go")
	mustExist(t, root, "internal/module/ai/image/jobs.go")
	mustNotExist(t, root, "internal/module/ai/image/transport/canvas")
	mustNotExist(t, root, "internal/module/ai/image/transport/admin/route.go")
	mustNotExist(t, root, "internal/module/ai/adminimage")
	mustNotExist(t, root, "internal/module/canvas/image")
}
