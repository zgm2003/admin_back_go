package money

import (
	"math"
	"testing"
)

func TestCentsToUnitsChecksRange(t *testing.T) {
	if got, err := CentsToUnits(1); err != nil || got != UnitsPerCent {
		t.Fatalf("one cent: got=%d err=%v", got, err)
	}
	if _, err := CentsToUnits(-1); err == nil {
		t.Fatal("negative cents must be rejected")
	}
	if _, err := CentsToUnits(math.MaxInt64/UnitsPerCent + 1); err == nil {
		t.Fatal("overflowing cents must be rejected")
	}
}

func TestFormatRMBUnits(t *testing.T) {
	tests := []struct {
		name  string
		units int64
		want  string
	}{
		{name: "zero", units: 0, want: "0"},
		{name: "one unit", units: 1, want: "0.00000001"},
		{name: "one cent", units: UnitsPerCent, want: "0.01"},
		{name: "one RMB", units: UnitsPerRMB, want: "1"},
		{name: "trailing zeros", units: 500000000, want: "5"},
		{name: "mixed", units: 123450000, want: "1.2345"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatRMBUnits(tt.units)
			if err != nil || got != tt.want {
				t.Fatalf("got=%q want=%q err=%v", got, tt.want, err)
			}
		})
	}
	if _, err := FormatRMBUnits(-1); err == nil {
		t.Fatal("negative units must be rejected")
	}
}
