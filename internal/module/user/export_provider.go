package user

import (
	"context"
	"fmt"
	"strconv"

	"admin_back_go/internal/module/exporttask"
)

type ExportDataProvider struct {
	repository Repository
}

func NewExportDataProvider(repository Repository) *ExportDataProvider {
	return &ExportDataProvider{repository: repository}
}

func (p *ExportDataProvider) BuildExportData(ctx context.Context, kind string, ids []int64) (*exporttask.FileData, error) {
	if p == nil || p.repository == nil {
		return nil, ErrRepositoryNotConfigured
	}
	if kind != exporttask.KindUserList {
		return nil, fmt.Errorf("unsupported export kind: %s", kind)
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil, fmt.Errorf("export user ids are required")
	}
	rows, err := p.repository.ExportUsersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	dataRows := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		dataRows = append(dataRows, map[string]string{
			"id":       strconv.FormatInt(row.ID, 10),
			"username": row.Username,
			"email":    row.Email,
			"phone":    row.Phone,
			"avatar":   row.Avatar,
			"sex":      exportSexText(row.Sex),
			"role":     row.RoleName,
		})
	}
	return &exporttask.FileData{
		Prefix: "用户列表导出",
		Headers: []exporttask.Column{
			{Key: "id", Title: "用户ID"},
			{Key: "username", Title: "用户名"},
			{Key: "email", Title: "邮箱"},
			{Key: "phone", Title: "手机号"},
			{Key: "avatar", Title: "头像"},
			{Key: "sex", Title: "性别"},
			{Key: "role", Title: "角色"},
		},
		Rows: dataRows,
	}, nil
}

func exportSexText(value int) string {
	switch value {
	case 1:
		return "男"
	case 2:
		return "女"
	default:
		return "未知"
	}
}
