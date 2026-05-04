package systemsetting

import "time"

type Setting struct {
	ID           int64     `gorm:"column:id"`
	SettingKey   string    `gorm:"column:setting_key"`
	SettingValue string    `gorm:"column:setting_value"`
	ValueType    int       `gorm:"column:value_type"`
	Remark       string    `gorm:"column:remark"`
	Status       int       `gorm:"column:status"`
	IsDel        int       `gorm:"column:is_del"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (Setting) TableName() string {
	return "system_settings"
}
