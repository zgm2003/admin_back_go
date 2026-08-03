package admin

import "github.com/gin-gonic/gin"

func RegisterRoutes(routes gin.IRoutes, trustedPlatform string, service HTTPService) {
	handler := NewHandler(trustedPlatform, service)
	routes.POST("/ai-context-profiles", handler.CreateProfile)
	routes.PUT("/ai-context-profiles/:id", handler.UpdateProfile)
	routes.POST("/ai-context-spaces", handler.CreateSpace)
	routes.PUT("/ai-context-spaces/:id", handler.UpdateSpace)
	routes.DELETE("/ai-context-spaces/:id", handler.DeleteSpace)
	routes.POST("/ai-context-documents", handler.CreateDocument)
	routes.POST("/ai-context-documents/:id/reindex", handler.ReindexDocument)
}
