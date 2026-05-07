package chat

import (
	"admin_back_go/internal/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	validate.MustRegister()
	handler := NewHandler(service)

	v1 := router.Group("/api/admin/v1/chat")
	v1.GET("/conversations", handler.ListConversations)
	v1.POST("/conversations/private", handler.CreatePrivate)
	v1.DELETE("/conversations/:id", handler.DeleteConversation)
	v1.PATCH("/conversations/:id/pin", handler.TogglePin)
	v1.GET("/conversations/:id/messages", handler.ListMessages)
	v1.POST("/conversations/:id/messages", handler.SendMessage)
	v1.PATCH("/conversations/:id/read", handler.MarkRead)
	v1.GET("/contacts", handler.ListContacts)
	v1.POST("/contacts/:user_id/requests", handler.AddContact)
	v1.PATCH("/contacts/:user_id/confirm", handler.ConfirmContact)
	v1.DELETE("/contacts/:user_id", handler.DeleteContact)
}
