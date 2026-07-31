package enum

import (
	"fmt"
	"strings"
)

const (
	UploadDriverCOS = "cos"
)

var UploadDrivers = []string{
	UploadDriverCOS,
}

var UploadDriverLabels = map[string]string{
	UploadDriverCOS: "腾讯云 COS",
}

var UploadImageExts = []string{
	"jpeg", "jpg", "jfif", "pjpeg", "png", "gif", "webp", "bmp",
	"tif", "tiff", "svg", "ico", "psd", "avif",
}

var UploadFileExts = []string{
	"pdf", "doc", "docx", "dot", "odt", "rtf", "ppt", "pptx", "pot", "ppa", "pps", "pwz", "wiz",
	"xla", "xlb", "xlc", "xlm", "xls", "xlsx", "xlt", "xlw", "csv", "tsv", "iif",
	"txt", "text", "md", "markdown", "json", "html", "htm", "xml", "css",
	"asm", "bat", "c", "cc", "cpp", "cxx", "h", "hh", "def", "in",
	"js", "mjs", "jsx", "ts", "tsx", "py", "go", "java", "cs", "php", "rb", "rs",
	"sh", "bash", "zsh", "ksh", "ps1", "sql", "pl", "lua", "r", "scala", "swift", "kt", "kts",
	"yaml", "yml", "toml", "ini", "conf", "properties", "proto",
	"eml", "log", "rst", "srt", "vtt", "ics", "ifb", "vcf", "diff", "patch", "zip", "tar",
}

var UploadFolders = []string{
	"avatars",
	"images",
	"videos",
	"cover_images",
	"ai-agents",
	"ai_chat_images",
	"ai_chat_attachments",
	"exports",
	"reconcile_reports",
}

func IsUploadDriver(value string) bool {
	return containsString(UploadDrivers, strings.TrimSpace(value))
}

func IsUploadImageExt(value string) bool {
	return containsString(UploadImageExts, strings.ToLower(strings.TrimSpace(value)))
}

func IsUploadFileExt(value string) bool {
	return containsString(UploadFileExts, strings.ToLower(strings.TrimSpace(value)))
}

func IsUploadFolder(value string) bool {
	return containsString(UploadFolders, strings.TrimSpace(value))
}

func NormalizeUploadExts(values []string, allowed func(string) bool, ordered []string) ([]string, error) {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if !allowed(normalized) {
			return nil, fmt.Errorf("unsupported upload extension %q", value)
		}
		seen[normalized] = true
	}

	result := make([]string, 0, len(seen))
	for _, value := range ordered {
		if seen[value] {
			result = append(result, value)
		}
	}
	return result, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
