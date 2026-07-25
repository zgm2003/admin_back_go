package pricing

import (
	"math"
	"math/big"
	"strings"
)

const multiplierScale int64 = 1_000_000

func Quote(price ModelPrice, lines []QuoteLine, multiplierPPM int64) (QuoteResult, error) {
	if multiplierPPM <= 0 {
		return QuoteResult{}, ErrInvalidMultiplier
	}
	if strings.TrimSpace(price.ModelID) == "" {
		return QuoteResult{}, ErrMissingModel
	}
	rateByKey := make(map[string]Rate, len(price.Rates))
	for _, rate := range price.Rates {
		if rate.UnitScale <= 0 || rate.PriceUnits < 0 || strings.TrimSpace(rate.Unit) == "" || !validCategory(rate.Category) {
			return QuoteResult{}, ErrInvalidCatalog
		}
		key := rateKey(rate.Category, strings.TrimSpace(rate.Unit), strings.TrimSpace(rate.TierKey))
		if _, exists := rateByKey[key]; exists {
			return QuoteResult{}, ErrInvalidCatalog
		}
		rate.Unit, rate.TierKey = strings.TrimSpace(rate.Unit), strings.TrimSpace(rate.TierKey)
		rateByKey[key] = rate
	}
	seenIDs := make(map[string]struct{}, len(lines))
	seenIdentities := make(map[string]struct{}, len(lines))
	exact := make([]exactLine, 0, len(lines))
	total := new(big.Rat)
	for index, line := range lines {
		if strings.TrimSpace(line.Key) == "" {
			return QuoteResult{}, ErrDuplicateLine
		}
		if _, exists := seenIDs[line.Key]; exists {
			return QuoteResult{}, ErrDuplicateLine
		}
		seenIDs[line.Key] = struct{}{}
		item, err := line.Item.Normalized()
		if err != nil {
			return QuoteResult{}, ErrUnsupportedUsage
		}
		category, ok := categoryForUsage(item.Category)
		if !ok {
			return QuoteResult{}, ErrUnsupportedUsage
		}
		key := rateKey(category, item.Unit, item.TierKey)
		attemptID := strings.TrimSpace(line.AttemptID)
		if attemptID == "" {
			attemptID = line.Key
		}
		identity := attemptID + "\x00" + key
		if _, exists := seenIdentities[identity]; exists {
			return QuoteResult{}, ErrDuplicateLine
		}
		seenIdentities[identity] = struct{}{}
		rate, ok := rateByKey[key]
		if !ok {
			if category == MediaUnits {
				return QuoteResult{}, ErrUnsupportedUsage
			}
			return QuoteResult{}, ErrPriceUnavailable
		}
		value := new(big.Rat).SetFrac(
			new(big.Int).Mul(big.NewInt(item.Quantity), big.NewInt(rate.PriceUnits)),
			big.NewInt(rate.UnitScale),
		)
		value.Mul(value, new(big.Rat).SetFrac(big.NewInt(multiplierPPM), big.NewInt(multiplierScale)))
		total.Add(total, value)
		exact = append(exact, exactLine{index: index, key: line.Key, attemptID: attemptID, category: category, tierKey: item.TierKey, unit: item.Unit, rate: rate, value: value})
	}
	if len(exact) == 0 {
		return QuoteResult{}, nil
	}
	// Ceil once after summing all exact line values.
	numerator := total.Num()
	denominator := total.Denom()
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if quotient.Sign() < 0 || !quotient.IsInt64() || quotient.Cmp(big.NewInt(math.MaxInt64)) > 0 {
		return QuoteResult{}, ErrQuoteOverflow
	}
	amount := quotient.Int64()
	allocated := allocate(exact, amount)
	return QuoteResult{AmountUnits: amount, Lines: allocated}, nil
}
