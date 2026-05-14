package i18n

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLocalizeUsesAcceptLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Localize())
	router.GET("/probe", func(c *gin.Context) {
		message, err := Message(c, "auth.token.missing", nil, "fallback")
		if err != nil {
			t.Fatalf("localize message: %v", err)
		}
		c.String(http.StatusOK, message)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Accept-Language", "en-US")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Body.String() != "Missing token" {
		t.Fatalf("expected English message, got %q", recorder.Body.String())
	}
}

func TestLocalizeFallsBackToZhCN(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Localize())
	router.GET("/probe", func(c *gin.Context) {
		message, err := Message(c, "auth.token.missing", nil, "fallback")
		if err != nil {
			t.Fatalf("localize message: %v", err)
		}
		c.String(http.StatusOK, message)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Accept-Language", "fr-FR")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Body.String() != "缺少Token" {
		t.Fatalf("expected zh-CN fallback, got %q", recorder.Body.String())
	}
}

func TestCatalogKeysMatch(t *testing.T) {
	zhKeys, err := CatalogKeys("zh-CN")
	if err != nil {
		t.Fatalf("load zh-CN keys: %v", err)
	}
	enKeys, err := CatalogKeys("en-US")
	if err != nil {
		t.Fatalf("load en-US keys: %v", err)
	}
	if len(zhKeys) != len(enKeys) {
		t.Fatalf("catalog key count mismatch zh=%d en=%d", len(zhKeys), len(enKeys))
	}
	for key := range zhKeys {
		if _, ok := enKeys[key]; !ok {
			t.Fatalf("missing en-US key %q", key)
		}
	}
}
