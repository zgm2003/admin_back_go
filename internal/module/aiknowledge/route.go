package aiknowledge

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	h := NewHandler(service)
	group := router.Group("/api/admin/v1")
	{
		group.GET("/ai-knowledge-bases/page-init", h.Init)
		group.GET("/ai-knowledge-bases", h.ListBases)
		group.GET("/ai-knowledge-bases/:id", h.GetBase)
		group.POST("/ai-knowledge-bases", h.CreateBase)
		group.PUT("/ai-knowledge-bases/:id", h.UpdateBase)
		group.PATCH("/ai-knowledge-bases/:id/status", h.ChangeBaseStatus)
		group.DELETE("/ai-knowledge-bases/:id", h.DeleteBase)
		group.GET("/ai-knowledge-bases/:id/documents", h.ListDocuments)
		group.POST("/ai-knowledge-bases/:id/documents", h.CreateDocument)
		group.GET("/ai-knowledge-documents/:id", h.GetDocument)
		group.PUT("/ai-knowledge-documents/:id", h.UpdateDocument)
		group.PATCH("/ai-knowledge-documents/:id/status", h.ChangeDocumentStatus)
		group.DELETE("/ai-knowledge-documents/:id", h.DeleteDocument)
		group.POST("/ai-knowledge-documents/:id/reindex", h.ReindexDocument)
		group.GET("/ai-knowledge-documents/:id/chunks", h.ListChunks)
		group.POST("/ai-knowledge-bases/:id/retrieval-tests", h.RetrievalTest)
		group.GET("/ai-agents/:id/knowledge-bases", h.AgentKnowledgeBases)
		group.PUT("/ai-agents/:id/knowledge-bases", h.UpdateAgentKnowledgeBases)
	}
}
