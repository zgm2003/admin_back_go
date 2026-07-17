package architecture

import (
	"testing"
)

func TestInfraRenameComplete(t *testing.T) {
	root := backendRoot(t)
	mustExist(t, root, "internal/infra")
	mustExist(t, root, "internal/platform/admin/graph.go")
	mustExist(t, root, "internal/platform/admin/build.go")
	mustExist(t, root, "internal/platform/retired/graph.go")

	for _, legacyInfraPath := range []string{
		"internal/platform/database",
		"internal/platform/redisclient",
		"internal/platform/taskqueue",
		"internal/platform/storage",
		"internal/platform/payment",
		"internal/platform/ai",
	} {
		mustNotExist(t, root, legacyInfraPath)
	}
}
