package authplatform

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"admin_back_go/internal/module/session"
)

const commonYes = 1

type Platform struct {
	ID            int64  `gorm:"column:id"`
	Code          string `gorm:"column:code"`
	Name          string `gorm:"column:name"`
	LoginTypes    string `gorm:"column:login_types"`
	BindPlatform  int    `gorm:"column:bind_platform"`
	BindDevice    int    `gorm:"column:bind_device"`
	BindIP        int    `gorm:"column:bind_ip"`
	SingleSession int    `gorm:"column:single_session"`
	MaxSessions   int    `gorm:"column:max_sessions"`
	AllowRegister int    `gorm:"column:allow_register"`
	AccessTTL     int    `gorm:"column:access_ttl"`
	RefreshTTL    int    `gorm:"column:refresh_ttl"`
	Status        int    `gorm:"column:status"`
	IsDel         int    `gorm:"column:is_del"`
}

func (Platform) TableName() string {
	return "auth_platforms"
}

type Repository interface {
	FindActiveByCode(ctx context.Context, code string) (*Platform, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Policy(ctx context.Context, platform string) (*session.AuthPolicy, error) {
	if s == nil || s.repository == nil {
		return nil, ErrRepositoryNotConfigured
	}

	code := strings.TrimSpace(platform)
	if code == "" {
		return nil, nil
	}

	row, err := s.repository.FindActiveByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}

	return &session.AuthPolicy{
		BindPlatform:             row.BindPlatform == commonYes,
		BindDevice:               row.BindDevice == commonYes,
		BindIP:                   row.BindIP == commonYes,
		SingleSessionPerPlatform: row.SingleSession == commonYes,
		MaxSessions:              row.MaxSessions,
		AllowRegister:            row.AllowRegister == commonYes,
		AccessTTL:                time.Duration(row.AccessTTL) * time.Second,
		RefreshTTL:               time.Duration(row.RefreshTTL) * time.Second,
	}, nil
}

func (s *Service) LoginTypes(ctx context.Context, platform string) ([]string, error) {
	if s == nil || s.repository == nil {
		return nil, ErrRepositoryNotConfigured
	}

	code := strings.TrimSpace(platform)
	if code == "" {
		return nil, nil
	}

	row, err := s.repository.FindActiveByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	return normalizeLoginTypes(row.LoginTypes), nil
}

func normalizeLoginTypes(raw string) []string {
	var decoded []string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return []string{}
	}
	allowed := make(map[string]struct{}, len(decoded))
	for _, value := range decoded {
		value = strings.TrimSpace(value)
		if value != "" {
			allowed[value] = struct{}{}
		}
	}

	ordered := []string{"email", "phone", "password"}
	result := make([]string, 0, len(ordered))
	for _, value := range ordered {
		if _, ok := allowed[value]; ok {
			result = append(result, value)
		}
	}
	return result
}
