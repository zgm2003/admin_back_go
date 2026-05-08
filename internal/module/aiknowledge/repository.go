package aiknowledge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

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
func (r *GormRepository) Init(ctx context.Context) (*InitResponse, error) { return nil, nil }
func (r *GormRepository) List(ctx context.Context, q ListQuery) ([]KnowledgeBase, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.activeKB(ctx)
	if strings.TrimSpace(q.Name) != "" {
		db = db.Where("name LIKE ?", "%"+strings.TrimSpace(q.Name)+"%")
	}
	if strings.TrimSpace(q.Visibility) != "" {
		db = db.Where("visibility = ?", strings.TrimSpace(q.Visibility))
	}
	if q.Status != nil {
		db = db.Where("status = ?", *q.Status)
	}
	var total int64
	if err := db.Model(&KnowledgeBase{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []KnowledgeBase
	err := db.Order("id DESC").Limit(q.PageSize).Offset((q.CurrentPage - 1) * q.PageSize).Find(&rows).Error
	return rows, total, err
}
func (r *GormRepository) Get(ctx context.Context, id int64) (*KnowledgeBase, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row KnowledgeBase
	err := r.activeKB(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
func (r *GormRepository) Create(ctx context.Context, row KnowledgeBase) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}
func (r *GormRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeKB(ctx).Where("id = ?", id).Updates(fields).Error
}
func (r *GormRepository) ChangeStatus(ctx context.Context, id int64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeKB(ctx).Where("id = ?", id).Update("status", status).Error
}
func (r *GormRepository) Delete(ctx context.Context, ids []int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	returnRows := int64(0)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&KnowledgeBase{}).Where("id IN ?", ids).Where("is_del = ?", enum.CommonNo).Update("is_del", enum.CommonYes)
		if res.Error != nil {
			return res.Error
		}
		returnRows = res.RowsAffected
		if err := tx.Model(&Document{}).Where("knowledge_base_id IN ?", ids).Where("is_del = ?", enum.CommonNo).Update("is_del", enum.CommonYes).Error; err != nil {
			return err
		}
		if err := tx.Model(&Chunk{}).Where("knowledge_base_id IN ?", ids).Where("is_del = ?", enum.CommonNo).Update("is_del", enum.CommonYes).Error; err != nil {
			return err
		}
		return tx.Table("ai_agent_knowledge_bases").Where("knowledge_base_id IN ?", ids).Where("is_del = ?", enum.CommonNo).Updates(map[string]any{"is_del": enum.CommonYes, "status": enum.CommonNo}).Error
	})
	return returnRows, err
}
func (r *GormRepository) ListDocuments(ctx context.Context, q DocumentListQuery) ([]Document, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.activeDoc(ctx).Where("knowledge_base_id = ?", q.KnowledgeBaseID)
	if strings.TrimSpace(q.Title) != "" {
		db = db.Where("title LIKE ?", "%"+strings.TrimSpace(q.Title)+"%")
	}
	if q.Status != nil {
		db = db.Where("status = ?", *q.Status)
	}
	var total int64
	if err := db.Model(&Document{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Document
	err := db.Order("id DESC").Limit(q.PageSize).Offset((q.CurrentPage - 1) * q.PageSize).Find(&rows).Error
	return rows, total, err
}
func (r *GormRepository) GetDocument(ctx context.Context, id, kbID int64) (*Document, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var row Document
	err := r.activeDoc(ctx).Where("id = ?", id).Where("knowledge_base_id = ?", kbID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
func (r *GormRepository) CreateDocument(ctx context.Context, row Document) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}
func (r *GormRepository) UpdateDocument(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDoc(ctx).Where("id = ?", id).Updates(fields).Error
}
func (r *GormRepository) DeleteDocument(ctx context.Context, id, kbID int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Document{}).Where("id = ?", id).Where("knowledge_base_id = ?", kbID).Where("is_del = ?", enum.CommonNo).Update("is_del", enum.CommonYes).Error; err != nil {
			return err
		}
		return tx.Model(&Chunk{}).Where("document_id = ?", id).Where("knowledge_base_id = ?", kbID).Where("is_del = ?", enum.CommonNo).Update("is_del", enum.CommonYes).Error
	})
}
func (r *GormRepository) ListChunks(ctx context.Context, q ChunkListQuery) ([]Chunk, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.activeChunk(ctx).Where("knowledge_base_id = ?", q.KnowledgeBaseID)
	if q.DocumentID != nil {
		db = db.Where("document_id = ?", *q.DocumentID)
	}
	var total int64
	if err := db.Model(&Chunk{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Chunk
	err := db.Order("document_id DESC").Order("chunk_no ASC").Limit(q.PageSize).Offset((q.CurrentPage - 1) * q.PageSize).Find(&rows).Error
	return rows, total, err
}
func (r *GormRepository) ReplaceDocumentChunks(ctx context.Context, kbID, docID int64, chunks []ChunkPayload) (int, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.activeChunk(ctx).Where("knowledge_base_id = ?", kbID).Where("document_id = ?", docID).Update("is_del", enum.CommonYes).Error; err != nil {
		return 0, err
	}
	rows := make([]Chunk, 0, len(chunks))
	for i, ch := range chunks {
		meta := jsonString(ch.MetadataJSON)
		rows = append(rows, Chunk{KnowledgeBaseID: kbID, DocumentID: docID, ChunkNo: i + 1, Content: ch.Content, TokenEstimate: ch.TokenEstimate, MetadataJSON: meta, Status: enum.CommonYes, IsDel: enum.CommonNo})
	}
	if len(rows) == 0 {
		return 0, nil
	}
	if err := r.db.WithContext(ctx).Create(&rows).Error; err != nil {
		return 0, err
	}
	return len(rows), nil
}
func (r *GormRepository) UpdateDocumentChunkStatus(ctx context.Context, id int64, count, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.activeDoc(ctx).Where("id = ?", id).Updates(map[string]any{"chunk_count": count, "index_status": status}).Error
}
func (r *GormRepository) CandidateChunks(ctx context.Context, kbID int64, terms []string, limit int) ([]RetrievalChunk, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = 300
	}
	db := r.db.WithContext(ctx).Table("ai_knowledge_chunks as c").Joins("JOIN ai_knowledge_documents d ON d.id = c.document_id").Where("c.knowledge_base_id = ?", kbID).Where("c.is_del = ?", enum.CommonNo).Where("c.status = ?", enum.CommonYes).Where("d.is_del = ?", enum.CommonNo).Where("d.status = ?", enum.CommonYes)
	terms = terms[:min(len(terms), 8)]
	if len(terms) > 0 {
		db = db.Where(func(tx *gorm.DB) *gorm.DB {
			for _, term := range terms {
				tx = tx.Or("c.content LIKE ?", "%"+strings.ReplaceAll(strings.ReplaceAll(term, "%", "\\%"), "_", "\\_")+"%")
			}
			return tx
		}(r.db.WithContext(ctx)))
	}
	var rows []RetrievalChunk
	err := db.Select("c.knowledge_base_id, c.document_id, d.title as document_title, c.chunk_no, c.content").Order("c.id DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}
func (r *GormRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&GormRepository{db: tx}) })
}
func (r *GormRepository) activeKB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
func (r *GormRepository) activeDoc(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
func (r *GormRepository) activeChunk(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo)
}
func jsonString(v map[string]any) *string {
	if len(v) == 0 {
		return nil
	}
	b, _ := json.Marshal(v)
	s := string(b)
	return &s
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
