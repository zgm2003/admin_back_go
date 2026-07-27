package pricing

import (
	"math/big"
	"sort"

	"admin_back_go/internal/module/ai/billing"
)

type QuoteLine struct {
	Key       string            `json:"key"`
	AttemptID string            `json:"attempt_id,omitempty"`
	Item      billing.UsageItem `json:"item"`
}

type QuoteLineResult struct {
	Key         string `json:"key"`
	AttemptID   string `json:"attempt_id,omitempty"`
	Rate        Rate   `json:"rate"`
	AmountUnits int64  `json:"amount_units"`
}

type QuoteResult struct {
	AmountUnits int64             `json:"amount_units"`
	Lines       []QuoteLineResult `json:"lines"`
}

type exactLine struct {
	index     int
	key       string
	attemptID string
	category  Category
	tierKey   string
	unit      string
	rate      Rate
	value     *big.Rat
}

func categoryForUsage(category billing.UsageCategory) (Category, bool) {
	switch category {
	case billing.UsageCategoryInputText:
		return InputTokens, true
	case billing.UsageCategoryOutputText:
		return OutputTokens, true
	case billing.UsageCategoryCacheRead:
		return CacheRead, true
	case billing.UsageCategoryCacheWrite:
		return CacheWrite, true
	case billing.UsageCategoryMedia:
		return MediaUnits, true
	default:
		return "", false
	}
}

func usageCategoryForPricing(category Category) billing.UsageCategory {
	switch category {
	case InputTokens:
		return billing.UsageCategoryInputText
	case OutputTokens:
		return billing.UsageCategoryOutputText
	case CacheRead:
		return billing.UsageCategoryCacheRead
	case CacheWrite:
		return billing.UsageCategoryCacheWrite
	case MediaUnits:
		return billing.UsageCategoryMedia
	default:
		return ""
	}
}

func allocate(lines []exactLine, total int64) []QuoteLineResult {
	result := make([]QuoteLineResult, len(lines))
	var floors int64
	for i, line := range lines {
		floor := new(big.Int).Quo(line.value.Num(), line.value.Denom())
		result[i] = QuoteLineResult{Key: line.key, AttemptID: line.attemptID, Rate: line.rate, AmountUnits: floor.Int64()}
		floors += floor.Int64()
	}
	remaining := total - floors
	if remaining > 0 {
		order := append([]exactLine(nil), lines...)
		sort.SliceStable(order, func(i, j int) bool {
			left := new(big.Int).Mod(order[i].value.Num(), order[i].value.Denom())
			right := new(big.Int).Mod(order[j].value.Num(), order[j].value.Denom())
			crossLeft := new(big.Int).Mul(left, order[j].value.Denom())
			crossRight := new(big.Int).Mul(right, order[i].value.Denom())
			if crossLeft.Cmp(crossRight) != 0 {
				return crossLeft.Cmp(crossRight) > 0
			}
			if order[i].attemptID != order[j].attemptID {
				return order[i].attemptID < order[j].attemptID
			}
			if order[i].category != order[j].category {
				return order[i].category < order[j].category
			}
			if order[i].tierKey != order[j].tierKey {
				return order[i].tierKey < order[j].tierKey
			}
			if order[i].unit != order[j].unit {
				return order[i].unit < order[j].unit
			}
			return order[i].key < order[j].key
		})
		for i := int64(0); i < remaining; i++ {
			result[order[i%int64(len(order))].index].AmountUnits++
		}
	}
	return result
}
