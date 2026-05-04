package logstore

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const timeLayout = "2006-01-02 15:04:05"

var (
	ErrInvalidRoot      = errors.New("logstore: invalid root")
	ErrInvalidFilename  = errors.New("logstore: invalid filename")
	ErrFileNotFound     = errors.New("logstore: file not found")
	ErrExtensionDenied  = errors.New("logstore: extension not allowed")
	ErrContextCancelled = errors.New("logstore: context cancelled")
)

type Options struct {
	AllowedExtensions []string
	MaxTailLines      int
}

type Store struct {
	root              string
	allowedExtensions map[string]struct{}
	maxTailLines      int
}

type FileItem struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	SizeHuman string `json:"size_human"`
	MTime     string `json:"mtime"`
}

type TailQuery struct {
	Name    string
	Lines   int
	Level   string
	Keyword string
}

type TailResponse struct {
	Filename string     `json:"filename"`
	Total    int        `json:"total"`
	Lines    []LineItem `json:"lines"`
}

type LineItem struct {
	Number  int    `json:"number"`
	Level   string `json:"level"`
	Content string `json:"content"`
}

func New(root string, options Options) *Store {
	allowed := normalizeExtensions(options.AllowedExtensions)
	maxTailLines := options.MaxTailLines
	if maxTailLines <= 0 {
		maxTailLines = 2000
	}
	return &Store{
		root:              filepath.Clean(root),
		allowedExtensions: allowed,
		maxTailLines:      maxTailLines,
	}
}

func (s *Store) ListFiles(ctx context.Context) ([]FileItem, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	root, err := s.cleanRoot()
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []FileItem{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrInvalidRoot
	}

	items := make([]FileItem, 0)
	rootFiles, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range rootFiles {
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		if entry.Type().IsRegular() {
			item, ok, err := s.fileItem(root, entry.Name())
			if err != nil {
				return nil, err
			}
			if ok {
				items = append(items, item)
			}
			continue
		}
		if !entry.IsDir() {
			continue
		}
		children, err := os.ReadDir(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if err := ctxErr(ctx); err != nil {
				return nil, err
			}
			if !child.Type().IsRegular() {
				continue
			}
			name := filepath.ToSlash(filepath.Join(entry.Name(), child.Name()))
			item, ok, err := s.fileItem(root, name)
			if err != nil {
				return nil, err
			}
			if ok {
				items = append(items, item)
			}
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s *Store) Tail(ctx context.Context, query TailQuery) (*TailResponse, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	root, err := s.cleanRoot()
	if err != nil {
		return nil, err
	}
	name, err := s.cleanName(query.Name)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, filepath.FromSlash(name))
	if !isWithinRoot(root, path) {
		return nil, ErrInvalidFilename
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, ErrFileNotFound
	}

	limit := query.Lines
	if limit <= 0 {
		limit = 500
	}
	if limit > s.maxTailLines {
		limit = s.maxTailLines
	}

	lines, err := tailLines(ctx, path, limit)
	if err != nil {
		return nil, err
	}
	level := strings.ToUpper(strings.TrimSpace(query.Level))
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	items := make([]LineItem, 0, len(lines))
	for _, line := range lines {
		lineLevel := detectLevel(line.Content)
		if level != "" && lineLevel != level {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(line.Content), keyword) {
			continue
		}
		items = append(items, LineItem{Number: line.Number, Level: lineLevel, Content: line.Content})
	}
	return &TailResponse{Filename: name, Total: len(items), Lines: items}, nil
}

func (s *Store) fileItem(root string, name string) (FileItem, bool, error) {
	clean, err := s.cleanName(name)
	if err != nil {
		return FileItem{}, false, nil
	}
	path := filepath.Join(root, filepath.FromSlash(clean))
	info, err := os.Stat(path)
	if err != nil {
		return FileItem{}, false, err
	}
	return FileItem{
		Name:      clean,
		Size:      info.Size(),
		SizeHuman: FormatSize(info.Size()),
		MTime:     info.ModTime().Format(timeLayout),
	}, true, nil
}

func (s *Store) cleanRoot() (string, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return "", ErrInvalidRoot
	}
	abs, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func (s *Store) cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") {
		return "", ErrInvalidFilename
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", ErrInvalidFilename
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
	if clean == "." || clean == "" || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", ErrInvalidFilename
	}
	if strings.Count(clean, "/") > 1 {
		return "", ErrInvalidFilename
	}
	if !s.extensionAllowed(filepath.Ext(clean)) {
		return "", ErrExtensionDenied
	}
	return clean, nil
}

func (s *Store) extensionAllowed(ext string) bool {
	if len(s.allowedExtensions) == 0 {
		return true
	}
	_, ok := s.allowedExtensions[strings.ToLower(ext)]
	return ok
}

type numberedLine struct {
	Number  int
	Content string
}

func tailLines(ctx context.Context, path string, limit int) ([]numberedLine, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	queue := make([]numberedLine, 0, limit)
	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		lineNumber++
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		queue = append(queue, numberedLine{Number: lineNumber, Content: scanner.Text()})
		if len(queue) > limit {
			copy(queue[0:], queue[1:])
			queue = queue[:limit]
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return queue, nil
}

func detectLevel(line string) string {
	upper := strings.ToUpper(line)
	levels := []string{"CRITICAL", "ERROR", "WARNING", "WARN", "INFO", "DEBUG"}
	for _, level := range levels {
		if strings.Contains(upper, "."+level+":") || strings.HasPrefix(upper, level+" ") || strings.Contains(upper, `"LEVEL":"`+level+`"`) || strings.Contains(upper, `"level":"`+level+`"`) {
			if level == "WARN" {
				return "WARNING"
			}
			return level
		}
	}
	return ""
}

func FormatSize(bytes int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value = value / 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.2f %s", value, units[unit])
}

func normalizeExtensions(values []string) map[string]struct{} {
	if len(values) == 0 {
		values = []string{".log"}
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if !strings.HasPrefix(value, ".") {
			value = "." + value
		}
		result[value] = struct{}{}
	}
	return result
}

func isWithinRoot(root string, path string) bool {
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", ErrContextCancelled, ctx.Err())
	default:
		return nil
	}
}
