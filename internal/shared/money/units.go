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

func ParseRMBUnits(value string) (int64, error) {
	if value == "" {
		return 0, ErrInvalidAmount
	}
	dot := -1
	for i := 0; i < len(value); i++ {
		switch {
		case value[i] >= '0' && value[i] <= '9':
		case value[i] == '.' && dot == -1:
			dot = i
		default:
			return 0, ErrInvalidAmount
		}
	}
	if dot == 0 || dot == len(value)-1 {
		return 0, ErrInvalidAmount
	}
	whole, fraction := value, ""
	if dot >= 0 {
		whole, fraction = value[:dot], value[dot+1:]
	}
	if len(fraction) > 8 {
		return 0, ErrInvalidAmount
	}
	digits := strings.TrimLeft(whole+fraction+strings.Repeat("0", 8-len(fraction)), "0")
	if digits == "" {
		return 0, nil
	}
	units, err := strconv.ParseUint(digits, 10, 63)
	if err != nil || units > math.MaxInt64 {
		return 0, ErrInvalidAmount
	}
	return int64(units), nil
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
