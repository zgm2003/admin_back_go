package clientversion

import "time"

type Version struct {
	ID          int64     `gorm:"column:id;primaryKey"`
	Version     string    `gorm:"column:version"`
	Notes       string    `gorm:"column:notes"`
	FileURL     string    `gorm:"column:file_url"`
	Signature   string    `gorm:"column:signature"`
	Platform    string    `gorm:"column:platform"`
	FileSize    int64     `gorm:"column:file_size"`
	IsLatest    int       `gorm:"column:is_latest"`
	ForceUpdate int       `gorm:"column:force_update"`
	IsDel       int       `gorm:"column:is_del"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (Version) TableName() string {
	return "client_versions"
}
