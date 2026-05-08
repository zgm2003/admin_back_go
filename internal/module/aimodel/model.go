package aimodel

import "time"

type Model struct {
	ID         int64     `gorm:"column:id;primaryKey"`
	Name       string    `gorm:"column:name"`
	Driver     string    `gorm:"column:driver"`
	ModelCode  string    `gorm:"column:model_code"`
	Endpoint   string    `gorm:"column:endpoint"`
	APIKeyEnc  string    `gorm:"column:api_key_enc"`
	APIKeyHint string    `gorm:"column:api_key_hint"`
	Modalities string    `gorm:"column:modalities"`
	Status     int       `gorm:"column:status"`
	IsDel      int       `gorm:"column:is_del"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (Model) TableName() string {
	return "ai_models"
}
