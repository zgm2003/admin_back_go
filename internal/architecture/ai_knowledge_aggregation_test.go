package architecture

import "testing"

func TestAIKnowledgeOwnedByAIModule(t *testing.T) {
	root := backendRoot(t)
	mustNotExist(t, root, "internal/module/aiknowledge")
	mustExist(t, root, "internal/module/ai/knowledge/transport/admin/route.go")
	mustExist(t, root, "internal/module/ai/knowledge/retriever.go")
}
