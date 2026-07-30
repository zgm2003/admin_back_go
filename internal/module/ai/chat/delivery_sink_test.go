package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	infrarealtime "admin_back_go/internal/infra/realtime"
)

type deliveryCommitterFunc func(context.Context, DeliveryCommit) (uint32, bool, error)

func (f deliveryCommitterFunc) CommitDelivery(ctx context.Context, input DeliveryCommit) (uint32, bool, error) {
	return f(ctx, input)
}

type deliveryPublisherFunc func(context.Context, infrarealtime.Publication) error

func (f deliveryPublisherFunc) Publish(ctx context.Context, input infrarealtime.Publication) error {
	return f(ctx, input)
}

func TestPersistentDeliverySinkCommitsBeforePublishing(t *testing.T) {
	order := make([]string, 0, 2)
	committer := deliveryCommitterFunc(func(_ context.Context, input DeliveryCommit) (uint32, bool, error) {
		order = append(order, "commit")
		if input.Delta != "12" {
			t.Fatalf("delta=%q", input.Delta)
		}
		return 1, true, nil
	})
	publisher := deliveryPublisherFunc(func(_ context.Context, publication infrarealtime.Publication) error {
		order = append(order, "publish")
		if publication.UserID != 7 || publication.Envelope.Type != EventAIResponseDelta {
			t.Fatalf("publication=%+v", publication)
		}
		var payload DeltaPayload
		if err := json.Unmarshal(publication.Envelope.Data, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.DeliverySeq != 1 || payload.Delta != "12" {
			t.Fatalf("payload=%+v", payload)
		}
		return nil
	})
	sink := newDeliverySink(testDeliverySinkOptions(committer, publisher))
	defer sink.Close(context.Background())

	if err := sink.Accept("1"); err != nil {
		t.Fatal(err)
	}
	if err := sink.Accept("2"); err != nil {
		t.Fatal(err)
	}
	if err := sink.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "commit" || order[1] != "publish" {
		t.Fatalf("unexpected delivery order: %v", order)
	}
}

func TestPersistentDeliverySinkDoesNotPublishCommitFailure(t *testing.T) {
	want := errors.New("database unavailable")
	published := 0
	sink := newDeliverySink(testDeliverySinkOptions(
		deliveryCommitterFunc(func(context.Context, DeliveryCommit) (uint32, bool, error) {
			return 0, false, want
		}),
		deliveryPublisherFunc(func(context.Context, infrarealtime.Publication) error {
			published++
			return nil
		}),
	))
	defer sink.Close(context.Background())

	if err := sink.Accept("hello"); err != nil {
		t.Fatal(err)
	}
	err := sink.Flush(context.Background())
	if !errors.Is(err, want) || !infraai.IsFatalEventSinkError(err) {
		t.Fatalf("err=%v", err)
	}
	if published != 0 {
		t.Fatalf("published=%d", published)
	}
}

func TestSplitDeliveryUTF8PreservesBytesAt16KiBBoundary(t *testing.T) {
	value := strings.Repeat("a", maxDeliveryBytes-1) + "你" + strings.Repeat("界", maxDeliveryBytes/3+1)
	parts := splitDeliveryUTF8(value, maxDeliveryBytes)
	if len(parts) < 2 || strings.Join(parts, "") != value {
		t.Fatalf("parts=%d preserved=%v", len(parts), strings.Join(parts, "") == value)
	}
	for index, part := range parts {
		if part == "" || len(part) > maxDeliveryBytes {
			t.Fatalf("part[%d] bytes=%d", index, len(part))
		}
	}
}

func TestPersistentDeliverySinkSplitsLargeDeltaBeforeCommit(t *testing.T) {
	value := strings.Repeat("界", maxDeliveryBytes)
	committed := make([]string, 0, 4)
	sink := newDeliverySink(testDeliverySinkOptions(
		deliveryCommitterFunc(func(_ context.Context, input DeliveryCommit) (uint32, bool, error) {
			committed = append(committed, input.Delta)
			return uint32(len(committed)), true, nil
		}),
		deliveryPublisherFunc(func(context.Context, infrarealtime.Publication) error { return nil }),
	))
	defer sink.Close(context.Background())

	if err := sink.Accept(value); err != nil {
		t.Fatal(err)
	}
	if err := sink.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(committed, "") != value {
		t.Fatal("committed chunks did not preserve the original UTF-8 bytes")
	}
	for index, part := range committed {
		if len(part) > maxDeliveryBytes {
			t.Fatalf("chunk[%d] bytes=%d", index, len(part))
		}
	}
}

func TestDeliveryStopDiscardsBuffer(t *testing.T) {
	commits := 0
	sink := newDeliverySink(testDeliverySinkOptions(
		deliveryCommitterFunc(func(context.Context, DeliveryCommit) (uint32, bool, error) {
			commits++
			return uint32(commits), true, nil
		}),
		deliveryPublisherFunc(func(context.Context, infrarealtime.Publication) error { return nil }),
	))
	defer sink.Close(context.Background())

	if err := sink.Accept("not committed yet"); err != nil {
		t.Fatal(err)
	}
	if err := sink.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := sink.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if commits != 0 {
		t.Fatalf("commits=%d", commits)
	}
}

func testDeliverySinkOptions(committer DeliveryCommitter, publisher infrarealtime.Publisher) deliverySinkOptions {
	return deliverySinkOptions{
		Committer:      committer,
		Publisher:      publisher,
		CommandID:      41,
		Owner:          "worker-a",
		Token:          7,
		ConversationID: 3,
		UserID:         7,
		RequestID:      "request-1",
		MaxWait:        50 * time.Millisecond,
		Now:            func() time.Time { return time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC) },
	}
}
