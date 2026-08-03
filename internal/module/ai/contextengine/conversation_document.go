package contextengine

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/documentparser"
	"admin_back_go/internal/shared/enum"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const AttachmentIngestionPending = "attachment_ingestion_pending"

type ConversationDocumentEnsurer interface {
	EnsureConversationDocuments(context.Context, uint64) error
}

type ConversationAttachmentDiagnostic struct {
	AttachmentIndex uint32
	Code            string
}

type ConversationDocumentRepairRepository interface {
	ListConversationAttachmentMessageIDs(context.Context, uint64, int) ([]uint64, uint64, error)
}

type HistoricalAttachmentAvailability interface {
	HistoricalAttachmentReady(context.Context, uint64, uint64, uint32) (bool, error)
}

type ConversationDocumentService struct {
	db       *gorm.DB
	enqueuer *QueueDocumentVersionEnqueuer
}

func NewConversationDocumentService(client *database.Client, enqueuer *QueueDocumentVersionEnqueuer) *ConversationDocumentService {
	if client == nil {
		return nil
	}
	return &ConversationDocumentService{db: client.Gorm, enqueuer: enqueuer}
}

func conversationDocumentVersionAuthoritative(ctx context.Context, db *gorm.DB, versionID uint64, requireActive bool) (bool, error) {
	if db == nil || versionID == 0 {
		return false, errors.New("conversation document authority is not configured")
	}
	var row struct {
		ConversationID        *uint64 `gorm:"column:conversation_id"`
		SourceMessageID       *uint64 `gorm:"column:source_message_id"`
		SourceAttachmentIndex *uint32 `gorm:"column:source_attachment_index"`
		DocumentStatus        string  `gorm:"column:document_status"`
		ActiveVersionID       *uint64 `gorm:"column:active_version_id"`
		ProfileID             uint64  `gorm:"column:profile_id"`
		SourceStorageProvider string  `gorm:"column:source_storage_provider"`
		SourceObjectKey       string  `gorm:"column:source_object_key"`
		SourceETag            string  `gorm:"column:source_etag"`
		SourceSize            int64   `gorm:"column:source_size_bytes"`
		SourceMIMEType        string  `gorm:"column:source_mime_type"`
		SourceFilename        string  `gorm:"column:source_filename"`
		AgentProfileID        *uint64 `gorm:"column:agent_profile_id"`
		MetaJSON              *string `gorm:"column:meta_json"`
	}
	err := db.WithContext(ctx).Table("ai_context_document_versions AS v").
		Select(`d.conversation_id, d.source_message_id, d.source_attachment_index, d.status AS document_status, d.active_version_id,
			v.profile_id, v.source_storage_provider, v.source_object_key, v.source_etag, v.source_size_bytes, v.source_mime_type, v.source_filename,
			a.context_profile_id AS agent_profile_id, m.meta_json`).
		Joins("JOIN ai_context_documents AS d ON d.id = v.document_id AND d.deleted_at IS NULL").
		Joins("LEFT JOIN ai_conversations AS c ON c.id = d.conversation_id AND c.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN ai_agents AS a ON a.id = c.agent_id AND a.is_del = ?", enum.CommonNo).
		Joins("LEFT JOIN ai_messages AS m ON m.id = d.source_message_id AND m.conversation_id = d.conversation_id AND m.role = ? AND m.is_del = ?", enum.AIMessageRoleUser, enum.CommonNo).
		Where("v.id = ?", versionID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.ConversationID == nil {
		return true, nil
	}
	if row.SourceMessageID == nil || row.SourceAttachmentIndex == nil || row.DocumentStatus != DocumentEnabled ||
		row.AgentProfileID == nil || *row.AgentProfileID != row.ProfileID || row.MetaJSON == nil ||
		(requireActive && (row.ActiveVersionID == nil || *row.ActiveVersionID != versionID)) {
		return false, nil
	}
	attachments, err := decodeConversationAttachments(row.MetaJSON)
	if err != nil || uint64(*row.SourceAttachmentIndex) >= uint64(len(attachments)) {
		return false, err
	}
	attachment := attachments[*row.SourceAttachmentIndex]
	if _, supported := supportedConversationAttachment(documentparser.NewRegistry(), attachment); !supported {
		return false, nil
	}
	return row.SourceStorageProvider == "cos" && row.SourceObjectKey == strings.TrimSpace(attachment.ObjectKey) &&
		row.SourceETag == strings.TrimSpace(attachment.ETag) && row.SourceSize == attachment.Size &&
		row.SourceMIMEType == strings.TrimSpace(attachment.MIMEType) && row.SourceFilename == strings.TrimSpace(attachment.Name), nil
}

type conversationDocumentSource struct {
	MessageID      uint64  `gorm:"column:message_id"`
	ConversationID uint64  `gorm:"column:conversation_id"`
	UserID         uint64  `gorm:"column:user_id"`
	ProfileID      *uint64 `gorm:"column:profile_id"`
	MetaJSON       *string `gorm:"column:meta_json"`
}

type conversationAttachment struct {
	Type      string `json:"type"`
	ObjectKey string `json:"object_key"`
	MIMEType  string `json:"mime_type"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	ETag      string `json:"etag"`
}

type conversationDocumentWork struct {
	Index   uint32
	Source  conversationAttachment
	Parser  documentparser.Parser
	Version ContextDocumentVersion
}

func (service *ConversationDocumentService) EnsureConversationDocuments(ctx context.Context, messageID uint64) error {
	if service == nil || service.db == nil || service.enqueuer == nil || messageID == 0 {
		return errors.New("conversation document service is not configured")
	}
	source, work, err := service.loadConversationDocumentWork(ctx, messageID)
	if err != nil || source.ProfileID == nil || len(work) == 0 {
		return err
	}
	versionIDs := make([]uint64, 0, len(work))
	err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range work {
			versionID, err := ensureConversationDocumentVersion(tx, source, item)
			if err != nil {
				return err
			}
			if versionID != 0 {
				versionIDs = append(versionIDs, versionID)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, versionID := range versionIDs {
		if err := service.enqueuer.EnqueueDocumentVersion(ctx, versionID); err != nil {
			return err
		}
	}
	return nil
}

func (service *ConversationDocumentService) ListConversationAttachmentMessageIDs(ctx context.Context, afterMessageID uint64, limit int) ([]uint64, uint64, error) {
	if service == nil || service.db == nil || limit <= 0 {
		return nil, 0, errors.New("conversation document repair repository is not configured")
	}
	var ids []uint64
	err := service.db.WithContext(ctx).Table("ai_messages AS m").Select("m.id").
		Joins("JOIN ai_conversations AS c ON c.id = m.conversation_id AND c.is_del = ?", enum.CommonNo).
		Joins("JOIN ai_agents AS a ON a.id = c.agent_id AND a.is_del = ? AND a.context_profile_id IS NOT NULL", enum.CommonNo).
		Where("m.id > ? AND m.role = ? AND m.is_del = ? AND m.meta_json IS NOT NULL AND JSON_LENGTH(JSON_EXTRACT(m.meta_json, '$.attachments')) > 0", afterMessageID, enum.AIMessageRoleUser, enum.CommonNo).
		Order("m.id ASC").Limit(limit).Scan(&ids).Error
	if err != nil {
		return nil, afterMessageID, err
	}
	next := uint64(0)
	if len(ids) == limit {
		next = ids[len(ids)-1]
	}
	return ids, next, nil
}

func (service *ConversationDocumentService) ConversationAttachmentDiagnosticsForRun(ctx context.Context, runID uint64) ([]ConversationAttachmentDiagnostic, error) {
	if service == nil || service.db == nil || runID == 0 {
		return nil, errors.New("conversation attachment diagnostics are not configured")
	}
	var row struct {
		MessageID uint64 `gorm:"column:user_message_id"`
	}
	if err := service.db.WithContext(ctx).Table("ai_runs").Select("user_message_id").Where("id = ?", runID).Take(&row).Error; err != nil {
		return nil, err
	}
	return service.ConversationAttachmentDiagnostics(ctx, row.MessageID)
}

func (service *ConversationDocumentService) ConversationAttachmentDiagnostics(ctx context.Context, messageID uint64) ([]ConversationAttachmentDiagnostic, error) {
	_, work, err := service.loadConversationDocumentWork(ctx, messageID)
	if err != nil || len(work) == 0 {
		return nil, err
	}
	diagnostics := make([]ConversationAttachmentDiagnostic, 0)
	for _, item := range work {
		code, err := service.conversationAttachmentDiagnostic(ctx, messageID, item)
		if err != nil {
			return nil, err
		}
		if code != "" {
			diagnostics = append(diagnostics, ConversationAttachmentDiagnostic{AttachmentIndex: item.Index, Code: code})
		}
	}
	return diagnostics, nil
}

func (service *ConversationDocumentService) HistoricalAttachmentReady(ctx context.Context, conversationID, messageID uint64, attachmentIndex uint32) (bool, error) {
	source, work, err := service.loadConversationDocumentWork(ctx, messageID)
	if err != nil || source.ConversationID != conversationID {
		return false, err
	}
	for _, item := range work {
		if item.Index != attachmentIndex {
			continue
		}
		code, err := service.conversationAttachmentDiagnostic(ctx, messageID, item)
		return code == "", err
	}
	return false, nil
}

func (service *ConversationDocumentService) conversationAttachmentDiagnostic(ctx context.Context, messageID uint64, work conversationDocumentWork) (string, error) {
	var row struct {
		DocumentID        uint64  `gorm:"column:document_id"`
		DocumentStatus    string  `gorm:"column:document_status"`
		ActiveVersionID   *uint64 `gorm:"column:active_version_id"`
		VersionID         uint64  `gorm:"column:version_id"`
		SourceFactsSHA256 []byte  `gorm:"column:source_facts_sha256"`
		State             string  `gorm:"column:state"`
		ErrorCode         *string `gorm:"column:error_code"`
	}
	err := service.db.WithContext(ctx).Table("ai_context_documents AS d").
		Select("d.id AS document_id, d.status AS document_status, d.active_version_id, v.id AS version_id, v.source_facts_sha256, v.state, v.error_code").
		Joins("LEFT JOIN ai_context_document_versions AS v ON v.id = (SELECT latest.id FROM ai_context_document_versions AS latest WHERE latest.document_id = d.id ORDER BY latest.id DESC LIMIT 1)").
		Where("d.source_message_id = ? AND d.source_attachment_index = ? AND d.deleted_at IS NULL", messageID, work.Index).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AttachmentIngestionPending, nil
	}
	if err != nil {
		return "", err
	}
	if row.DocumentStatus != DocumentEnabled || row.VersionID == 0 || string(row.SourceFactsSHA256) != string(work.Version.SourceFactsSHA256) {
		return AttachmentIngestionPending, nil
	}
	switch row.State {
	case DocumentVersionReady:
		if row.ActiveVersionID != nil && *row.ActiveVersionID == row.VersionID {
			return "", nil
		}
		return AttachmentIngestionPending, nil
	case DocumentVersionFailed:
		if row.ErrorCode != nil && strings.TrimSpace(*row.ErrorCode) != "" {
			return strings.TrimSpace(*row.ErrorCode), nil
		}
		return AttachmentIngestionPending, nil
	default:
		return AttachmentIngestionPending, nil
	}
}

func (service *ConversationDocumentService) loadConversationDocumentWork(ctx context.Context, messageID uint64) (conversationDocumentSource, []conversationDocumentWork, error) {
	var source conversationDocumentSource
	err := service.db.WithContext(ctx).Table("ai_messages AS m").
		Select("m.id AS message_id, m.conversation_id, m.meta_json, c.user_id, a.context_profile_id AS profile_id").
		Joins("JOIN ai_conversations AS c ON c.id = m.conversation_id AND c.is_del = ?", enum.CommonNo).
		Joins("JOIN ai_agents AS a ON a.id = c.agent_id AND a.is_del = ?", enum.CommonNo).
		Where("m.id = ? AND m.role = ? AND m.is_del = ?", messageID, enum.AIMessageRoleUser, enum.CommonNo).
		Take(&source).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return conversationDocumentSource{}, nil, nil
	}
	if err != nil || source.ProfileID == nil {
		return source, nil, err
	}
	if source.UserID == 0 || source.UserID > math.MaxUint32 || *source.ProfileID == 0 {
		return conversationDocumentSource{}, nil, errors.New("conversation document authority is invalid")
	}
	attachments, err := decodeConversationAttachments(source.MetaJSON)
	if err != nil {
		return conversationDocumentSource{}, nil, err
	}
	registry := documentparser.NewRegistry()
	work := make([]conversationDocumentWork, 0, len(attachments))
	for index, attachment := range attachments {
		if index > math.MaxUint32 {
			return conversationDocumentSource{}, nil, errors.New("conversation attachment index overflows")
		}
		parser, supported := supportedConversationAttachment(registry, attachment)
		if !supported {
			continue
		}
		version := ContextDocumentVersion{
			ProfileID: *source.ProfileID, SourceStorageProvider: "cos", SourceObjectKey: strings.TrimSpace(attachment.ObjectKey),
			SourceETag: strings.TrimSpace(attachment.ETag), SourceSize: attachment.Size, SourceMIMEType: strings.TrimSpace(attachment.MIMEType),
			SourceFilename: strings.TrimSpace(attachment.Name), ParserName: parser.Name(), ParserVersion: parser.Version(),
			ChunkerVersion: ChunkerVersionV1, State: DocumentVersionQueued,
		}
		version.SourceFactsSHA256 = sourceFactsHash(version)
		work = append(work, conversationDocumentWork{Index: uint32(index), Source: attachment, Parser: parser, Version: version})
	}
	return source, work, nil
}

func decodeConversationAttachments(raw *string) ([]conversationAttachment, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	var metadata struct {
		Attachments []conversationAttachment `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(*raw), &metadata); err != nil {
		return nil, err
	}
	return metadata.Attachments, nil
}

func supportedConversationAttachment(registry *documentparser.Registry, attachment conversationAttachment) (documentparser.Parser, bool) {
	if registry == nil || strings.TrimSpace(attachment.Type) != "file" ||
		!strings.HasPrefix(strings.TrimSpace(attachment.ObjectKey), "ai_chat_attachments/") ||
		strings.TrimSpace(attachment.ETag) == "" || attachment.Size <= 0 ||
		strings.TrimSpace(attachment.MIMEType) == "" || strings.TrimSpace(attachment.Name) == "" {
		return nil, false
	}
	parser, err := registry.Resolve(attachment.Name, attachment.MIMEType)
	return parser, err == nil
}

func ensureConversationDocumentVersion(tx *gorm.DB, source conversationDocumentSource, work conversationDocumentWork) (uint64, error) {
	document := ContextDocument{
		ConversationID: &source.ConversationID, SourceMessageID: &source.MessageID, SourceAttachmentIndex: &work.Index,
		Title: strings.TrimSpace(work.Source.Name), Status: DocumentEnabled, CreatedBy: uint32(source.UserID),
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&document).Error; err != nil {
		return 0, err
	}
	if document.ID == 0 {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("conversation_id = ? AND source_message_id = ? AND source_attachment_index = ?", source.ConversationID, source.MessageID, work.Index).
			Take(&document).Error; err != nil {
			return 0, err
		}
	}
	if document.Status != DocumentEnabled {
		if err := tx.Model(&ContextDocument{}).Where("id = ?", document.ID).
			Updates(map[string]any{"status": DocumentEnabled, "deleted_at": nil, "title": strings.TrimSpace(work.Source.Name)}).Error; err != nil {
			return 0, err
		}
	}
	var latest ContextDocumentVersion
	err := tx.Where("document_id = ?", document.ID).Order("id DESC").Take(&latest).Error
	if err == nil && string(latest.SourceFactsSHA256) == string(work.Version.SourceFactsSHA256) {
		if latest.State == DocumentVersionQueued || latest.State == DocumentVersionProcessing {
			return latest.ID, nil
		}
		return 0, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	version := work.Version
	version.DocumentID = document.ID
	if err := tx.Create(&version).Error; err != nil {
		return 0, err
	}
	return version.ID, nil
}
