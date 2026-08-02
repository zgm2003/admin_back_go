package ai

import (
	"encoding/json"
	"testing"
)

func TestUTF8BytesTokenCounterReturnsConservativeByteBounds(t *testing.T) {
	counter, err := ResolveTokenCounter(TokenCounterUTF8BytesV1)
	if err != nil {
		t.Fatal(err)
	}
	if counter.ID() != TokenCounterUTF8BytesV1 {
		t.Fatalf("counter ID = %q", counter.ID())
	}
	for _, test := range []struct {
		name string
		text string
		want int64
	}{
		{name: "ascii", text: "hello", want: 5},
		{name: "utf8", text: "上下文", want: 9},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := counter.UpperBoundText(test.text)
			if err != nil || got != test.want {
				t.Fatalf("UpperBoundText() = %d, %v; want %d", got, err, test.want)
			}
		})
	}

	raw := json.RawMessage(`{"message":"上下文"}`)
	got, err := counter.UpperBoundJSON(raw)
	if err != nil || got != int64(len(raw)) {
		t.Fatalf("UpperBoundJSON() = %d, %v; want %d", got, err, len(raw))
	}
}

func TestTokenCounterRejectsUnknownIDsAndInvalidInputs(t *testing.T) {
	if _, err := ResolveTokenCounter("unknown_v1"); err == nil {
		t.Fatal("unknown counter ID was accepted")
	}
	counter, err := ResolveTokenCounter(TokenCounterUTF8BytesV1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := counter.UpperBoundText(string([]byte{0xff})); err == nil {
		t.Fatal("invalid UTF-8 text was accepted")
	}
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`{"a":1} trailing`), {'"', 0xff, '"'}} {
		if _, err := counter.UpperBoundJSON(raw); err == nil {
			t.Fatalf("invalid JSON %q was accepted", raw)
		}
	}
}
