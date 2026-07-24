package redeemcode

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestGenerateCodeUsesFixedFormatAlphabetAndRejectionSampling(t *testing.T) {
	rejected := []byte{248, 249, 250, 251, 252, 253, 254, 255}
	accepted := make([]byte, 20)
	for index := range accepted {
		accepted[index] = byte(index)
	}
	reader := &countingReader{Reader: bytes.NewReader(append(rejected, accepted...))}
	code, err := GenerateCode(reader)
	if err != nil {
		t.Fatalf("GenerateCode error=%v", err)
	}
	const want = "ZHR-2345-6789-ABCD-EFGH-JKMN"
	if code != want {
		t.Fatalf("GenerateCode=%q want %q; bytes 248..255 must be discarded", code, want)
	}
	if reader.reads != 28 {
		t.Fatalf("reader reads=%d want 28", reader.reads)
	}
}

func TestGeneratePublicConstantsMatchApprovedContract(t *testing.T) {
	if CodeAlphabet != "23456789ABCDEFGHJKMNPQRSTUVWXYZ" {
		t.Fatalf("CodeAlphabet=%q", CodeAlphabet)
	}
	if RequestFingerprintVersion != "redeem_batch_request_v1" {
		t.Fatalf("RequestFingerprintVersion=%q", RequestFingerprintVersion)
	}
	if MaxBatchQuantity != 1000 || MaxVoidCodes != 1000 || MaxExportRows != 10000 || MaxRawCodeBytes != 128 || MaxAmountCents != 100_000_000 {
		t.Fatalf("limits=(%d,%d,%d,%d,%d)", MaxBatchQuantity, MaxVoidCodes, MaxExportRows, MaxRawCodeBytes, MaxAmountCents)
	}
}

func TestGenerateCodeHasFiniteRejectionBudgetAndDoesNotLeakReaderInput(t *testing.T) {
	reader := repeatedByteReader{value: 255}
	code, err := GenerateCode(reader)
	if !errors.Is(err, ErrCodeGenerationExhausted) || code != "" {
		t.Fatalf("GenerateCode=(%q,%v)", code, err)
	}
	if strings.Contains(err.Error(), "255") {
		t.Fatalf("error leaked rejected input: %v", err)
	}
}

func TestGenerateUniqueCodesRejectsDuplicateReaderWithinFiniteBudget(t *testing.T) {
	codes, err := generateUniqueCodes(repeatedByteReader{value: 0}, 2)
	if !errors.Is(err, ErrCodeUniquenessExhausted) || codes != nil {
		t.Fatalf("generateUniqueCodes=(%v,%v)", codes, err)
	}
}

func TestNormalizeCodeAcceptsOnlyApprovedASCIISyntax(t *testing.T) {
	want := "ZHR-2345-6789-ABCD-EFGH-JKMN"
	valid := []string{
		want,
		"zhr-2345-6789-abcd-efgh-jkmn",
		" zhr 2345 6789 abcd efgh jkmn ",
		"ZHR23456789ABCDEFGHJKMN",
	}
	for _, raw := range valid {
		got, err := NormalizeCode(raw)
		if err != nil || got != want {
			t.Fatalf("NormalizeCode(valid)=(%q,%v), want %q", got, err, want)
		}
	}

	invalid := []string{
		"",
		"ZHR-2345-6789-ABCD-EFGH-JKMO", // O is excluded.
		"ZHR-2345-6789-ABCD-EFGH-JKM1", // 1 is excluded.
		"ZHR_2345_6789_ABCD_EFGH_JKMN",
		"X-ZHR-2345-6789-ABCD-EFGH-JKMN",
		"ZHR-2345-6789-ABCD-EFGH-JKMN\n",
		"ZHR-2345-6789-ABCD-EFGH-JKMＮ", // Unicode full-width homograph.
		strings.Repeat("Z", 129),
	}
	for _, raw := range invalid {
		got, err := NormalizeCode(raw)
		if !errors.Is(err, ErrInvalidCode) || got != "" {
			t.Fatalf("NormalizeCode(invalid)=(%q,%v)", got, err)
		}
		if raw != "" && strings.Contains(err.Error(), raw) {
			t.Fatalf("error leaked raw code")
		}
	}
}

func TestParseAmountCentsUsesStrictASCIIIntegerParsing(t *testing.T) {
	tests := []struct {
		raw       string
		wantCents int64
		wantText  string
	}{
		{raw: "0", wantCents: 0, wantText: "0.00"},
		{raw: "1", wantCents: 100, wantText: "1.00"},
		{raw: "0.1", wantCents: 10, wantText: "0.10"},
		{raw: "12.34", wantCents: 1234, wantText: "12.34"},
		{raw: "1000000.00", wantCents: 100_000_000, wantText: "1000000.00"},
	}
	for _, test := range tests {
		cents, text, err := ParseAmountCents(test.raw)
		if err != nil || cents != test.wantCents || text != test.wantText {
			t.Fatalf("ParseAmountCents(%q)=(%d,%q,%v)", test.raw, cents, text, err)
		}
	}

	invalid := []string{"", " 1", "1 ", "+1", "-1", ".5", "1.", "01", "1.000", "1e2", "1,000", "１.００", strings.Repeat("9", 64)}
	for _, raw := range invalid {
		cents, text, err := ParseAmountCents(raw)
		if !errors.Is(err, ErrInvalidAmount) || cents != 0 || text != "" {
			t.Fatalf("ParseAmountCents(%q)=(%d,%q,%v)", raw, cents, text, err)
		}
	}
}

type countingReader struct {
	*bytes.Reader
	reads int
}

func (reader *countingReader) Read(buffer []byte) (int, error) {
	n, err := reader.Reader.Read(buffer)
	reader.reads += n
	return n, err
}

type repeatedByteReader struct{ value byte }

func (reader repeatedByteReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = reader.value
	}
	return len(buffer), nil
}
