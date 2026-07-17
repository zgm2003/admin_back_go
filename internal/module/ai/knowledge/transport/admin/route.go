package admin

import (
	"net/http"

	aiknowledgemodule "admin_back_go/internal/module/ai/knowledge"
	"admin_back_go/internal/server/adminroute"

	"github.com/gin-gonic/gin"
)

func Register(router *gin.Engine, service aiknowledgemodule.HTTPService, routeRegistries ...*adminroute.Registry) {
	h := NewHandler(service)
	routes := adminroute.NewRegistrar(router, routeRegistries...)
	{
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-knowledge-bases/page-init",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
		}, h.PageInit)
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-knowledge-bases",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
		}, h.ListBases)
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-knowledge-bases/:id",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
		}, h.GetBase)
		routes.Handle(adminroute.Definition{
			Method: http.MethodPost,
			Path:   "/api/admin/v1/ai-knowledge-bases",
			Access: adminroute.Permission("ai_knowledge_add"),
			Audit: adminroute.AuditDecision{
				Enabled: true,
				Module:  "ai_knowledge",
				Action:  "create",
				Title:   "新增AI知识库",
			},
		}, h.CreateBase)
		routes.Handle(adminroute.Definition{
			Method: http.MethodPut,
			Path:   "/api/admin/v1/ai-knowledge-bases/:id",
			Access: adminroute.Permission("ai_knowledge_edit"),
			Audit: adminroute.AuditDecision{
				Enabled: true,
				Module:  "ai_knowledge",
				Action:  "update",
				Title:   "编辑AI知识库",
			},
		}, h.UpdateBase)
		routes.Handle(adminroute.Definition{
			Method: http.MethodPatch,
			Path:   "/api/admin/v1/ai-knowledge-bases/:id/status",
			Access: adminroute.Permission("ai_knowledge_status"),
			Audit: adminroute.AuditDecision{
				Enabled: true,
				Module:  "ai_knowledge",
				Action:  "change_status",
				Title:   "修改AI知识库状态",
			},
		}, h.ChangeBaseStatus)
		routes.Handle(adminroute.Definition{
			Method: http.MethodDelete,
			Path:   "/api/admin/v1/ai-knowledge-bases/:id",
			Access: adminroute.Permission("ai_knowledge_del"),
			Audit: adminroute.AuditDecision{
				Enabled: true,
				Module:  "ai_knowledge",
				Action:  "delete",
				Title:   "删除AI知识库",
			},
		}, h.DeleteBase)
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-knowledge-bases/:id/documents",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
		}, h.ListDocuments)
		routes.Handle(adminroute.Definition{
			Method: http.MethodPost,
			Path:   "/api/admin/v1/ai-knowledge-bases/:id/documents",
			Access: adminroute.Permission("ai_knowledge_document_add"),
			Audit: adminroute.AuditDecision{
				Enabled: true,
				Module:  "ai_knowledge_document",
				Action:  "create",
				Title:   "新增AI知识库文档",
			},
		}, h.CreateDocument)
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-knowledge-documents/:id",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
		}, h.GetDocument)
		routes.Handle(adminroute.Definition{
			Method: http.MethodPut,
			Path:   "/api/admin/v1/ai-knowledge-documents/:id",
			Access: adminroute.Permission("ai_knowledge_document_edit"),
			Audit: adminroute.AuditDecision{
				Enabled: true,
				Module:  "ai_knowledge_document",
				Action:  "update",
				Title:   "编辑AI知识库文档",
			},
		}, h.UpdateDocument)
		routes.Handle(adminroute.Definition{
			Method: http.MethodPatch,
			Path:   "/api/admin/v1/ai-knowledge-documents/:id/status",
			Access: adminroute.Permission("ai_knowledge_document_status"),
			Audit: adminroute.AuditDecision{
				Enabled: true,
				Module:  "ai_knowledge_document",
				Action:  "change_status",
				Title:   "修改AI知识库文档状态",
			},
		}, h.ChangeDocumentStatus)
		routes.Handle(adminroute.Definition{
			Method: http.MethodDelete,
			Path:   "/api/admin/v1/ai-knowledge-documents/:id",
			Access: adminroute.Permission("ai_knowledge_document_del"),
			Audit: adminroute.AuditDecision{
				Enabled: true,
				Module:  "ai_knowledge_document",
				Action:  "delete",
				Title:   "删除AI知识库文档",
			},
		}, h.DeleteDocument)
		routes.Handle(adminroute.Definition{
			Method: http.MethodPost,
			Path:   "/api/admin/v1/ai-knowledge-documents/:id/reindex",
			Access: adminroute.Permission("ai_knowledge_reindex"),
			Audit: adminroute.AuditDecision{
				Enabled: true,
				Module:  "ai_knowledge",
				Action:  "reindex",
				Title:   "重建AI知识库文档索引",
			},
		}, h.ReindexDocument)
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-knowledge-documents/:id/chunks",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
		}, h.ListChunks)
		routes.Handle(adminroute.Definition{
			Method: http.MethodPost,
			Path:   "/api/admin/v1/ai-knowledge-bases/:id/retrieval-tests",
			Access: adminroute.Permission("ai_knowledge_retrieval_test"),
			Audit: adminroute.AuditDecision{
				Enabled: true,
				Module:  "ai_knowledge",
				Action:  "retrieval_test",
				Title:   "AI知识库检索测试",
			},
		}, h.RetrievalTest)
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-agents/:id/knowledge-bases",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
		}, h.AgentKnowledgeBases)
		routes.Handle(adminroute.Definition{
			Method: http.MethodPut,
			Path:   "/api/admin/v1/ai-agents/:id/knowledge-bases",
			Access: adminroute.Permission("ai_agent_binding_add"),
			Audit: adminroute.AuditDecision{
				Enabled: true,
				Module:  "ai_agent_knowledge",
				Action:  "update_binding",
				Title:   "更新智能体知识库绑定",
			},
		}, h.UpdateAgentKnowledgeBases)
	}
}
