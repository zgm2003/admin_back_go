package admin

import (
	"net/http"
	"strings"

	contextengine "admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes is the sole Context Admin route table. Definitions use the
// published full path even when isolated transport tests provide a prefixed
// RouterGroup.
func RegisterRoutes(router gin.IRoutes, trustedPlatform string, service HTTPService, registries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(trustedPlatform, service)
	registry := adminroute.NewRegistrar(router, registries...)
	base := ""
	if value, ok := router.(interface{ BasePath() string }); ok {
		base = strings.TrimSuffix(strings.TrimSpace(value.BasePath()), "/")
	}
	register := func(definition adminroute.Definition, handlerFunc gin.HandlerFunc) {
		if base != "" && base != "/" && strings.HasPrefix(definition.Path, base+"/") {
			definition.Path = strings.TrimPrefix(definition.Path, base)
		}
		registry.Handle(definition, handlerFunc)
	}
	register(adminroute.Definition{Method: http.MethodGet, Path: "/api/admin/v1/ai/context/page-init", OperationID: "ai_context_page_init", Access: adminroute.Permission("ai_context_view"), Audit: adminroute.NoAudit("read-only"), Contract: &adminroute.HTTPContract{Response: contextengine.ContextPageInitResponse{}}}, handler.PageInit)
	register(adminroute.Definition{Method: http.MethodGet, Path: "/api/admin/v1/ai/context-profiles", OperationID: "ai_context_profiles_list", Access: adminroute.Permission("ai_context_view"), Audit: adminroute.NoAudit("read-only"), Contract: &adminroute.HTTPContract{Query: profileListRequest{}, Response: contextengine.ProfileListResponse{}}}, handler.ListProfiles)
	register(adminroute.Definition{Method: http.MethodGet, Path: "/api/admin/v1/ai/context-profiles/:id", OperationID: "ai_context_profile_get", Access: adminroute.Permission("ai_context_view"), Audit: adminroute.NoAudit("read-only"), Contract: &adminroute.HTTPContract{Response: contextengine.ProfileDTO{}}}, handler.GetProfile)
	register(adminroute.Definition{Method: http.MethodPost, Path: "/api/admin/v1/ai/context-profiles", OperationID: "ai_context_profile_create", Access: adminroute.Permission("ai_context_profile_manage"), Audit: adminroute.Audit("ai_context_profile", "create", "新增上下文配置"), Contract: &adminroute.HTTPContract{Request: profileCreateRequest{}, Response: contextengine.ProfileDTO{}}}, handler.CreateProfile)
	register(adminroute.Definition{Method: http.MethodPut, Path: "/api/admin/v1/ai/context-profiles/:id", OperationID: "ai_context_profile_update_metadata", Access: adminroute.Permission("ai_context_profile_manage"), Audit: adminroute.Audit("ai_context_profile", "update_metadata", "编辑上下文配置"), Contract: &adminroute.HTTPContract{Request: profileUpdateRequest{}, Response: contextengine.ProfileDTO{}}}, handler.UpdateProfile)
	register(adminroute.Definition{Method: http.MethodPatch, Path: "/api/admin/v1/ai/context-profiles/:id/status", OperationID: "ai_context_profile_change_status", Access: adminroute.Permission("ai_context_profile_manage"), Audit: adminroute.Audit("ai_context_profile", "change_status", "修改上下文配置状态"), Contract: &adminroute.HTTPContract{Request: contextStatusRequest{}, Response: contextengine.ProfileDTO{}}}, handler.ChangeProfileStatus)

	register(adminroute.Definition{Method: http.MethodGet, Path: "/api/admin/v1/ai/context-spaces", OperationID: "ai_context_spaces_list", Access: adminroute.Permission("ai_context_view"), Audit: adminroute.NoAudit("read-only"), Contract: &adminroute.HTTPContract{Query: spaceListRequest{}, Response: contextengine.SpaceListResponse{}}}, handler.ListSpaces)
	register(adminroute.Definition{Method: http.MethodGet, Path: "/api/admin/v1/ai/context-spaces/:id", OperationID: "ai_context_space_get", Access: adminroute.Permission("ai_context_view"), Audit: adminroute.NoAudit("read-only"), Contract: &adminroute.HTTPContract{Response: contextengine.SpaceDTO{}}}, handler.GetSpace)
	register(adminroute.Definition{Method: http.MethodPost, Path: "/api/admin/v1/ai/context-spaces", OperationID: "ai_context_space_create", Access: adminroute.Permission("ai_context_manage"), Audit: adminroute.Audit("ai_context_space", "create", "新增上下文空间"), Contract: &adminroute.HTTPContract{Request: spaceRequest{}, Response: contextengine.SpaceDTO{}}}, handler.CreateSpace)
	register(adminroute.Definition{Method: http.MethodPut, Path: "/api/admin/v1/ai/context-spaces/:id", OperationID: "ai_context_space_update", Access: adminroute.Permission("ai_context_manage"), Audit: adminroute.Audit("ai_context_space", "update", "编辑上下文空间"), Contract: &adminroute.HTTPContract{Request: spaceRequest{}, Response: contextengine.SpaceDTO{}}}, handler.UpdateSpace)
	register(adminroute.Definition{Method: http.MethodPatch, Path: "/api/admin/v1/ai/context-spaces/:id/status", OperationID: "ai_context_space_change_status", Access: adminroute.Permission("ai_context_manage"), Audit: adminroute.Audit("ai_context_space", "change_status", "修改上下文空间状态"), Contract: &adminroute.HTTPContract{Request: contextStatusRequest{}, Response: contextengine.SpaceDTO{}}}, handler.ChangeSpaceStatus)
	register(adminroute.Definition{Method: http.MethodDelete, Path: "/api/admin/v1/ai/context-spaces/:id", OperationID: "ai_context_space_delete", Access: adminroute.Permission("ai_context_manage"), Audit: adminroute.Audit("ai_context_space", "delete", "删除上下文空间"), Contract: &adminroute.HTTPContract{Response: adminroute.EmptyData{}}}, handler.DeleteSpace)

	register(adminroute.Definition{Method: http.MethodGet, Path: "/api/admin/v1/ai/context-spaces/:id/documents", OperationID: "ai_context_space_documents_list", Access: adminroute.Permission("ai_context_view"), Audit: adminroute.NoAudit("read-only"), Contract: &adminroute.HTTPContract{Query: documentListRequest{}, Response: contextengine.DocumentListResponse{}}}, handler.ListSpaceDocuments)
	register(adminroute.Definition{Method: http.MethodPost, Path: "/api/admin/v1/ai/context-spaces/:id/documents", OperationID: "ai_context_document_create", Access: adminroute.Permission("ai_context_document_manage"), Audit: payloadFreeAudit("ai_context_document", "create", "新增上下文文档"), Contract: &adminroute.HTTPContract{Request: spaceDocumentRequest{}, Response: contextengine.DocumentAdminDTO{}}}, handler.CreateSpaceDocument)
	register(adminroute.Definition{Method: http.MethodGet, Path: "/api/admin/v1/ai/context-documents/:id", OperationID: "ai_context_document_get", Access: adminroute.Permission("ai_context_view"), Audit: adminroute.NoAudit("read-only"), Contract: &adminroute.HTTPContract{Response: contextengine.DocumentAdminDTO{}}}, handler.GetDocument)
	register(adminroute.Definition{Method: http.MethodGet, Path: "/api/admin/v1/ai/context-documents/:id/versions", OperationID: "ai_context_document_versions_list", Access: adminroute.Permission("ai_context_view"), Audit: adminroute.NoAudit("read-only"), Contract: &adminroute.HTTPContract{Response: contextengine.DocumentVersionListResponse{}}}, handler.ListDocumentVersions)
	register(adminroute.Definition{Method: http.MethodPost, Path: "/api/admin/v1/ai/context-documents/:id/versions", OperationID: "ai_context_document_version_create", Access: adminroute.Permission("ai_context_document_manage"), Audit: payloadFreeAudit("ai_context_document", "create_version", "创建上下文文档版本"), Contract: &adminroute.HTTPContract{Request: documentVersionRequest{}, Response: contextengine.DocumentAdminDTO{}}}, handler.CreateDocumentVersion)
	register(adminroute.Definition{Method: http.MethodPatch, Path: "/api/admin/v1/ai/context-documents/:id/status", OperationID: "ai_context_document_change_status", Access: adminroute.Permission("ai_context_document_manage"), Audit: payloadFreeAudit("ai_context_document", "change_status", "修改上下文文档状态"), Contract: &adminroute.HTTPContract{Request: contextStatusRequest{}, Response: contextengine.DocumentAdminDTO{}}}, handler.ChangeDocumentStatus)
	register(adminroute.Definition{Method: http.MethodDelete, Path: "/api/admin/v1/ai/context-documents/:id", OperationID: "ai_context_document_delete", Access: adminroute.Permission("ai_context_document_manage"), Audit: adminroute.Audit("ai_context_document", "delete", "删除上下文文档"), Contract: &adminroute.HTTPContract{Response: adminroute.EmptyData{}}}, handler.DeleteDocument)
	register(adminroute.Definition{Method: http.MethodPost, Path: "/api/admin/v1/ai/context-documents/:id/reindex", OperationID: "ai_context_document_reindex", Access: adminroute.Permission("ai_context_document_manage"), Audit: payloadFreeAudit("ai_context_document", "reindex", "重建上下文文档索引"), Contract: &adminroute.HTTPContract{Response: contextengine.DocumentAdminDTO{}}}, handler.ReindexDocument)
	register(adminroute.Definition{Method: http.MethodGet, Path: "/api/admin/v1/ai/context-document-versions/:id/preview", OperationID: "ai_context_document_version_preview", Access: adminroute.Permission("ai_context_view"), Audit: adminroute.NoAudit("read-only"), Contract: &adminroute.HTTPContract{Response: contextengine.DocumentVersionPreviewResponse{}}}, handler.PreviewDocumentVersion)

	register(adminroute.Definition{Method: http.MethodPost, Path: "/api/admin/v1/ai/context-evaluations", OperationID: "ai_context_evaluate", Access: adminroute.Permission("ai_context_evaluate"), Audit: payloadFreeAudit("ai_context", "evaluate", "执行上下文评测"), Contract: &adminroute.HTTPContract{Request: evaluationRequest{}, Response: contextengine.ContextEvaluationResponse{}}}, handler.Evaluate)
	register(adminroute.Definition{Method: http.MethodGet, Path: "/api/admin/v1/ai/agents/:id/context-profile", OperationID: "ai_agent_context_profile_get", Access: adminroute.Permission("ai_context_view"), Audit: adminroute.NoAudit("read-only"), Contract: &adminroute.HTTPContract{Response: contextengine.AgentContextProfileInput{}}}, handler.GetAgentContextProfile)
	register(adminroute.Definition{Method: http.MethodPut, Path: "/api/admin/v1/ai/agents/:id/context-profile", OperationID: "ai_agent_context_profile_update", Access: adminroute.Permission("ai_context_profile_manage"), Audit: adminroute.Audit("ai_agent_context", "update_profile", "修改AI智能体上下文配置"), Contract: &adminroute.HTTPContract{Request: agentContextProfileRequest{}, Response: contextengine.AgentContextProfileInput{}}}, handler.UpdateAgentContextProfile)
	register(adminroute.Definition{Method: http.MethodGet, Path: "/api/admin/v1/ai/agents/:id/context-spaces", OperationID: "ai_agent_context_spaces_get", Access: adminroute.Permission("ai_context_view"), Audit: adminroute.NoAudit("read-only"), Contract: &adminroute.HTTPContract{Response: contextengine.AgentContextSpacesInput{}}}, handler.GetAgentContextSpaces)
	register(adminroute.Definition{Method: http.MethodPut, Path: "/api/admin/v1/ai/agents/:id/context-spaces", OperationID: "ai_agent_context_spaces_update", Access: adminroute.Permission("ai_context_profile_manage"), Audit: adminroute.Audit("ai_agent_context", "update_spaces", "修改AI智能体上下文空间"), Contract: &adminroute.HTTPContract{Request: agentContextSpacesRequest{}, Response: contextengine.AgentContextSpacesInput{}}}, handler.UpdateAgentContextSpaces)
}

func payloadFreeAudit(module string, action string, title string) adminroute.AuditDecision {
	decision := adminroute.Audit(module, action, title)
	decision.SkipRequestPayload = true
	decision.SkipResponsePayload = true
	return decision
}

// Register is kept as the conventional module entrypoint used by the server.
func Register(router gin.IRoutes, service HTTPService, registries ...*adminroute.Registry) {
	RegisterRoutes(router, "admin", service, registries...)
}
