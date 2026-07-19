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
			Contract: &adminroute.HTTPContract{
				Response: aiknowledgemodule.InitResponse{},
			},
		}, h.PageInit)
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-knowledge-bases",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
			Contract: &adminroute.HTTPContract{
				Query:    baseListRequest{},
				Response: aiknowledgemodule.BaseListResponse{},
			},
		}, h.ListBases)
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-knowledge-bases/:id",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
			Contract: &adminroute.HTTPContract{
				Response: aiknowledgemodule.BaseDetailResponse{},
			},
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
			Contract: &adminroute.HTTPContract{
				Request:  baseMutationRequest{},
				Response: adminroute.IDData{},
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
			Contract: &adminroute.HTTPContract{
				Request:  baseMutationRequest{},
				Response: adminroute.EmptyData{},
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
			Contract: &adminroute.HTTPContract{
				Request:  statusRequest{},
				Response: adminroute.EmptyData{},
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
			Contract: &adminroute.HTTPContract{
				Response: adminroute.EmptyData{},
			},
		}, h.DeleteBase)
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-knowledge-bases/:id/documents",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
			Contract: &adminroute.HTTPContract{
				Query:    documentListRequest{},
				Response: aiknowledgemodule.DocumentListResponse{},
			},
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
			Contract: &adminroute.HTTPContract{
				Request:  documentMutationRequest{},
				Response: adminroute.IDData{},
			},
		}, h.CreateDocument)
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-knowledge-documents/:id",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
			Contract: &adminroute.HTTPContract{
				Response: aiknowledgemodule.DocumentDetailResponse{},
			},
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
			Contract: &adminroute.HTTPContract{
				Request:  documentMutationRequest{},
				Response: adminroute.EmptyData{},
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
			Contract: &adminroute.HTTPContract{
				Request:  statusRequest{},
				Response: adminroute.EmptyData{},
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
			Contract: &adminroute.HTTPContract{
				Response: adminroute.EmptyData{},
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
			Contract: &adminroute.HTTPContract{
				Response: adminroute.EmptyData{},
			},
		}, h.ReindexDocument)
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-knowledge-documents/:id/chunks",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
			Contract: &adminroute.HTTPContract{
				Response: aiknowledgemodule.ChunkListResponse{},
			},
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
			Contract: &adminroute.HTTPContract{
				Request:  retrievalTestRequest{},
				Response: aiknowledgemodule.RetrievalResult{},
			},
		}, h.RetrievalTest)
		routes.Handle(adminroute.Definition{
			Method: http.MethodGet,
			Path:   "/api/admin/v1/ai-agents/:id/knowledge-bases",
			Access: adminroute.Authenticated(),
			Audit:  adminroute.NoAudit("read-only"),
			Contract: &adminroute.HTTPContract{
				Response: aiknowledgemodule.AgentKnowledgeBindingsResponse{},
			},
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
			Contract: &adminroute.HTTPContract{
				Request:  updateAgentKnowledgeBindingsRequest{},
				Response: adminroute.EmptyData{},
			},
		}, h.UpdateAgentKnowledgeBases)
	}
}
