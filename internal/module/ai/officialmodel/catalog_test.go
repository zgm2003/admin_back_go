package officialmodel

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/module/ai/pricing"
)

func validCatalogModel(id string) Model {
	return Model{
		CatalogVersion:      "test-v1",
		CatalogVendor:       "openai",
		ModelFamily:         "gpt",
		ModelID:             id,
		LifecycleStatus:     LifecycleActive,
		ContextWindowTokens: 128000,
		MaxOutputTokens:     4096,
		TokenCounterID:      "utf8_bytes_v1",
		Capabilities: Capabilities{
			InputModalities:     []string{ModalityText},
			OutputModalities:    []string{ModalityText},
			SupportsStreaming:   true,
			SupportsTools:       true,
			SupportedParameters: []string{ParameterTemperature},
		},
		OfficialPrice: pricing.PriceBook{
			ModelID: id,
			Rates: []pricing.Rate{
				{Category: pricing.InputTokens, Unit: "token", PriceUnits: 1, UnitScale: 1_000_000},
				{Category: pricing.OutputTokens, Unit: "token", PriceUnits: 2, UnitScale: 1_000_000},
			},
		},
		ModelSourceURL:   "https://developers.openai.com/api/docs/models/" + id,
		PricingSourceURL: "https://developers.openai.com/api/docs/pricing",
		RetrievedAt:      "2026-07-27",
		ReviewAfter:      "2026-10-27",
	}
}

func TestOfficialCatalogRejectsDuplicateCaseSensitiveIdentity(t *testing.T) {
	first := validCatalogModel("model-a")
	second := validCatalogModel("model-a")
	if _, err := NewCatalog("test-v1", []Model{first, second}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("duplicate canonical identity error = %v", err)
	}

	second = validCatalogModel("model-b")
	first.Aliases = []string{"model-b"}
	if _, err := NewCatalog("test-v1", []Model{first, second}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("alias/canonical conflict error = %v", err)
	}

	first = validCatalogModel("model-a")
	second = validCatalogModel("model-b")
	first.Aliases = []string{"reviewed-alias"}
	second.Aliases = []string{"reviewed-alias"}
	if _, err := NewCatalog("test-v1", []Model{first, second}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("duplicate reviewed alias error = %v", err)
	}
}

func TestOfficialCatalogMatchesOnlyCanonicalIDOrReviewedAlias(t *testing.T) {
	model := validCatalogModel("Exact-Model")
	model.Aliases = []string{"exact-model-reviewed"}
	catalog, err := NewCatalog("test-v1", []Model{model})
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []string{"Exact-Model", " exact-model-reviewed "} {
		resolved, err := catalog.ResolveIdentity(identity)
		if err != nil || resolved.ModelID != "Exact-Model" {
			t.Fatalf("ResolveIdentity(%q) = %#v, %v", identity, resolved, err)
		}
	}
	for _, identity := range []string{"exact-model", "Exact", "Exact-Model-extra"} {
		if resolved, err := catalog.ResolveIdentity(identity); !errors.Is(err, ErrModelUnmapped) || resolved.ModelID != "" {
			t.Fatalf("ResolveIdentity(%q) = %#v, %v", identity, resolved, err)
		}
	}
}

func TestOfficialCatalogDefaultHasCompleteSourcesAndLimits(t *testing.T) {
	models := Default.Models()
	if Default.Version() != "official_models_v1" || len(models) != 24 {
		t.Fatalf("default catalog version=%q count=%d", Default.Version(), len(models))
	}
	for _, model := range models {
		if model.CatalogVersion != Default.Version() || model.ModelID == "" || model.CatalogVendor == "" || model.ModelFamily == "" {
			t.Fatalf("incomplete identity: %#v", model)
		}
		if model.ContextWindowTokens <= 0 || model.MaxOutputTokens <= 0 || model.MaxOutputTokens > model.ContextWindowTokens {
			t.Fatalf("invalid model limits: %#v", model)
		}
		if model.TokenCounterID != "utf8_bytes_v1" {
			t.Fatalf("%s token counter = %q", model.ModelID, model.TokenCounterID)
		}
		if len(model.OfficialPrice.Rates) == 0 || model.OfficialPrice.ModelID != model.ModelID {
			t.Fatalf("missing official price: %#v", model)
		}
		for _, source := range []string{model.ModelSourceURL, model.PricingSourceURL} {
			parsed, err := url.Parse(source)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
				t.Fatalf("invalid source %q for %s", source, model.ModelID)
			}
		}
		retrieved, err := time.Parse(time.DateOnly, model.RetrievedAt)
		if err != nil {
			t.Fatalf("invalid retrieved_at for %s: %v", model.ModelID, err)
		}
		reviewAfter, err := time.Parse(time.DateOnly, model.ReviewAfter)
		if err != nil || !reviewAfter.After(retrieved) {
			t.Fatalf("invalid review_after for %s: %q", model.ModelID, model.ReviewAfter)
		}
	}
}

func TestOfficialCatalogRequiresRegisteredTokenCounter(t *testing.T) {
	model := validCatalogModel("model-a")
	model.TokenCounterID = ""
	if _, err := NewCatalog("test-v1", []Model{model}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("missing token counter error = %v", err)
	}
	model.TokenCounterID = "unknown_v1"
	if _, err := NewCatalog("test-v1", []Model{model}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("unknown token counter error = %v", err)
	}
}

func TestOfficialCatalogDefaultRecordsVerifiedMultimodalCapabilities(t *testing.T) {
	for _, model := range Default.Models() {
		expectedInput := []string{ModalityText, ModalityImage, ModalityFile}
		expectedOutput := []string{ModalityText}
		if model.ModelID == "gpt-image-2" {
			expectedInput = []string{ModalityText, ModalityImage}
			expectedOutput = []string{ModalityImage}
		}
		if !reflect.DeepEqual(model.Capabilities.InputModalities, expectedInput) {
			t.Fatalf("%s input modalities = %#v", model.ModelID, model.Capabilities.InputModalities)
		}
		if !reflect.DeepEqual(model.Capabilities.OutputModalities, expectedOutput) {
			t.Fatalf("%s output modalities = %#v", model.ModelID, model.Capabilities.OutputModalities)
		}
		image := model.Capabilities.ImageInput
		if image == nil || len(image.MIMETypes) == 0 || image.MaxFiles <= 0 || image.MaxBytes <= 0 {
			t.Fatalf("%s image input = %#v", model.ModelID, image)
		}
		if model.ModelID == "gpt-image-2" {
			if model.Capabilities.NativeFileInput || containsString(model.Capabilities.InputModalities, ModalityFile) {
				t.Fatalf("%s must not advertise document input: %#v", model.ModelID, model.Capabilities)
			}
			continue
		}
		if !model.Capabilities.NativeFileInput || !containsString(model.Capabilities.InputModalities, ModalityFile) {
			t.Fatalf("%s document input = %#v", model.ModelID, model.Capabilities)
		}
	}
}

func TestOfficialCatalogDefaultRecordsVerifiedContextLimits(t *testing.T) {
	expected := map[string]int64{
		"gpt-5.6-sol": 1_050_000, "gpt-5.6-terra": 1_050_000, "gpt-5.6-luna": 1_050_000,
		"gpt-5.5": 1_050_000, "gpt-5.5-pro": 1_050_000,
		"gpt-5.4": 1_050_000, "gpt-5.4-mini": 400_000, "gpt-5.4-nano": 400_000, "gpt-5.4-pro": 1_050_000,
		"claude-fable-5": 1_000_000, "claude-opus-5": 1_000_000, "claude-sonnet-5": 1_000_000,
		"claude-opus-4-8": 1_000_000, "claude-opus-4-7": 1_000_000,
		"claude-opus-4-6": 1_000_000, "claude-sonnet-4-6": 1_000_000,
		"claude-haiku-4-5-20251001":  200_000,
		"claude-sonnet-4-5-20250929": 200_000, "claude-opus-4-5-20251101": 200_000,
	}
	for modelID, contextWindow := range expected {
		model, err := Default.ResolveIdentity(modelID)
		if err != nil {
			t.Fatal(err)
		}
		if model.ContextWindowTokens != contextWindow {
			t.Fatalf("%s context window = %d, want %d", modelID, model.ContextWindowTokens, contextWindow)
		}
	}
	sonnet, err := Default.ResolveIdentity("claude-sonnet-4-6")
	if err != nil {
		t.Fatal(err)
	}
	if sonnet.MaxOutputTokens != 128_000 {
		t.Fatalf("claude-sonnet-4-6 max output = %d, want 128000", sonnet.MaxOutputTokens)
	}
}

func TestOfficialCatalogDefaultDoesNotInventUnsupportedOpenAIFeatures(t *testing.T) {
	tests := []struct {
		modelID          string
		streaming        bool
		structuredOutput bool
	}{
		{modelID: "gpt-5.5-pro", streaming: false, structuredOutput: true},
		{modelID: "gpt-5.4-pro", streaming: true, structuredOutput: false},
	}
	for _, test := range tests {
		model, err := Default.ResolveIdentity(test.modelID)
		if err != nil {
			t.Fatal(err)
		}
		if model.Capabilities.SupportsStreaming != test.streaming ||
			model.Capabilities.SupportsStructuredOutput != test.structuredOutput {
			t.Fatalf("%s capabilities = %#v", test.modelID, model.Capabilities)
		}
	}
}

func TestOfficialModelManagementDTOEncodesEmptyCollectionsAsArrays(t *testing.T) {
	model := validCatalogModel("model-with-empty-collections")
	model.Aliases = []string{}
	model.Capabilities.SupportedParameters = []string{}
	catalog, err := NewCatalog("test-v1", []Model{model})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(
		&fakeOverrideRepository{},
		WithCatalog(catalog),
	)
	detail, appErr := service.Detail(context.Background(), model.ModelID)
	if appErr != nil {
		t.Fatal(appErr)
	}
	payload, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, field := range []string{`"aliases":[]`, `"supported_parameters":[]`, `"token_counter_id":"utf8_bytes_v1"`} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("management DTO must encode %s: %s", field, encoded)
		}
	}
}
