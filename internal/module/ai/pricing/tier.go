package pricing

import (
	"math"
	"math/big"
	"strings"
)

func UpperBoundLines(price ModelPrice, lines []QuoteLine) ([]QuoteLine, error) {
	return selectTiers(price, lines, true)
}

func SettlementLines(price ModelPrice, lines []QuoteLine) ([]QuoteLine, error) {
	return selectTiers(price, lines, false)
}

func selectTiers(price ModelPrice, lines []QuoteLine, upperBound bool) ([]QuoteLine, error) {
	result := append([]QuoteLine(nil), lines...)
	attemptTotals := make(map[string]int64)
	if !upperBound {
		for i := range result {
			item, err := result[i].Item.Normalized()
			if err != nil {
				return nil, ErrUnsupportedUsage
			}
			result[i].Item = item
			category, ok := categoryForUsage(item.Category)
			if !ok {
				return nil, ErrUnsupportedUsage
			}
			if category != InputTokens && category != CacheRead && category != CacheWrite {
				continue
			}
			attempt := normalizedAttemptID(result[i])
			if item.Quantity > math.MaxInt64-attemptTotals[attempt] {
				return nil, ErrQuoteOverflow
			}
			attemptTotals[attempt] += item.Quantity
		}
	}

	for i := range result {
		item, err := result[i].Item.Normalized()
		if err != nil {
			return nil, ErrUnsupportedUsage
		}
		category, ok := categoryForUsage(item.Category)
		if !ok {
			return nil, ErrUnsupportedUsage
		}
		candidates := ratesFor(price, category, item.Unit)
		if len(candidates) == 0 {
			if category == MediaUnits {
				return nil, ErrUnsupportedUsage
			}
			return nil, ErrPriceUnavailable
		}

		if upperBound {
			// A tierless input line is the aggregate prompt upper bound. Final usage
			// may partition those tokens into ordinary input, cache read and cache
			// write, so reserve the most expensive mutually exclusive destination.
			if category == InputTokens && item.TierKey == "" {
				inputCandidates := append([]Rate(nil), candidates...)
				inputCandidates = append(inputCandidates, ratesFor(price, CacheRead, item.Unit)...)
				inputCandidates = append(inputCandidates, ratesFor(price, CacheWrite, item.Unit)...)
				selected := mostExpensive(inputCandidates)
				item.Category = usageCategoryForPricing(selected.Category)
				item.TierKey = selected.TierKey
				result[i].Item = item
				continue
			}
			if hasTier(candidates, "short_context") && hasTier(candidates, "long_context") {
				item.TierKey = mostExpensive(candidates).TierKey
			} else if item.TierKey != "" {
				if !hasTier(candidates, item.TierKey) {
					return nil, ErrPriceUnavailable
				}
			} else {
				item.TierKey = mostExpensive(candidates).TierKey
			}
		} else {
			contextTier := ""
			if price.ContextTierThresholdTokens > 0 && hasTier(candidates, "short_context") && hasTier(candidates, "long_context") {
				contextTier = "short_context"
				if attemptTotals[normalizedAttemptID(result[i])] > price.ContextTierThresholdTokens {
					contextTier = "long_context"
				}
			}
			switch {
			case contextTier != "":
				item.TierKey = contextTier
			case item.TierKey != "":
				if !hasTier(candidates, item.TierKey) {
					return nil, ErrPriceUnavailable
				}
			case len(candidates) == 1:
				item.TierKey = candidates[0].TierKey
			default:
				return nil, ErrPriceUnavailable
			}
		}
		result[i].Item = item
	}
	return result, nil
}

func normalizedAttemptID(line QuoteLine) string {
	attempt := strings.TrimSpace(line.AttemptID)
	if attempt == "" {
		return line.Key
	}
	return attempt
}

func ratesFor(price ModelPrice, category Category, unit string) []Rate {
	var rates []Rate
	for _, rate := range price.Rates {
		if rate.Category == category && strings.TrimSpace(rate.Unit) == unit {
			rate.Unit = strings.TrimSpace(rate.Unit)
			rate.TierKey = strings.TrimSpace(rate.TierKey)
			rates = append(rates, rate)
		}
	}
	return rates
}

func hasTier(rates []Rate, tier string) bool {
	for _, rate := range rates {
		if rate.TierKey == tier {
			return true
		}
	}
	return false
}

func mostExpensive(rates []Rate) Rate {
	best := rates[0]
	for _, rate := range rates[1:] {
		left := new(big.Int).Mul(big.NewInt(rate.PriceUnits), big.NewInt(best.UnitScale))
		right := new(big.Int).Mul(big.NewInt(best.PriceUnits), big.NewInt(rate.UnitScale))
		if left.Cmp(right) > 0 || (left.Cmp(right) == 0 && rate.TierKey > best.TierKey) {
			best = rate
		}
	}
	return best
}
