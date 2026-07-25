package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/module/ai/pricing"
)

// PricingSnapshot is the closed, immutable pricing proof accepted with a Run.
// Per-attempt request quantities remain in QuoteEvidence.
type PricingSnapshot struct {
	Version                  string         `json:"version"`
	Billable                 bool           `json:"billable"`
	CatalogVendor            string         `json:"catalog_vendor"`
	TransportEngine          string         `json:"transport_engine"`
	RequestedModelID         string         `json:"requested_model_id"`
	CanonicalModelID         string         `json:"canonical_model_id"`
	CatalogMaxOutputTokens   int64          `json:"catalog_max_output_tokens"`
	EffectiveMaxOutputTokens int            `json:"effective_max_output_tokens"`
	MultiplierPPM            int64          `json:"multiplier_ppm"`
	SourceURL                string         `json:"source_url"`
	RetrievedAt              string         `json:"retrieved_at"`
	Rates                    []pricing.Rate `json:"rates"`
}

type PersistedQuoteValidator struct{}

func ParsePricingSnapshot(raw string) (PricingSnapshot, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var snapshot PricingSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return PricingSnapshot{}, fmt.Errorf("decode pricing snapshot: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return PricingSnapshot{}, err
	}
	snapshot.Version = strings.TrimSpace(snapshot.Version)
	snapshot.CatalogVendor = strings.TrimSpace(snapshot.CatalogVendor)
	snapshot.TransportEngine = strings.TrimSpace(snapshot.TransportEngine)
	snapshot.RequestedModelID = strings.TrimSpace(snapshot.RequestedModelID)
	snapshot.CanonicalModelID = strings.TrimSpace(snapshot.CanonicalModelID)
	snapshot.SourceURL = strings.TrimSpace(snapshot.SourceURL)
	snapshot.RetrievedAt = strings.TrimSpace(snapshot.RetrievedAt)
	if !snapshot.Billable || snapshot.Version == "" || snapshot.CatalogVendor == "" || snapshot.TransportEngine == "" || snapshot.RequestedModelID == "" || snapshot.CanonicalModelID == "" || snapshot.SourceURL == "" || snapshot.RetrievedAt == "" || len(snapshot.Rates) == 0 || snapshot.MultiplierPPM <= 0 || snapshot.CatalogMaxOutputTokens <= 0 || snapshot.EffectiveMaxOutputTokens <= 0 || int64(snapshot.EffectiveMaxOutputTokens) > snapshot.CatalogMaxOutputTokens {
		return PricingSnapshot{}, errors.New("pricing snapshot is incomplete or non-billable")
	}
	model := snapshot.modelPrice()
	catalog, err := pricing.NewCatalogChecked([]pricing.ModelPrice{model})
	if err != nil {
		return PricingSnapshot{}, fmt.Errorf("validate pricing snapshot catalog: %w", err)
	}
	resolved, err := catalog.Resolve(snapshot.RequestedModelID)
	if err != nil || resolved.ModelID != snapshot.CanonicalModelID || resolved.Version != snapshot.Version || resolved.CatalogVendor != snapshot.CatalogVendor {
		return PricingSnapshot{}, errors.New("pricing snapshot model resolution is inconsistent")
	}
	return snapshot, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("pricing snapshot contains trailing JSON")
		}
		return fmt.Errorf("decode pricing snapshot trailer: %w", err)
	}
	return nil
}

func (snapshot PricingSnapshot) modelPrice() pricing.ModelPrice {
	aliases := []string(nil)
	if snapshot.RequestedModelID != snapshot.CanonicalModelID {
		aliases = []string{snapshot.RequestedModelID}
	}
	return pricing.ModelPrice{
		Version:         snapshot.Version,
		CatalogVendor:   snapshot.CatalogVendor,
		ModelID:         snapshot.CanonicalModelID,
		Aliases:         aliases,
		MaxOutputTokens: snapshot.CatalogMaxOutputTokens,
		SourceURL:       snapshot.SourceURL,
		RetrievedAt:     snapshot.RetrievedAt,
		Rates:           append([]pricing.Rate(nil), snapshot.Rates...),
	}
}

func (PersistedQuoteValidator) ValidateQuote(ctx context.Context, run RunSnapshot, preparedRequestSHA256 [32]byte, quote QuoteEvidence) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	snapshot, err := ParsePricingSnapshot(run.PricingSnapshotJSON)
	if err != nil {
		return gatewayError(ErrCodeInvalidPrepared, err.Error(), 409)
	}
	if run.RunID <= 0 || run.UserID <= 0 || strings.TrimSpace(run.ModelID) != snapshot.RequestedModelID || preparedRequestSHA256 == ([32]byte{}) || quote.PreparedRequestSHA256 != preparedRequestSHA256 || quote.RequestFingerprint != run.RequestFingerprint || strings.TrimSpace(quote.PricingVersion) != snapshot.Version || quote.EffectiveMaxOutputTokens != snapshot.EffectiveMaxOutputTokens || quote.TargetHoldUnits <= 0 || len(quote.UpperBoundItems) == 0 {
		return gatewayError(ErrCodeInvalidPrepared, "quote does not match the locked pricing snapshot", 409)
	}
	lines := make([]pricing.QuoteLine, len(quote.UpperBoundItems))
	seen := make(map[string]struct{}, len(quote.UpperBoundItems))
	for index, rawItem := range quote.UpperBoundItems {
		item, normalizeErr := rawItem.Normalized()
		if normalizeErr != nil {
			return gatewayError(ErrCodeInvalidPrepared, "quote contains invalid upper-bound usage", 409)
		}
		identity := string(item.Category) + "\x00" + item.Unit + "\x00" + item.TierKey
		if _, exists := seen[identity]; exists {
			return gatewayError(ErrCodeInvalidPrepared, "quote contains duplicate upper-bound usage", 409)
		}
		seen[identity] = struct{}{}
		if item.Category == billing.UsageCategoryOutputText && item.Unit == "token" && item.Quantity != int64(snapshot.EffectiveMaxOutputTokens) {
			return gatewayError(ErrCodeInvalidPrepared, "quote output bound differs from the effective output cap", 409)
		}
		lines[index] = pricing.QuoteLine{Key: "upper-bound-" + strconv.Itoa(index), Item: item}
	}
	recomputed, err := pricing.Quote(snapshot.modelPrice(), lines, snapshot.MultiplierPPM)
	if err != nil {
		return gatewayError(ErrCodeInvalidPrepared, fmt.Sprintf("quote is not priceable from the locked snapshot: %v", err), 409)
	}
	if recomputed.AmountUnits != quote.TargetHoldUnits {
		return gatewayError(ErrCodeInvalidPrepared, "quote hold target differs from locked snapshot pricing", 409)
	}
	return nil
}
