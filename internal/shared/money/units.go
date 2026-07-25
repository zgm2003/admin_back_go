package money

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

const (
	UnitsPerRMB  int64 = 100_000_000
	UnitsPerCent int64 = 1_000_000
)

var ErrInvalidAmount = errors.New("money amount must be non-negative and fit in int64 units")

func CentsToUnits(cents int64) (int64, error) {
	if cents < 0 || cents > math.MaxInt64/UnitsPerCent {
		return 0, ErrInvalidAmount
	}
	return cents * UnitsPerCent, nil
}

func FormatRMBUnits(units int64) (string, error) {
	if units < 0 {
		return "", ErrInvalidAmount
	}
	whole := units / UnitsPerRMB
	fraction := units % UnitsPerRMB
	if fraction == 0 {
		return strconv.FormatInt(whole, 10), nil
	}
	fractionText := strings.TrimRight(strconv.FormatInt(fraction+UnitsPerRMB, 10)[1:], "0")
	return strconv.FormatInt(whole, 10) + "." + fractionText, nil
}
