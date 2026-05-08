package aiagent

import (
	"context"
	"errors"
	"strings"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"gorm.io/gorm"
)

var ErrRepositoryNotConfigured = errors.New("aiagent repository not configured")

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(client *database.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	return &GormRepository{db: client.Gorm}
}

func (r *GormRepository) InitModels(ctx context.Context) ([]ModelOptionRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []ModelOptionRow
	err := r.db.WithContext(ctx).Table("ai_models").Select("id, name, driver").Where("is_del = ?", enum.CommonNo).Where("status = ?", enum.CommonYes).Order("id DESC").Scan(&rows).Error
	return rows, err
}
func (r *GormRepository) InitKnowledgeBases(ctx context.Context) ([]OptionRow, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var rows []OptionRow
	err := r.db.WithContext(ctx).Table("ai_knowledge_bases").Select("id, name").Where("is_del = ?", enum.CommonNo).Where("status = ?", enum.CommonYes).Order("id DESC").Scan(&rows).Error
	return rows, err
}
func (r *GormRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrRepositoryNotConfigured
	}
	db := r.db.WithContext(ctx).Table("ai_agents as a").Where("a.is_del = ?", enum.CommonNo)
	if strings.TrimSpace(query.Name) != "" {
		db = db.Where("a.name LIKE ?", strings.TrimSpace(query.Name)+"%")
	}
	if query.ModelID != nil {
		db = db.Where("a.model_id = ?", *query.ModelID)
	}
	if strings.TrimSpace(query.Mode) != "" {
		db = db.Where("a.mode = ?", strings.TrimSpace(query.Mode))
	}
	if query.Status != nil {
		db = db.Where("a.status = ?", *query.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var flats []listRowFlat
	err := db.Select("a.id, a.name, a.model_id, a.avatar, a.system_prompt, a.mode, a.scene, a.capabilities_json, a.runtime_config_json, a.policy_json, a.status, a.is_del, a.created_at, a.updated_at, m.name as model_name, m.driver, m.model_code, IF(m.is_del = 1, true, false) as model_deleted").Joins("LEFT JOIN ai_models m ON m.id = a.model_id").Order("a.id DESC").Limit(query.PageSize).Offset((query.CurrentPage - 1) * query.PageSize).Scan(&flats).Error
	if err != nil {
		return nil, 0, err
	}
	rows := make([]ListRow, 0, len(flats))
	for _, f := range flats {
		rows = append(rows, f.toListRow())
	}
	return rows, total, nil
}
func (r *GormRepository) Get(ctx context.Context, id int64) (*Agent, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if id <= 0 {
		return nil, nil
	}
	var row Agent
	err := r.db.WithContext(ctx).Where("is_del = ?", enum.CommonNo).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
func (r *GormRepository) ActiveModelExists(ctx context.Context, id int64) (bool, error) {
	return r.activeIDExists(ctx, "ai_models", id)
}
func (r *GormRepository) ActiveToolIDs(ctx context.Context, ids []int64) (map[int64]struct{}, error) {
	return r.activeIDSet(ctx, "ai_tools", ids)
}
func (r *GormRepository) ActiveKnowledgeBaseIDs(ctx context.Context, ids []int64) (map[int64]struct{}, error) {
	return r.activeIDSet(ctx, "ai_knowledge_bases", ids)
}
func (r *GormRepository) BindingData(ctx context.Context, agentIDs []int64) (BindingData, error) {
	data := BindingData{SceneCodes: map[int64][]string{}, ToolIDs: map[int64][]int64{}, KnowledgeBaseIDs: map[int64][]int64{}, KnowledgeBaseNames: map[int64][]string{}}
	if r == nil || r.db == nil {
		return data, ErrRepositoryNotConfigured
	}
	ids := normalizeIDs(agentIDs)
	if len(ids) == 0 {
		return data, nil
	}
	var scenes []struct {
		AgentID   int64
		SceneCode string
	}
	if err := r.db.WithContext(ctx).Table("ai_agent_scenes").Select("agent_id, scene_code").Where("agent_id IN ?", ids).Where("is_del = ?", enum.CommonNo).Where("status = ?", enum.CommonYes).Order("id ASC").Scan(&scenes).Error; err != nil {
		return data, err
	}
	for _, row := range scenes {
		data.SceneCodes[row.AgentID] = append(data.SceneCodes[row.AgentID], row.SceneCode)
	}
	var tools []struct {
		AssistantID int64
		ToolID      int64
	}
	if err := r.db.WithContext(ctx).Table("ai_assistant_tools").Select("assistant_id, tool_id").Where("assistant_id IN ?", ids).Where("is_del = ?", enum.CommonNo).Where("status = ?", enum.CommonYes).Order("tool_id ASC").Scan(&tools).Error; err != nil {
		return data, err
	}
	for _, row := range tools {
		data.ToolIDs[row.AssistantID] = append(data.ToolIDs[row.AssistantID], row.ToolID)
	}
	var kbs []struct {
		AgentID         int64
		KnowledgeBaseID int64
		Name            string
	}
	if err := r.db.WithContext(ctx).Table("ai_agent_knowledge_bases akb").Select("akb.agent_id, akb.knowledge_base_id, kb.name").Joins("JOIN ai_knowledge_bases kb ON kb.id = akb.knowledge_base_id AND kb.is_del = ?", enum.CommonNo).Where("akb.agent_id IN ?", ids).Where("akb.is_del = ?", enum.CommonNo).Where("akb.status = ?", enum.CommonYes).Order("akb.knowledge_base_id ASC").Scan(&kbs).Error; err != nil {
		return data, err
	}
	for _, row := range kbs {
		data.KnowledgeBaseIDs[row.AgentID] = append(data.KnowledgeBaseIDs[row.AgentID], row.KnowledgeBaseID)
		data.KnowledgeBaseNames[row.AgentID] = append(data.KnowledgeBaseNames[row.AgentID], row.Name)
	}
	return data, nil
}
func (r *GormRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&GormRepository{db: tx}) })
}
func (r *GormRepository) CreateAgent(ctx context.Context, row Agent) (int64, error) {
	if r == nil || r.db == nil {
		return 0, ErrRepositoryNotConfigured
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}
func (r *GormRepository) UpdateAgent(ctx context.Context, id int64, fields map[string]any) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Agent{}).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).Updates(fields).Error
}
func (r *GormRepository) ChangeStatus(ctx context.Context, id int64, status int) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Model(&Agent{}).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).Update("status", status).Error
}
func (r *GormRepository) SoftDeleteAgentAndBindings(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Agent{}).Where("id = ?", id).Where("is_del = ?", enum.CommonNo).Update("is_del", enum.CommonYes).Error; err != nil {
			return err
		}
		for _, table := range []string{"ai_assistant_tools", "ai_agent_knowledge_bases", "ai_agent_scenes"} {
			column := "agent_id"
			if table == "ai_assistant_tools" {
				column = "assistant_id"
			}
			if err := tx.Table(table).Where(column+" = ?", id).Where("is_del = ?", enum.CommonNo).Updates(map[string]any{"is_del": enum.CommonYes, "status": enum.CommonNo}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *GormRepository) SyncToolBindings(ctx context.Context, agentID int64, toolIDs []int64) error {
	return r.syncBindings(ctx, "ai_assistant_tools", "assistant_id", "tool_id", agentID, toolIDs)
}
func (r *GormRepository) SyncKnowledgeBindings(ctx context.Context, agentID int64, knowledgeIDs []int64) error {
	return r.syncBindings(ctx, "ai_agent_knowledge_bases", "agent_id", "knowledge_base_id", agentID, knowledgeIDs)
}
func (r *GormRepository) SyncSceneBindings(ctx context.Context, agentID int64, sceneCodes []string) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	var existing []AgentScene
	if err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).Find(&existing).Error; err != nil {
		return err
	}
	next := map[string]struct{}{}
	for _, code := range sceneCodes {
		next[code] = struct{}{}
	}
	for _, row := range existing {
		if _, ok := next[row.SceneCode]; ok {
			if row.IsDel != enum.CommonNo || row.Status != enum.CommonYes {
				if err := r.db.WithContext(ctx).Model(&AgentScene{}).Where("id = ?", row.ID).Updates(map[string]any{"is_del": enum.CommonNo, "status": enum.CommonYes}).Error; err != nil {
					return err
				}
			}
			delete(next, row.SceneCode)
			continue
		}
		if row.IsDel == enum.CommonNo {
			if err := r.db.WithContext(ctx).Model(&AgentScene{}).Where("id = ?", row.ID).Updates(map[string]any{"is_del": enum.CommonYes, "status": enum.CommonNo}).Error; err != nil {
				return err
			}
		}
	}
	for code := range next {
		if err := r.db.WithContext(ctx).Create(&AgentScene{AgentID: agentID, SceneCode: code, Status: enum.CommonYes, IsDel: enum.CommonNo}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *GormRepository) activeIDExists(ctx context.Context, table string, id int64) (bool, error) {
	ids, err := r.activeIDSet(ctx, table, []int64{id})
	if err != nil {
		return false, err
	}
	_, ok := ids[id]
	return ok, nil
}
func (r *GormRepository) activeIDSet(ctx context.Context, table string, ids []int64) (map[int64]struct{}, error) {
	out := map[int64]struct{}{}
	if len(ids) == 0 {
		return out, nil
	}
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	var found []int64
	if err := r.db.WithContext(ctx).Table(table).Where("id IN ?", ids).Where("is_del = ?", enum.CommonNo).Where("status = ?", enum.CommonYes).Pluck("id", &found).Error; err != nil {
		return nil, err
	}
	for _, id := range found {
		out[id] = struct{}{}
	}
	return out, nil
}
func (r *GormRepository) syncBindings(ctx context.Context, table string, ownerColumn string, targetColumn string, ownerID int64, targetIDs []int64) error {
	if r == nil || r.db == nil {
		return ErrRepositoryNotConfigured
	}
	type binding struct {
		ID       int64
		TargetID int64
		IsDel    int
		Status   int
	}
	var existing []binding
	if err := r.db.WithContext(ctx).Table(table).Select("id, "+targetColumn+" as target_id, is_del, status").Where(ownerColumn+" = ?", ownerID).Scan(&existing).Error; err != nil {
		return err
	}
	next := map[int64]struct{}{}
	for _, id := range targetIDs {
		next[id] = struct{}{}
	}
	for _, row := range existing {
		if _, ok := next[row.TargetID]; ok {
			if row.IsDel != enum.CommonNo || row.Status != enum.CommonYes {
				if err := r.db.WithContext(ctx).Table(table).Where("id = ?", row.ID).Updates(map[string]any{"is_del": enum.CommonNo, "status": enum.CommonYes}).Error; err != nil {
					return err
				}
			}
			delete(next, row.TargetID)
			continue
		}
		if row.IsDel == enum.CommonNo {
			if err := r.db.WithContext(ctx).Table(table).Where("id = ?", row.ID).Updates(map[string]any{"is_del": enum.CommonYes, "status": enum.CommonNo}).Error; err != nil {
				return err
			}
		}
	}
	for id := range next {
		if err := r.db.WithContext(ctx).Table(table).Create(map[string]any{ownerColumn: ownerID, targetColumn: id, "status": enum.CommonYes, "is_del": enum.CommonNo}).Error; err != nil {
			return err
		}
	}
	return nil
}

type listRowFlat struct {
	ID                int64
	Name              string
	ModelID           int64
	Avatar            *string
	SystemPrompt      *string
	Mode              string
	Scene             *string
	CapabilitiesJSON  *string
	RuntimeConfigJSON *string
	PolicyJSON        *string
	Status            int
	IsDel             int
	CreatedAt         string
	UpdatedAt         string
	ModelName         string
	Driver            string
	ModelCode         string
	ModelDeleted      bool
}

func (f listRowFlat) toListRow() ListRow {
	return ListRow{Agent: Agent{ID: f.ID, Name: f.Name, ModelID: f.ModelID, Avatar: f.Avatar, SystemPrompt: f.SystemPrompt, Mode: f.Mode, Scene: f.Scene, CapabilitiesJSON: f.CapabilitiesJSON, RuntimeConfigJSON: f.RuntimeConfigJSON, PolicyJSON: f.PolicyJSON, Status: f.Status, IsDel: f.IsDel}, ModelName: f.ModelName, Driver: f.Driver, ModelCode: f.ModelCode, ModelDeleted: f.ModelDeleted}
}
