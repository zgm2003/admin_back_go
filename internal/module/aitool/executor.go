package aitool

import (
	"context"
	"encoding/json"
	"fmt"
)

func DefaultExecutors(repo Repository) map[string]Executor {
	return map[string]Executor{
		"admin_user_count": NewAdminUserCountExecutor(repo),
	}
}

type AdminUserCountExecutor struct{ repo Repository }

func NewAdminUserCountExecutor(repo Repository) AdminUserCountExecutor {
	return AdminUserCountExecutor{repo: repo}
}

func (e AdminUserCountExecutor) Execute(ctx context.Context, arguments json.RawMessage) (map[string]any, error) {
	if e.repo == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if !emptyObject(arguments) {
		return nil, fmt.Errorf("admin_user_count 不接受参数")
	}
	count, err := e.repo.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"total_users":    count.TotalUsers,
		"enabled_users":  count.EnabledUsers,
		"disabled_users": count.DisabledUsers,
	}, nil
}

func emptyObject(raw json.RawMessage) bool {
	trimmed := compactJSON(raw)
	if trimmed == "{}" {
		return true
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		return false
	}
	return len(value) == 0
}
