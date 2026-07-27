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
	SchemaVersion              int            `json:"schema_version,omitempty"`
	Version                    string         `json:"version"`
	CatalogVersion             string         `json:"catalog_version,omitempty"`
	OverrideVersion            uint64         `json:"override_version"`
	Billable                   bool           `json:"billable"`
	CatalogVendor              string         `json:"catalog_vendor"`
	TransportEngine            string         `json:"transport_engine"`
	RequestedModelID           string         `json:"requested_model_id"`
	CanonicalModelID           string         `json:"canonical_model_id"`
	CatalogMaxOutputTokens     int64          `json:"catalog_max_output_tokens"`
	EffectiveMaxOutputTokens   int            `json:"effective_max_output_tokens"`
	ContextTierThresholdTokens int64          `json:"context_tier_threshold_tokens"`
	MultiplierPPM              int64          `json:"multiplier_ppm"`
	PriceSource                string         `json:"price_source,omitempty"`
	SourceURL                  string         `json:"source_url"`
	RetrievedAt                string         `json:"retrieved_at"`
	Rates                      []pricing.Rate `json:"rates"`
}

const CurrentPricingSnapshotSchemaVersion = 2

type PricingSnapshotInput struct {
	TransportEngine          string
	RequestedModelID         string
	EffectiveMaxOutputTokens int
	MultiplierPPM            int64
}

func NewPricingSnapshot(model pricing.ModelPrice, input PricingSnapshotInput) (PricingSnapshot, error) {
	snapshot := PricingSnapshot{
		SchemaVersion: CurrentPricingSnapshotSchemaVersion,
		Version:       model.Version, CatalogVersion: model.CatalogVersion, OverrideVersion: model.OverrideVersion,
		Billable: true, CatalogVendor: model.CatalogVendor, TransportEngine: input.TransportEngine,
		RequestedModelID: input.RequestedModelID, CanonicalModelID: model.ModelID,
		CatalogMaxOutputTokens: model.MaxOutputTokens, EffectiveMaxOutputTokens: input.EffectiveMaxOutputTokens,
		ContextTierThresholdTokens: model.ContextTierThresholdTokens, MultiplierPPM: input.MultiplierPPM,
		PriceSource: model.PriceSource, SourceURL: model.SourceURL, RetrievedAt: model.RetrievedAt,
		Rates: append([]pricing.Rate(nil), model.Rates...),
	}
	normalizePricingSnapshot(&snapshot)
	if err := validatePricingSnapshot(snapshot, true); err != nil {
		return PricingSnapshot{}, err
	}
	return snapshot, nil
}

func EncodePricingSnapshot(model pricing.ModelPrice, input PricingSnapshotInput) (string, error) {
	snapshot, err := NewPricingSnapshot(model, input)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(raw), nil
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
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &fields); err != nil || fields == nil {
		return PricingSnapshot{}, errors.New("pricing snapshot must be a JSON object")
	}
	normalizePricingSnapshot(&snapshot)
	if snapshot.SchemaVersion == 0 {
		if _, hasPriceSource := fields["price_source"]; hasPriceSource {
			return PricingSnapshot{}, errors.New("pricing snapshot schema version is missing")
		}
		if _, hasCatalogVersion := fields["catalog_version"]; hasCatalogVersion {
			return PricingSnapshot{}, errors.New("pricing snapshot schema version is missing")
		}
	} else {
		if snapshot.SchemaVersion != CurrentPricingSnapshotSchemaVersion {
			return PricingSnapshot{}, errors.New("pricing snapshot schema version is unsupported")
		}
		for _, required := range []string{"price_source", "catalog_version", "override_version", "context_tier_threshold_tokens"} {
			if _, exists := fields[required]; !exists {
				return PricingSnapshot{}, fmt.Errorf("pricing snapshot is missing %s", required)
			}
		}
	}
	if err := validatePricingSnapshot(snapshot, snapshot.SchemaVersion != 0); err != nil {
		return PricingSnapshot{}, err
	}
	return snapshot, nil
}

func normalizePricingSnapshot(snapshot *PricingSnapshot) {
	snapshot.Version = strings.TrimSpace(snapshot.Version)
	snapshot.CatalogVersion = strings.TrimSpace(snapshot.CatalogVersion)
	snapshot.CatalogVendor = strings.TrimSpace(snapshot.CatalogVendor)
	snapshot.TransportEngine = strings.TrimSpace(snapshot.TransportEngine)
	snapshot.RequestedModelID = strings.TrimSpace(snapshot.RequestedModelID)
	snapshot.CanonicalModelID = strings.TrimSpace(snapshot.CanonicalModelID)
	snapshot.PriceSource = strings.TrimSpace(snapshot.PriceSource)
	snapshot.SourceURL = strings.TrimSpace(snapshot.SourceURL)
	snapshot.RetrievedAt = strings.TrimSpace(snapshot.RetrievedAt)
	snapshot.Rates = append([]pricing.Rate(nil), snapshot.Rates...)
}

func validatePricingSnapshot(snapshot PricingSnapshot, current bool) error {
	if !snapshot.Billable || snapshot.Version == "" || snapshot.CatalogVendor == "" || snapshot.TransportEngine == "" || snapshot.RequestedModelID == "" || snapshot.CanonicalModelID == "" || snapshot.SourceURL == "" || snapshot.RetrievedAt == "" || len(snapshot.Rates) == 0 || snapshot.MultiplierPPM <= 0 || snapshot.CatalogMaxOutputTokens <= 0 || snapshot.EffectiveMaxOutputTokens <= 0 || int64(snapshot.EffectiveMaxOutputTokens) > snapshot.CatalogMaxOutputTokens || snapshot.ContextTierThresholdTokens < 0 {
		return errors.New("pricing snapshot is incomplete or non-billable")
	}
	if current {
		if snapshot.CatalogVersion == "" {
			return errors.New("pricing snapshot price metadata is incomplete")
		}
		switch snapshot.PriceSource {
		case "official":
			if snapshot.OverrideVersion != 0 || snapshot.Version != snapshot.CatalogVersion {
				return errors.New("official pricing snapshot version is inconsistent")
			}
		case "override":
			wantVersion := snapshot.CatalogVersion + ":override:" + strconv.FormatUint(snapshot.OverrideVersion, 10)
			if snapshot.OverrideVersion == 0 || snapshot.Version != wantVersion {
				return errors.New("override pricing snapshot version is inconsistent")
			}
		default:
			return errors.New("pricing snapshot price source is invalid")
		}
		if hasContextTier(snapshot.Rates) && snapshot.ContextTierThresholdTokens <= 0 {
			return errors.New("pricing snapshot context tier threshold is missing")
		}
	}
	model := snapshot.modelPrice()
	catalog, err := pricing.NewCatalogChecked([]pricing.ModelPrice{model})
	if err != nil {
		return fmt.Errorf("validate pricing snapshot catalog: %w", err)
	}
	resolved, err := catalog.Resolve(snapshot.RequestedModelID)
	if err != nil || resolved.ModelID != snapshot.CanonicalModelID || resolved.Version != snapshot.Version || resolved.CatalogVendor != snapshot.CatalogVendor {
		return errors.New("pricing snapshot model resolution is inconsistent")
	}
	return nil
}

func hasContextTier(rates []pricing.Rate) bool {
	for _, rate := range rates {
		if rate.TierKey == "short_context" || rate.TierKey == "long_context" {
			return true
		}
	}
	return false
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
		Version: snapshot.Version, CatalogVersion: snapshot.CatalogVersion, OverrideVersion: snapshot.OverrideVersion,
		CatalogVendor: snapshot.CatalogVendor, ModelID: snapshot.CanonicalModelID, Aliases: aliases,
		MaxOutputTokens: snapshot.CatalogMaxOutputTokens, ContextTierThresholdTokens: snapshot.ContextTierThresholdTokens,
		PriceSource: snapshot.PriceSource, SourceURL: snapshot.SourceURL, RetrievedAt: snapshot.RetrievedAt,
		Rates: append([]pricing.Rate(nil), snapshot.Rates...),
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
	if run.RunID <= 0 || run.UserID <= 0 || strings.TrimSpace(run.ModelID) != snapshot.RequestedModelID || preparedRequestSHA256 == ([32]byte{}) || quote.PreparedRequestSHA256 != preparedRequestSHA256 || quote.RequestFingerprint != run.RequestFingerprint || strings.TrimSpace(quote.PricingVersion) != snapshot.Version || quote.EffectiveMaxOutputTokens != snapshot.EffectiveMaxOutputTokens || quote.CurrentCallMaxUnits <= 0 || quote.PriorBillableUnits < 0 || quote.TargetHoldUnits <= 0 || len(quote.UpperBoundItems) == 0 {
		return gatewayError(ErrCodeInvalidPrepared, "quote does not match the locked pricing snapshot", 409)
	}
	target, err := cumulativeHoldTarget(quote.PriorBillableUnits, quote.CurrentCallMaxUnits)
	if err != nil || target != quote.TargetHoldUnits {
		return gatewayError(ErrCodeInvalidPrepared, "quote cumulative hold evidence is inconsistent", 409)
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
	selected, err := pricing.UpperBoundLines(snapshot.modelPrice(), lines)
	if err != nil {
		return gatewayError(ErrCodeInvalidPrepared, fmt.Sprintf("quote upper bound is not priceable from the locked snapshot: %v", err), 409)
	}
	recomputed, err := pricing.Quote(snapshot.modelPrice(), selected, snapshot.MultiplierPPM)
	if err != nil {
		return gatewayError(ErrCodeInvalidPrepared, fmt.Sprintf("quote is not priceable from the locked snapshot: %v", err), 409)
	}
	if recomputed.AmountUnits != quote.CurrentCallMaxUnits {
		return gatewayError(ErrCodeInvalidPrepared, "quote current call maximum differs from locked snapshot pricing", 409)
	}
	return nil
}
