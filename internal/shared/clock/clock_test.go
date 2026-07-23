package clock

import (
	"testing"
	"time"
)

func TestFunc(t *testing.T) {
	want := time.Date(2026, 7, 23, 10, 11, 12, 345000000, time.UTC)
	if got := Func(func() time.Time { return want }).Now(); !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := (Func(nil)).Now(); got.IsZero() {
		t.Fatal("nil Func returned zero time")
	}
}

func TestSystemClock(t *testing.T) {
	before := time.Now()
	got := (SystemClock{}).Now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Fatalf("clock returned %v outside [%v, %v]", got, before, after)
	}
}
