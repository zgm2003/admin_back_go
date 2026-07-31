package capability

import aiprovider "admin_back_go/internal/module/ai/provider"

const (
	MaxAttachmentsPerMessage    = 5
	MaxMessageAttachmentBytes   = int64(50 << 20)
	MaxNativeFileBytesExclusive = int64(50 << 20)
	MaxRequestNativeFileBytes   = int64(50 << 20)

	NativeFileDisabledOfficialModel = "official_model_unsupported"
	NativeFileDisabledProviderMode  = "provider_file_input_disabled"
	NativeFileDisabledTransport     = "transport_unsupported"
	NativeFileDisabledPlatform      = "platform_unsupported"
)

var NativeFileExtensions = []string{
	"pdf", "doc", "docx", "dot", "odt", "rtf", "ppt", "pptx", "pot", "ppa", "pps", "pwz", "wiz",
	"xla", "xlb", "xlc", "xlm", "xls", "xlsx", "xlt", "xlw", "csv", "tsv", "iif",
	"txt", "text", "md", "markdown", "json", "html", "htm", "xml", "css",
	"asm", "bat", "c", "cc", "cpp", "cxx", "h", "hh", "def", "in", "js", "mjs", "jsx", "ts", "tsx",
	"py", "go", "java", "cs", "php", "rb", "rs", "sh", "bash", "zsh", "ksh", "ps1", "sql", "pl", "lua",
	"r", "scala", "swift", "kt", "kts", "yaml", "yml", "toml", "ini", "conf", "properties", "proto",
	"eml", "log", "rst", "srt", "vtt", "ics", "ifb", "vcf", "diff", "patch",
}

var ImageMIMETypes = []string{"image/jpeg", "image/png", "image/webp", "image/gif"}

type NativeFileCapabilityInput struct {
	OfficialEnabled      bool
	TransportEnabled     bool
	ProviderMode         string
	ProviderRouteEnabled bool
	PlatformReady        bool
	AcceptedExtensions   []string
}

type NativeFileCapability struct {
	Enabled            bool
	DisabledReason     string
	AcceptedExtensions []string
}

func AllowedNativeFileExtensions(systemExtensions []string) []string {
	requested := make(map[string]struct{}, len(systemExtensions))
	for _, extension := range systemExtensions {
		requested[extension] = struct{}{}
	}

	accepted := make([]string, 0, len(requested))
	for _, extension := range NativeFileExtensions {
		if _, ok := requested[extension]; ok {
			accepted = append(accepted, extension)
		}
	}
	return accepted
}

func ResolveNativeFileCapability(input NativeFileCapabilityInput) NativeFileCapability {
	reason := ""
	switch {
	case !input.OfficialEnabled:
		reason = NativeFileDisabledOfficialModel
	case !input.TransportEnabled:
		reason = NativeFileDisabledTransport
	case input.ProviderMode != aiprovider.FileInputModeChatCompletions || !input.ProviderRouteEnabled:
		reason = NativeFileDisabledProviderMode
	case !input.PlatformReady || len(input.AcceptedExtensions) == 0:
		reason = NativeFileDisabledPlatform
	}
	if reason != "" {
		return NativeFileCapability{DisabledReason: reason, AcceptedExtensions: []string{}}
	}
	return NativeFileCapability{
		Enabled:            true,
		AcceptedExtensions: append([]string(nil), input.AcceptedExtensions...),
	}
}
