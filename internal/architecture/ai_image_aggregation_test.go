package architecture

import "testing"

func TestAIImageOwnedByAIModule(t *testing.T) {
	root := backendRoot(t)
	mustNotExist(t, root, "internal/module/aiimage")
	mustExist(t, root, "internal/module/ai/image/transport/admin/route.go")
	mustExist(t, root, "internal/module/ai/image/jobs.go")
}
