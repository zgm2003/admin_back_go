package contextengine

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"admin_back_go/internal/infra/contextindex"

	"golang.org/x/text/unicode/norm"
)

func EncodeSparse(text string) (contextindex.SparseVector, error) {
	return encodeSparseWithIndexer(text, sparseIndexV1)
}

func encodeSparseWithIndexer(text string, indexer func(string) uint32) (contextindex.SparseVector, error) {
	if !utf8.ValidString(text) || indexer == nil {
		return contextindex.SparseVector{}, errors.New("sparse input must be valid UTF-8")
	}
	terms := lexicalTerms(strings.ToLower(norm.NFKC.String(text)))
	frequencies := make(map[string]uint64, len(terms))
	for _, term := range terms {
		frequencies[term]++
	}
	weights := make(map[uint32]float64, len(frequencies))
	for term, frequency := range frequencies {
		weight := 1 + math.Log(float64(frequency))
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 {
			return contextindex.SparseVector{}, errors.New("sparse weight is not finite and positive")
		}
		weights[indexer(term)] += weight
	}
	indices := make([]uint32, 0, len(weights))
	for index := range weights {
		indices = append(indices, index)
	}
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })
	values := make([]float32, len(indices))
	for position, index := range indices {
		values[position] = float32(weights[index])
	}
	return contextindex.NewSparseVector(indices, values)
}

func lexicalTerms(text string) []string {
	var terms []string
	var latin strings.Builder
	var han []rune
	flushLatin := func() {
		if latin.Len() > 0 {
			terms = append(terms, latin.String())
			latin.Reset()
		}
	}
	flushHan := func() {
		for index, char := range han {
			terms = append(terms, string(char))
			if index+1 < len(han) {
				terms = append(terms, string(han[index:index+2]))
			}
		}
		han = han[:0]
	}
	for _, char := range text {
		switch {
		case unicode.Is(unicode.Han, char):
			flushLatin()
			han = append(han, char)
		case unicode.IsLetter(char) || unicode.IsDigit(char):
			flushHan()
			latin.WriteRune(char)
		default:
			flushLatin()
			flushHan()
		}
	}
	flushLatin()
	flushHan()
	return terms
}

func sparseIndexV1(term string) uint32 {
	digest := sha256.Sum256([]byte("unicode-lexical-v1\x00" + term))
	return binary.BigEndian.Uint32(digest[:4])
}
