package admin

import aiknowledgemodule "admin_back_go/internal/module/aiknowledge"

type (
	ChunkOptions                      = aiknowledgemodule.ChunkOptions
	TextChunk                         = aiknowledgemodule.TextChunk
	RetrievalOptions                  = aiknowledgemodule.RetrievalOptions
	RetrievalCandidate                = aiknowledgemodule.RetrievalCandidate
	RetrievalResult                   = aiknowledgemodule.RetrievalResult
	RetrievalHit                      = aiknowledgemodule.RetrievalHit
	SelectedHit                       = aiknowledgemodule.SelectedHit
	ScoredHit                         = aiknowledgemodule.ScoredHit
	InitResponse                      = aiknowledgemodule.InitResponse
	InitDict                          = aiknowledgemodule.InitDict
	Page                              = aiknowledgemodule.Page
	BaseListQuery                     = aiknowledgemodule.BaseListQuery
	BaseListResponse                  = aiknowledgemodule.BaseListResponse
	BaseDetailResponse                = aiknowledgemodule.BaseDetailResponse
	BaseDTO                           = aiknowledgemodule.BaseDTO
	BaseMutationInput                 = aiknowledgemodule.BaseMutationInput
	DocumentListQuery                 = aiknowledgemodule.DocumentListQuery
	DocumentListResponse              = aiknowledgemodule.DocumentListResponse
	DocumentDetailResponse            = aiknowledgemodule.DocumentDetailResponse
	DocumentDTO                       = aiknowledgemodule.DocumentDTO
	DocumentMutationInput             = aiknowledgemodule.DocumentMutationInput
	ChunkListResponse                 = aiknowledgemodule.ChunkListResponse
	ChunkDTO                          = aiknowledgemodule.ChunkDTO
	RetrievalTestInput                = aiknowledgemodule.RetrievalTestInput
	KnowledgeBaseOptionRow            = aiknowledgemodule.KnowledgeBaseOptionRow
	KnowledgeBaseOption               = aiknowledgemodule.KnowledgeBaseOption
	AgentKnowledgeBindingRow          = aiknowledgemodule.AgentKnowledgeBindingRow
	AgentKnowledgeBindingInput        = aiknowledgemodule.AgentKnowledgeBindingInput
	UpdateAgentKnowledgeBindingsInput = aiknowledgemodule.UpdateAgentKnowledgeBindingsInput
	AgentKnowledgeBindingsResponse    = aiknowledgemodule.AgentKnowledgeBindingsResponse
	AgentKnowledgeBindingItem         = aiknowledgemodule.AgentKnowledgeBindingItem
	RuntimeBindingRow                 = aiknowledgemodule.RuntimeBindingRow
	KnowledgeRuntimeInput             = aiknowledgemodule.KnowledgeRuntimeInput
	KnowledgeContextResult            = aiknowledgemodule.KnowledgeContextResult
	CreateRetrievalInput              = aiknowledgemodule.CreateRetrievalInput
	FinishRetrievalInput              = aiknowledgemodule.FinishRetrievalInput
	Repository                        = aiknowledgemodule.Repository
	HTTPService                       = aiknowledgemodule.HTTPService
	RuntimeService                    = aiknowledgemodule.RuntimeService
	KnowledgeBase                     = aiknowledgemodule.KnowledgeBase
	KnowledgeDocument                 = aiknowledgemodule.KnowledgeDocument
	KnowledgeChunk                    = aiknowledgemodule.KnowledgeChunk
	AgentKnowledgeBase                = aiknowledgemodule.AgentKnowledgeBase
	KnowledgeRetrieval                = aiknowledgemodule.KnowledgeRetrieval
	KnowledgeRetrievalHit             = aiknowledgemodule.KnowledgeRetrievalHit
	GormRepository                    = aiknowledgemodule.GormRepository
	Service                           = aiknowledgemodule.Service
)
