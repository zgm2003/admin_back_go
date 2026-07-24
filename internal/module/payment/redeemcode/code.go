package redeemcode

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

const (
	CodeAlphabet                    = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
	RequestFingerprintVersion       = "redeem_batch_request_v1"
	MaxBatchQuantity                = 1000
	MaxVoidCodes                    = 1000
	MaxExportRows                   = 10000
	MaxRawCodeBytes                 = 128
	MaxAmountCents            int64 = 100_000_000

	randomCharacterCount     = 20
	maxRandomReadAttempts    = randomCharacterCount * 16
	maxDuplicateCodeAttempts = 32
)

var (
	ErrInvalidCode             = errors.New("redeem code is invalid")
	ErrInvalidAmount           = errors.New("redeem code amount is invalid")
	ErrCodeGenerationExhausted = errors.New("redeem code random source exhausted")
	ErrCodeUniquenessExhausted = errors.New("redeem code uniqueness budget exhausted")
)

func GenerateCode(random io.Reader) (string, error) {
	if random == nil {
		return "", ErrCodeGenerationExhausted
	}
	randomPart := make([]byte, 0, randomCharacterCount)
	one := []byte{0}
	for attempt := 0; attempt < maxRandomReadAttempts && len(randomPart) < randomCharacterCount; attempt++ {
		n, err := random.Read(one)
		if n == 1 {
			if one[0] < 248 {
				randomPart = append(randomPart, CodeAlphabet[int(one[0])%len(CodeAlphabet)])
			}
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return "", ErrCodeGenerationExhausted
		}
	}
	if len(randomPart) != randomCharacterCount {
		return "", ErrCodeGenerationExhausted
	}
	return fmt.Sprintf("ZHR-%s-%s-%s-%s-%s",
		randomPart[0:4], randomPart[4:8], randomPart[8:12], randomPart[12:16], randomPart[16:20]), nil
}

func generateUniqueCodes(random io.Reader, quantity int) ([]string, error) {
	if quantity <= 0 || quantity > MaxBatchQuantity {
		return nil, ErrCodeUniquenessExhausted
	}
	codes := make([]string, 0, quantity)
	seen := make(map[string]struct{}, quantity)
	for len(codes) < quantity {
		added := false
		for attempt := 0; attempt < maxDuplicateCodeAttempts; attempt++ {
			code, err := GenerateCode(random)
			if err != nil {
				return nil, err
			}
			if _, exists := seen[code]; exists {
				continue
			}
			seen[code] = struct{}{}
			codes = append(codes, code)
			added = true
			break
		}
		if !added {
			return nil, ErrCodeUniquenessExhausted
		}
	}
	return codes, nil
}

func NormalizeCode(raw string) (string, error) {
	if len(raw) == 0 || len(raw) > MaxRawCodeBytes {
		return "", ErrInvalidCode
	}
	compact := make([]byte, 0, 3+randomCharacterCount)
	for index := 0; index < len(raw); index++ {
		character := raw[index]
		if character >= 0x80 || character < 0x20 || character == 0x7f {
			return "", ErrInvalidCode
		}
		if character == ' ' || character == '-' {
			continue
		}
		if character >= 'a' && character <= 'z' {
			character -= 'a' - 'A'
		}
		if !((character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9')) {
			return "", ErrInvalidCode
		}
		compact = append(compact, character)
	}
	if len(compact) != 3+randomCharacterCount || string(compact[:3]) != "ZHR" {
		return "", ErrInvalidCode
	}
	for _, character := range compact[3:] {
		if !strings.ContainsRune(CodeAlphabet, rune(character)) {
			return "", ErrInvalidCode
		}
	}
	part := compact[3:]
	return fmt.Sprintf("ZHR-%s-%s-%s-%s-%s", part[0:4], part[4:8], part[8:12], part[12:16], part[16:20]), nil
}

func ParseAmountCents(raw string) (int64, string, error) {
	if raw == "" {
		return 0, "", ErrInvalidAmount
	}
	dot := -1
	for index := 0; index < len(raw); index++ {
		character := raw[index]
		if character == '.' {
			if dot >= 0 {
				return 0, "", ErrInvalidAmount
			}
			dot = index
			continue
		}
		if character < '0' || character > '9' {
			return 0, "", ErrInvalidAmount
		}
	}
	integerPart := raw
	fractionPart := ""
	if dot >= 0 {
		integerPart = raw[:dot]
		fractionPart = raw[dot+1:]
	}
	if integerPart == "" || (len(integerPart) > 1 && integerPart[0] == '0') || len(fractionPart) > 2 || (dot >= 0 && len(fractionPart) == 0) {
		return 0, "", ErrInvalidAmount
	}

	var whole int64
	for index := 0; index < len(integerPart); index++ {
		digit := int64(integerPart[index] - '0')
		if whole > (math.MaxInt64-digit)/10 {
			return 0, "", ErrInvalidAmount
		}
		whole = whole*10 + digit
	}
	fraction := int64(0)
	if len(fractionPart) > 0 {
		fraction = int64(fractionPart[0]-'0') * 10
		if len(fractionPart) == 2 {
			fraction += int64(fractionPart[1] - '0')
		}
	}
	if whole > (math.MaxInt64-fraction)/100 {
		return 0, "", ErrInvalidAmount
	}
	cents := whole*100 + fraction
	return cents, fmt.Sprintf("%d.%02d", cents/100, cents%100), nil
}
