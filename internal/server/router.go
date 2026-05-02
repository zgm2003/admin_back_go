package server

import (
	"admin_back_go/internal/response"
	"admin_back_go/internal/version"

	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"service": "admin-api",
			"status":  "ok",
			"version": version.Version,
		})
	})

	api := router.Group("/api/v1")
	api.GET("/ping", func(c *gin.Context) {
		response.OK(c, gin.H{"message": "pong"})
	})

	return router
}
