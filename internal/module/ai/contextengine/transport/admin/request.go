package admin

import contextengine "admin_back_go/internal/module/ai/contextengine"

type profileCreateRequest struct {
	Name                     string  `json:"name" binding:"required"`
	EmbeddingProviderModelID uint64  `json:"embedding_provider_model_id" binding:"required"`
	EmbeddingDimensions      uint32  `json:"embedding_dimensions" binding:"required"`
	EmbeddingMaxInputTokens  int64   `json:"embedding_max_input_tokens" binding:"required"`
	EmbeddingTokenCounterID  string  `json:"embedding_token_counter_id" binding:"required"`
	DenseDistance            string  `json:"dense_distance" binding:"required"`
	DenseMinScore            string  `json:"dense_min_score" binding:"required"`
	RerankerProviderModelID  *uint64 `json:"reranker_provider_model_id"`
	RerankerMinScore         *string `json:"reranker_min_score"`
	MemoryProviderModelID    *uint64 `json:"memory_provider_model_id"`
}
type profileUpdateRequest struct {
	Name   string                      `json:"name" binding:"required"`
	Status contextengine.ProfileStatus `json:"status" binding:"required"`
}
type spaceRequest struct {
	ProfileID   uint64 `json:"profile_id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Status      string `json:"status" binding:"required"`
}
type documentRequest struct {
	SpaceID               *uint64 `json:"space_id"`
	ConversationID        *uint64 `json:"conversation_id"`
	SourceMessageID       *uint64 `json:"source_message_id"`
	SourceAttachmentIndex *uint32 `json:"source_attachment_index"`
	Title                 string  `json:"title" binding:"required"`
	SourceStorageProvider string  `json:"source_storage_provider" binding:"required"`
	SourceObjectKey       string  `json:"source_object_key" binding:"required"`
	SourceETag            string  `json:"source_etag" binding:"required"`
	SourceSize            int64   `json:"source_size_bytes" binding:"required"`
	SourceFilename        string  `json:"source_filename" binding:"required"`
}

func profileCreateInput(request profileCreateRequest) contextengine.CreateProfileInput {
	return contextengine.CreateProfileInput{Name: request.Name, EmbeddingProviderModelID: request.EmbeddingProviderModelID, EmbeddingDimensions: request.EmbeddingDimensions, EmbeddingMaxInputTokens: request.EmbeddingMaxInputTokens, EmbeddingTokenCounterID: request.EmbeddingTokenCounterID, DenseDistance: request.DenseDistance, DenseMinScore: request.DenseMinScore, RerankerProviderModelID: request.RerankerProviderModelID, RerankerMinScore: request.RerankerMinScore, MemoryProviderModelID: request.MemoryProviderModelID}
}
func spaceInput(request spaceRequest) contextengine.CreateSpaceInput {
	return contextengine.CreateSpaceInput{ProfileID: request.ProfileID, Name: request.Name, Description: request.Description, Status: request.Status}
}
func documentInput(request documentRequest) contextengine.CreateDocumentInput {
	return contextengine.CreateDocumentInput{SpaceID: request.SpaceID, ConversationID: request.ConversationID, SourceMessageID: request.SourceMessageID, SourceAttachmentIndex: request.SourceAttachmentIndex, Title: request.Title, SourceStorageProvider: request.SourceStorageProvider, SourceObjectKey: request.SourceObjectKey, SourceETag: request.SourceETag, SourceSize: request.SourceSize, SourceFilename: request.SourceFilename}
}
