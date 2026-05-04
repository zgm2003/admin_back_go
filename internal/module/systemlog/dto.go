package systemlog

import "admin_back_go/internal/dict"

type InitResponse struct {
	Dict InitDict `json:"dict"`
}

type InitDict struct {
	LogLevelArr []dict.Option[string] `json:"log_level_arr"`
	LogTailArr  []dict.Option[int]    `json:"log_tail_arr"`
}

type FileItem struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	SizeHuman string `json:"size_human"`
	MTime     string `json:"mtime"`
}

type FilesResponse struct {
	List []FileItem `json:"list"`
}

type LineItem struct {
	Number  int    `json:"number"`
	Level   string `json:"level"`
	Content string `json:"content"`
}

type LinesResponse struct {
	Lines    []LineItem `json:"lines"`
	Total    int        `json:"total"`
	Filename string     `json:"filename"`
}
