package payment

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestUpdateOrderPaidUsesStatusCAS(t *testing.T) {
	repo, mock, closeDB := newPaymentMockRepository(t)
	defer closeDB()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_orders` SET") + ".*" + regexp.QuoteMeta("WHERE id = ? AND is_del = ? AND status IN (?,?,?)")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdateOrderPaid(context.Background(), 1, "202605302200", fixedOrderNow()); err != nil {
		t.Fatalf("UpdateOrderPaid error=%v", err)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestUpdateOrderPayingUsesStatusCAS(t *testing.T) {
	repo, mock, closeDB := newPaymentMockRepository(t)
	defer closeDB()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_orders` SET") + ".*" + regexp.QuoteMeta("WHERE id = ? AND is_del = ? AND status IN (?,?)")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdateOrderPaying(context.Background(), 1, "https://pay.example.test"); err != nil {
		t.Fatalf("UpdateOrderPaying error=%v", err)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestUpdateOrderPayingReturnsStateChangedOnCASMiss(t *testing.T) {
	repo, mock, closeDB := newPaymentMockRepository(t)
	defer closeDB()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_orders` SET") + ".*" + regexp.QuoteMeta("WHERE id = ? AND is_del = ? AND status IN (?,?)")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := repo.UpdateOrderPaying(context.Background(), 1, "https://pay.example.test"); !errors.Is(err, ErrPaymentStateChanged) {
		t.Fatalf("expected ErrPaymentStateChanged, got %v", err)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestUpdateOrderPaidReturnsStateChangedOnCASMiss(t *testing.T) {
	repo, mock, closeDB := newPaymentMockRepository(t)
	defer closeDB()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_orders` SET") + ".*" + regexp.QuoteMeta("WHERE id = ? AND is_del = ? AND status IN (?,?,?)")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := repo.UpdateOrderPaid(context.Background(), 1, "202605302200", fixedOrderNow()); !errors.Is(err, ErrPaymentStateChanged) {
		t.Fatalf("expected ErrPaymentStateChanged, got %v", err)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestUpdateOrderFailedUsesStatusCAS(t *testing.T) {
	repo, mock, closeDB := newPaymentMockRepository(t)
	defer closeDB()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_orders` SET") + ".*" + regexp.QuoteMeta("WHERE id = ? AND is_del = ? AND status IN (?,?)")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdateOrderFailed(context.Background(), 1, "gateway down"); err != nil {
		t.Fatalf("UpdateOrderFailed error=%v", err)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestUpdateOrderClosedUsesStatusCAS(t *testing.T) {
	repo, mock, closeDB := newPaymentMockRepository(t)
	defer closeDB()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_orders` SET") + ".*" + regexp.QuoteMeta("WHERE id = ? AND is_del = ? AND status IN (?,?,?)")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdateOrderClosed(context.Background(), 1, fixedOrderNow()); err != nil {
		t.Fatalf("UpdateOrderClosed error=%v", err)
	}
	assertPaymentMockExpectations(t, mock)
}
