package payment

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPaymentModelsUseNewTables(t *testing.T) {
	if (Channel{}).TableName() != "payment_channels" {
		t.Fatalf("unexpected channel table")
	}
	if (ChannelConfig{}).TableName() != "payment_channel_configs" {
		t.Fatalf("unexpected config table")
	}
	if (Order{}).TableName() != "payment_orders" {
		t.Fatalf("unexpected order table")
	}
	if (Event{}).TableName() != "payment_events" {
		t.Fatalf("unexpected event table")
	}
}

func TestPaymentPaginationDefaults(t *testing.T) {
	page, size, offset := normalizePage(0, -1)
	if page != 1 || size != 20 || offset != 0 {
		t.Fatalf("unexpected defaults page=%d size=%d offset=%d", page, size, offset)
	}

	page, size, offset = normalizePage(3, 10)
	if page != 3 || size != 10 || offset != 20 {
		t.Fatalf("unexpected pagination page=%d size=%d offset=%d", page, size, offset)
	}
}

func TestNormalizeLimitCapsAndDefaults(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "zero uses default", limit: 0, want: defaultPageSize},
		{name: "negative uses default", limit: -10, want: defaultPageSize},
		{name: "positive unchanged", limit: 7, want: 7},
		{name: "max unchanged", limit: maxPageSize, want: maxPageSize},
		{name: "above max capped", limit: maxPageSize + 1, want: maxPageSize},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeLimit(tc.limit); got != tc.want {
				t.Fatalf("normalizeLimit(%d)=%d, want %d", tc.limit, got, tc.want)
			}
		})
	}
}

func TestNilRepositoryReturnsNotConfigured(t *testing.T) {
	var repo *GormRepository
	_, _, err := repo.ListChannels(context.Background(), ChannelListQuery{})
	if !errors.Is(err, ErrRepositoryNotConfigured) {
		t.Fatalf("ListChannels error=%v, want %v", err, ErrRepositoryNotConfigured)
	}
}

func TestOrderOutTradeNoZeroValueIsNullSemantic(t *testing.T) {
	if (Order{}).OutTradeNo != nil {
		t.Fatalf("new unpaid order should keep out_trade_no nil for DB NULL semantics")
	}
}

func TestMarkOrderPayingRequiresOutTradeNoBeforeDB(t *testing.T) {
	repo := NewGormRepository(nil)

	err := repo.MarkOrderPaying(context.Background(), 1, " \t\n ", "", "", time.Time{})
	if !errors.Is(err, ErrOutTradeNoRequired) {
		t.Fatalf("MarkOrderPaying error=%v, want %v", err, ErrOutTradeNoRequired)
	}
}
