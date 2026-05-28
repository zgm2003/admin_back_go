package architecture

import "testing"

func TestAICoreModulesOwnedByAIModule(t *testing.T) {
	root := backendRoot(t)
	for _, rel := range []string{
		"internal/module/aiprovider",
		"internal/module/aiagent",
		"internal/module/aitool",
	} {
		mustNotExist(t, root, rel)
	}
	for _, rel := range []string{
		"internal/module/ai/provider/transport/admin/route.go",
		"internal/module/ai/agent/transport/admin/route.go",
		"internal/module/ai/tool/transport/admin/route.go",
	} {
		mustExist(t, root, rel)
	}
}
