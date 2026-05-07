package enum

import "strings"

const (
	ClientPlatformWindowsX8664 = "windows-x86_64"
	ClientPlatformDarwinX8664  = "darwin-x86_64"
)

var ClientPlatforms = []string{
	ClientPlatformWindowsX8664,
	ClientPlatformDarwinX8664,
}

var ClientPlatformLabels = map[string]string{
	ClientPlatformWindowsX8664: "Windows",
	ClientPlatformDarwinX8664:  "macOS",
}

func IsClientPlatform(value string) bool {
	return containsString(ClientPlatforms, strings.TrimSpace(value))
}

func ClientPlatformName(value string) string {
	return ClientPlatformLabels[strings.TrimSpace(value)]
}
