// Package i18n wires project catalogs into gin-contrib/i18n.
package i18n

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	gini18n "github.com/gin-contrib/i18n"
	"github.com/gin-gonic/gin"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

//go:embed locales/*/*.yaml
var localeFS embed.FS

var (
	zhCN               = language.MustParse("zh-CN")
	enUS               = language.MustParse("en-US")
	supportedLanguages = []language.Tag{zhCN, enUS}
)

// Localize returns the Gin middleware used by admin_back_go.
func Localize() gin.HandlerFunc {
	return gini18n.Localize(gini18n.WithBundle(&gini18n.BundleCfg{
		RootPath:          "locales",
		AcceptLanguage:    supportedLanguages,
		FallbackLanguages: []language.Tag{zhCN},
		DefaultLanguage:   zhCN,
		FormatBundleFile:  "yaml",
		UnmarshalFunc:     yaml.Unmarshal,
		Loader:            gini18n.LoaderFunc(loadLanguageCatalog),
	}), gini18n.WithGetLngHandle(func(c *gin.Context, defaultLng string) string {
		if c == nil || c.Request == nil {
			return defaultLng
		}
		return MatchLanguage(c.GetHeader("Accept-Language")).String()
	}))
}

// MatchLanguage maps browser Accept-Language values to supported project tags.
func MatchLanguage(header string) language.Tag {
	tags, _, err := language.ParseAcceptLanguage(strings.TrimSpace(header))
	if err != nil || len(tags) == 0 {
		return zhCN
	}
	for _, tag := range tags {
		base, _ := tag.Base()
		switch base.String() {
		case "en":
			return enUS
		case "zh":
			return zhCN
		}
	}
	return zhCN
}

// Message localizes a message key and falls back to fallback when i18n is absent
// or when the key is missing.
func Message(c *gin.Context, messageID string, templateData map[string]any, fallback string) (string, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		messageID = FallbackMessageID(fallback)
	}
	if messageID == "" {
		return fallback, nil
	}
	if c == nil {
		return fallback, nil
	}
	if _, ok := c.Get("i18n"); !ok {
		return fallback, nil
	}

	localized, err := gini18n.GetMessage(c, &goi18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: templateData,
	})
	if err != nil || strings.TrimSpace(localized) == "" {
		return fallback, err
	}
	return localized, nil
}

// CatalogKeys returns the flattened key set for a language directory.
func CatalogKeys(lang string) (map[string]struct{}, error) {
	files, err := catalogFiles(lang)
	if err != nil {
		return nil, err
	}

	keys := make(map[string]struct{})
	for _, file := range files {
		buf, err := localeFS.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read catalog %s: %w", file, err)
		}
		var values map[string]string
		if err := yaml.Unmarshal(buf, &values); err != nil {
			return nil, fmt.Errorf("parse catalog %s: %w", file, err)
		}
		for key, value := range values {
			key = strings.TrimSpace(key)
			if key == "" || strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("catalog %s contains empty key or value", file)
			}
			if _, exists := keys[key]; exists {
				return nil, fmt.Errorf("duplicate i18n key %s", key)
			}
			keys[key] = struct{}{}
		}
	}
	return keys, nil
}

func loadLanguageCatalog(filePath string) ([]byte, error) {
	lang := strings.TrimSuffix(path.Base(filePath), path.Ext(filePath))
	files, err := catalogFiles(lang)
	if err != nil {
		return nil, err
	}

	var merged bytes.Buffer
	for _, file := range files {
		buf, err := localeFS.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read catalog %s: %w", file, err)
		}
		merged.Write(buf)
		merged.WriteByte('\n')
	}
	return merged.Bytes(), nil
}

func catalogFiles(lang string) ([]string, error) {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return nil, fmt.Errorf("language is required")
	}
	pattern := "locales/" + lang + "/*.yaml"
	files, err := fs.Glob(localeFS, pattern)
	if err != nil {
		return nil, fmt.Errorf("glob catalog %s: %w", pattern, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no catalog files for %s", lang)
	}
	sort.Strings(files)
	return files, nil
}
