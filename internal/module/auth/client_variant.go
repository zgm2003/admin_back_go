package auth

import "strings"

type ClientVariant string

const (
	ClientBrowser ClientVariant = "browser"
	ClientDesktop ClientVariant = "desktop"
)

const ClientVariantHeader = "X-Admin-Client-Variant"

func ParseClientVariant(value string) (ClientVariant, bool) {
	variant := ClientVariant(strings.ToLower(strings.TrimSpace(value)))
	switch variant {
	case ClientBrowser, ClientDesktop:
		return variant, true
	default:
		return "", false
	}
}
