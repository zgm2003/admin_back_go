package aiknowledge

import (
	"context"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aiknowledge repository not configured")

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) ListBases(ctx context.Context, query BaseListQuery) ([]KnowledgeBase, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.activeBases(ctx)
	if query.Name != "" {
		db = db.Where("name LIKE ?", query.Name+"%")
	}
	if query.Code != "" {
		db = db.Where("code = ?", query.Code)
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	var total int64
	if err := db.Model(&KnowledgeBase{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []KnowledgeBase
	err := db.Order("id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) GetBase(ctx context.Context, id uint64) (*KnowledgeBase, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row KnowledgeBase
	err := r.activeBases(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) CreateBase(ctx context.Context, row KnowledgeBase) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) UpdateBase(ctx context.Context, id uint64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeBases(ctx).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) ChangeBaseStatus(ctx context.Context, id uint64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeBases(ctx).Where("id = ?", id).Update("status", status).Error
}

func (r *GormRepository) DeleteBase(ctx context.Context, id uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&KnowledgeBase{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Update("is_del", enum.CommonYes).Error; err != nil {
			return err
		}
		if err := tx.Model(&KnowledgeDocument{}).Where("knowledge_base_id = ? AND is_del = ?", id, enum.CommonNo).Updates(map[string]any{"is_del": enum.CommonYes, "status": enum.CommonNo}).Error; err != nil {
			return err
		}
		if err := tx.Model(&KnowledgeChunk{}).Where("knowledge_base_id = ? AND is_del = ?", id, enum.CommonNo).Updates(map[string]any{"is_del": enum.CommonYes, "status": enum.CommonNo}).Error; err != nil {
			return err
		}
		return tx.Model(&AgentKnowledgeBase{}).Where("knowledge_base_id = ? AND is_del = ?", id, enum.CommonNo).Updates(map[string]any{"is_del": enum.CommonYes, "status": enum.CommonNo}).Error
	})
}

func (r *GormRepository) ListDocuments(ctx context.Context, baseID uint64, query DocumentListQuery) ([]KnowledgeDocument, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.activeDocuments(ctx).Where("knowledge_base_id = ?", baseID)
	if query.Title != "" {
		db = db.Where("title LIKE ?", query.Title+"%")
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	var total int64
	if err := db.Model(&KnowledgeDocument{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []KnowledgeDocument
	err := db.Order("id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Find(&rows).Error
	return rows, total, err
}

func (r *GormRepository) GetDocument(ctx context.Context, id uint64) (*KnowledgeDocument, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row KnowledgeDocument
	err := r.activeDocuments(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *GormRepository) CreateDocument(ctx context.Context, row KnowledgeDocument) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) UpdateDocument(ctx context.Context, id uint64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDocuments(ctx).Where("id = ?", id).Updates(fields).Error
}

func (r *GormRepository) ChangeDocumentStatus(ctx context.Context, id uint64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDocuments(ctx).Where("id = ?", id).Update("status", status).Error
}

func (r *GormRepository) DeleteDocument(ctx context.Context, id uint64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&KnowledgeDocument{}).Where("id = ? AND is_del = ?", id, enum.CommonNo).Updates(map[string]any{"is_del": enum.CommonYes, "status": enum.CommonNo}).Error; err != nil {
			return err
		}
		return tx.Model(&KnowledgeChunk{}).Where("document_id = ? AND is_del = ?", id, enum.CommonNo).Updates(map[string]any{"is_del": enum.CommonYes, "status": enum.CommonNo}).Error
	})
}

func (r *GormRepository) ReplaceChunks(ctx context.Context, document KnowledgeDocument, chunks []TextChunk, indexedAt time.Time) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	if indexedAt.IsZero() {
		indexedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&KnowledgeChunk{}).Where("document_id = ? AND is_del = ?", document.ID, enum.CommonNo).Updates(map[string]any{"is_del": enum.CommonYes, "status": enum.CommonNo}).Error; err != nil {
			return err
		}
		for _, chunk := range chunks {
			row := KnowledgeChunk{KnowledgeBaseID: document.KnowledgeBaseID, DocumentID: document.ID, ChunkIndex: chunk.Index, Title: document.Title, Content: chunk.Content, ContentChars: chunk.Chars, Status: enum.CommonYes, IsDel: enum.CommonNo}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return tx.Model(&KnowledgeDocument{}).Where("id = ? AND is_del = ?", document.ID, enum.CommonNo).Updates(map[string]any{"index_status": IndexStatusIndexed, "last_indexed_at": indexedAt, "error_message": ""}).Error
	})
}

func (r *GormRepository) ListChunks(ctx context.Context, documentID uint64) ([]KnowledgeChunk, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []KnowledgeChunk
	err := r.db.WithContext(ctx).Where("document_id = ? AND is_del = ?", documentID, enum.CommonNo).Order("chunk_index ASC, id ASC").Find(&rows).Error
	return rows, err
}

func (r *GormRepository) ListActiveBaseOptions(ctx context.Context) ([]KnowledgeBaseOptionRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []KnowledgeBaseOptionRow
	err := r.db.WithContext(ctx).Model(&KnowledgeBase{}).
		Select("id, name, description, default_top_k, default_min_score, default_max_context_chars").
		Where("is_del = ? AND status = ?", enum.CommonNo, enum.CommonYes).
		Order("id DESC").Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) ListAgentKnowledgeBindings(ctx context.Context, agentID uint64) ([]AgentKnowledgeBindingRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []AgentKnowledgeBindingRow
	err := r.db.WithContext(ctx).Table("ai_agent_knowledge_bases akb").
		Select(`akb.id, akb.agent_id, akb.knowledge_base_id, kb.name AS knowledge_base_name, akb.top_k, akb.min_score, akb.max_context_chars, akb.status, kb.default_top_k, kb.default_min_score, kb.default_max_context_chars`).
		Joins("JOIN ai_knowledge_bases kb ON kb.id = akb.knowledge_base_id AND kb.is_del = ?", enum.CommonNo).
		Where("akb.agent_id = ? AND akb.is_del = ?", agentID, enum.CommonNo).
		Order("akb.knowledge_base_id ASC").Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) ReplaceAgentKnowledgeBindings(ctx context.Context, agentID uint64, rows []AgentKnowledgeBindingInput) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("agent_id = ? AND is_del = ?", agentID, enum.CommonYes).Delete(&AgentKnowledgeBase{}).Error; err != nil {
			return err
		}
		baseIDs := make([]uint64, 0, len(rows))
		for _, input := range rows {
			baseIDs = append(baseIDs, input.KnowledgeBaseID)
		}
		inactiveDB := tx.Model(&AgentKnowledgeBase{}).Where("agent_id = ? AND is_del = ?", agentID, enum.CommonNo)
		if len(baseIDs) > 0 {
			inactiveDB = inactiveDB.Where("knowledge_base_id NOT IN ?", baseIDs)
		}
		if err := inactiveDB.Updates(map[string]any{"is_del": enum.CommonYes, "status": enum.CommonNo}).Error; err != nil {
			return err
		}
		for _, input := range rows {
			row := AgentKnowledgeBase{AgentID: agentID, KnowledgeBaseID: input.KnowledgeBaseID, IsDel: enum.CommonNo}
			fields := AgentKnowledgeBase{TopK: input.TopK, MinScore: bindingMinScore(input), MaxContextChars: input.MaxContextChars, Status: input.Status}
			if err := tx.Where("agent_id = ? AND knowledge_base_id = ? AND is_del = ?", agentID, input.KnowledgeBaseID, enum.CommonNo).Assign(fields).FirstOrCreate(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GormRepository) ListRuntimeBindings(ctx context.Context, agentID uint64) ([]RuntimeBindingRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []RuntimeBindingRow
	err := r.db.WithContext(ctx).Table("ai_agent_knowledge_bases akb").
		Select("akb.knowledge_base_id, kb.name AS knowledge_base_name, akb.top_k, akb.min_score, akb.max_context_chars").
		Joins("JOIN ai_knowledge_bases kb ON kb.id = akb.knowledge_base_id AND kb.is_del = ? AND kb.status = ?", enum.CommonNo, enum.CommonYes).
		Where("akb.agent_id = ? AND akb.is_del = ? AND akb.status = ?", agentID, enum.CommonNo, enum.CommonYes).
		Order("akb.id ASC").Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) ListCandidates(ctx context.Context, baseIDs []uint64, limit int) ([]RetrievalCandidate, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if len(baseIDs) == 0 {
		return nil, nil
	}
	var rows []RetrievalCandidate
	db := r.db.WithContext(ctx).Table("ai_knowledge_chunks c").
		Select(`c.knowledge_base_id, kb.name AS knowledge_base_name, c.document_id, d.title AS document_title, c.id AS chunk_id, c.chunk_index, c.title, c.content, c.content_chars`).
		Joins("JOIN ai_knowledge_bases kb ON kb.id = c.knowledge_base_id AND kb.is_del = ? AND kb.status = ?", enum.CommonNo, enum.CommonYes).
		Joins("JOIN ai_knowledge_documents d ON d.id = c.document_id AND d.is_del = ? AND d.status = ? AND d.index_status = ?", enum.CommonNo, enum.CommonYes, IndexStatusIndexed).
		Where("c.knowledge_base_id IN ? AND c.is_del = ? AND c.status = ?", baseIDs, enum.CommonNo, enum.CommonYes).
		Order("c.id ASC")
	if limit > 0 {
		db = db.Limit(limit)
	}
	err := db.Scan(&rows).Error
	return rows, err
}

func (r *GormRepository) CreateRetrieval(ctx context.Context, input CreateRetrievalInput) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	startedAt := input.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	row := KnowledgeRetrieval{RunID: input.RunID, Query: strings.TrimSpace(input.Query), Status: input.Status, IsDel: enum.CommonNo, CreatedAt: startedAt, UpdatedAt: startedAt}
	if row.Status == "" {
		row.Status = RetrievalStatusSuccess
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *GormRepository) FinishRetrieval(ctx context.Context, input FinishRetrievalInput) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	fields := map[string]any{"status": input.Status, "total_hits": input.TotalHits, "selected_hits": input.SelectedHits, "error_message": strings.TrimSpace(input.ErrorMessage)}
	if input.DurationMS > 0 {
		fields["duration_ms"] = input.DurationMS
	}
	return r.db.WithContext(ctx).Model(&KnowledgeRetrieval{}).Where("id = ? AND is_del = ?", input.ID, enum.CommonNo).Updates(fields).Error
}

func (r *GormRepository) InsertRetrievalHits(ctx context.Context, retrievalID uint64, hits []ScoredHit) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	for _, hit := range hits {
		row := KnowledgeRetrievalHit{RetrievalID: retrievalID, KnowledgeBaseID: hit.KnowledgeBaseID, KnowledgeBaseName: hit.KnowledgeBaseName, DocumentID: hit.DocumentID, DocumentTitle: hit.DocumentTitle, ChunkID: hit.ChunkID, ChunkIndex: hit.ChunkIndex, Score: hit.Score, RankNo: hit.RankNo, ContentSnapshot: hit.Content, Status: hit.Status, SkipReason: hit.SkipReason, IsDel: enum.CommonNo}
		if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *GormRepository) activeBases(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}

func (r *GormRepository) activeDocuments(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
