package enum

import (
	"fmt"
	"strings"
)

const (
	UploadDriverCOS = "cos"
	UploadDriverOSS = "oss"
)

var UploadDrivers = []string{
	UploadDriverCOS,
	UploadDriverOSS,
}

var UploadDriverLabels = map[string]string{
	UploadDriverCOS: "腾讯云 COS",
	UploadDriverOSS: "阿里云 OSS",
}

var UploadImageExts = []string{
	"jpeg",
	"jpg",
	"gif",
	"png",
	"svg",
	"ico",
	"doc",
	"psd",
	"bmp",
	"tiff",
	"webp",
	"tif",
	"pjpeg",
}

var UploadFileExts = []string{
	"docx",
	"pdf",
	"txt",
	"html",
	"zip",
	"tar",
	"doc",
	"css",
	"csv",
	"ppt",
	"xlsx",
	"xls",
	"xml",
}

var UploadFolders = []string{
	"avatars",
	"images",
	"videos",
	"cover_images",
	"ai-agents",
	"ai_chat_images",
	"releases",
	"tauri_updater",
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
