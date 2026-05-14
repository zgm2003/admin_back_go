package systemlog

import (
	"context"
	"errors"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/platform/logstore"
)

type Store interface {
	ListFiles(ctx context.Context) ([]logstore.FileItem, error)
	Tail(ctx context.Context, query logstore.TailQuery) (*logstore.TailResponse, error)
}

type LinesQuery struct {
	Filename string
	Tail     int
	Level    string
	Keyword  string
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{LogLevelArr: dict.LogLevelOptions(), LogTailArr: dict.LogTailOptions()}}, nil
}

func (s *Service) Files(ctx context.Context) (*FilesResponse, *apperror.Error) {
	if s == nil || s.store == nil {
		return nil, apperror.InternalKey("systemlog.service_missing", nil, "系统日志服务未配置")
	}
	files, err := s.store.ListFiles(ctx)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "systemlog.files_read_failed", nil, "读取日志文件列表失败", err)
	}
	items := make([]FileItem, 0, len(files))
	for _, file := range files {
		items = append(items, FileItem{Name: file.Name, Size: file.Size, SizeHuman: file.SizeHuman, MTime: file.MTime})
	}
	return &FilesResponse{List: items}, nil
}

func (s *Service) Lines(ctx context.Context, query LinesQuery) (*LinesResponse, *apperror.Error) {
	if s == nil || s.store == nil {
		return nil, apperror.InternalKey("systemlog.service_missing", nil, "系统日志服务未配置")
	}
	result, err := s.store.Tail(ctx, logstore.TailQuery{Name: query.Filename, Lines: query.Tail, Level: query.Level, Keyword: query.Keyword})
	if err != nil {
		return nil, mapLogstoreError(err)
	}
	items := make([]LineItem, 0, len(result.Lines))
	for _, line := range result.Lines {
		items = append(items, LineItem{Number: line.Number, Level: line.Level, Content: line.Content})
	}
	return &LinesResponse{Filename: result.Filename, Total: result.Total, Lines: items}, nil
}

func mapLogstoreError(err error) *apperror.Error {
	switch {
	case errors.Is(err, logstore.ErrInvalidFilename), errors.Is(err, logstore.ErrExtensionDenied):
		return apperror.BadRequestKey("systemlog.filename.invalid", nil, "日志文件名不合法")
	case errors.Is(err, logstore.ErrFileNotFound):
		return apperror.NotFoundKey("systemlog.file_not_found", nil, "日志文件不存在")
	default:
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemlog.lines_read_failed", nil, "读取日志内容失败", err)
	}
}
