package aibilling

import (
	"context"
	"errors"
	"testing"
	"time"

	walletmodule "admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

func TestChargeCreatesBillingRecordAndDebitsWalletWithRecordSource(t *testing.T) {
	now := time.Date(2026, 5, 30, 15, 0, 0, 0, time.UTC)
	repo := &fakeBillingRecordRepository{enabled: &Rule{ID: 11, Scene: SceneAdminImageGenerate, Unit: UnitImage, UnitPriceCents: 123, Status: RuleStatusEnabled, IsDel: enum.CommonNo}, nextRecordID: 501}
	wallet := &fakeBillingWallet{debitTransactionID: 9001}
	service := NewServiceWithWallet(repo, wallet, func() time.Time { return now })

	result, appErr := service.Charge(context.Background(), ChargeInput{RequestNo: "req-1", UserID: 7, Platform: "admin", Scene: SceneAdminImageGenerate, AgentID: 3, ProviderID: 4, ModelID: "gpt-image-2", UnitCount: 2, Remark: "AI图片生成"})

	if appErr != nil {
		t.Fatalf("Charge returned error: %#v", appErr)
	}
	if result == nil || result.RecordID != 501 || result.AmountCents != 246 || result.DebitTransactionID != 9001 {
		t.Fatalf("unexpected charge result: %#v", result)
	}
	if repo.created.Status != BillingStatusCharged || repo.created.Unit != UnitImage || repo.created.UnitCount != 2 || repo.created.UnitPriceCents != 123 || repo.created.AmountCents != 246 {
		t.Fatalf("billing snapshot mismatch: %#v", repo.created)
	}
	if wallet.debitInput.SourceType != walletmodule.SourceAIGenerate || wallet.debitInput.SourceID != 501 || wallet.debitInput.AmountCents != 246 {
		t.Fatalf("wallet debit source mismatch: %#v", wallet.debitInput)
	}
	if repo.updatedRecordID != 501 || repo.updatedFields["debit_transaction_id"] != int64(9001) {
		t.Fatalf("debit transaction was not bound back to billing record: id=%d fields=%#v", repo.updatedRecordID, repo.updatedFields)
	}
}

func TestChargeRejectsMissingEnabledRuleBeforeWalletDebit(t *testing.T) {
	repo := &fakeBillingRecordRepository{}
	wallet := &fakeBillingWallet{}
	service := NewServiceWithWallet(repo, wallet, fixedBillingNow)

	_, appErr := service.Charge(context.Background(), ChargeInput{RequestNo: "req-2", UserID: 7, Platform: "admin", Scene: SceneAdminImageGenerate, UnitCount: 1})

	if appErr == nil || appErr.MessageID != "aibilling.rule.not_configured" {
		t.Fatalf("expected missing rule keyed error, got %#v", appErr)
	}
	if wallet.debitCalled || repo.created.ID != 0 {
		t.Fatalf("missing rule must not create record or debit wallet: created=%#v debit=%v", repo.created, wallet.debitCalled)
	}
}

func TestChargeReturnsInsufficientBalanceBeforeProviderTaskIsCreated(t *testing.T) {
	repo := &fakeBillingRecordRepository{enabled: &Rule{ID: 11, Scene: SceneAdminImageGenerate, Unit: UnitImage, UnitPriceCents: 500, Status: RuleStatusEnabled, IsDel: enum.CommonNo}, nextRecordID: 502}
	wallet := &fakeBillingWallet{debitErr: apperror.BadRequestKey("wallet.debit.insufficient_balance", nil, "余额不足")}
	service := NewServiceWithWallet(repo, wallet, fixedBillingNow)

	_, appErr := service.Charge(context.Background(), ChargeInput{RequestNo: "req-3", UserID: 7, Platform: "admin", Scene: SceneAdminImageGenerate, UnitCount: 1})

	if appErr == nil || appErr.MessageID != "wallet.debit.insufficient_balance" {
		t.Fatalf("expected insufficient balance from wallet, got %#v", appErr)
	}
	if repo.created.ID == 0 || repo.created.Status != BillingStatusCharged {
		t.Fatalf("record must exist so wallet source_id is stable before debit: %#v", repo.created)
	}
}

func TestChargeDoesNotFailAfterDebitWhenDebitTransactionBindingFails(t *testing.T) {
	repo := &fakeBillingRecordRepository{
		enabled:      &Rule{ID: 11, Scene: SceneAdminImageGenerate, Unit: UnitImage, UnitPriceCents: 123, Status: RuleStatusEnabled, IsDel: enum.CommonNo},
		nextRecordID: 503,
		updateErr:    errors.New("billing update down"),
	}
	wallet := &fakeBillingWallet{debitTransactionID: 9002}
	service := NewServiceWithWallet(repo, wallet, fixedBillingNow)

	result, appErr := service.Charge(context.Background(), ChargeInput{RequestNo: "req-4", UserID: 7, Platform: "admin", Scene: SceneAdminImageGenerate, UnitCount: 1, Remark: "AI图片生成"})

	if appErr != nil {
		t.Fatalf("debit already succeeded; debit id binding failure must not orphan the charge: %#v", appErr)
	}
	if result == nil || result.RecordID != 503 || result.DebitTransactionID != 9002 {
		t.Fatalf("unexpected charge result after binding failure: %#v", result)
	}
	if wallet.debitInput.SourceType != walletmodule.SourceAIGenerate || wallet.debitInput.SourceID != 503 {
		t.Fatalf("wallet debit must still use billing record as source: %#v", wallet.debitInput)
	}
}

func TestMarkSuccessMovesChargedToSuccessAndSetsFinishedAt(t *testing.T) {
	repo := &fakeBillingRecordRepository{byID: &BillingRecord{ID: 601, Status: BillingStatusCharged}}
	service := NewServiceWithWallet(repo, &fakeBillingWallet{}, fixedBillingNow)

	appErr := service.MarkSuccess(context.Background(), 601)

	if appErr != nil {
		t.Fatalf("MarkSuccess returned error: %#v", appErr)
	}
	if repo.updatedFields["status"] != BillingStatusSuccess || repo.updatedFields["finished_at"] == nil {
		t.Fatalf("success fields mismatch: %#v", repo.updatedFields)
	}
}

func TestRefundMovesChargedOrFailedToRefundedOnceAndCreditsWallet(t *testing.T) {
	for _, status := range []string{BillingStatusCharged, BillingStatusFailed} {
		t.Run(status, func(t *testing.T) {
			debitID := int64(9001)
			repo := &fakeBillingRecordRepository{byID: &BillingRecord{ID: 701, UserID: 7, AmountCents: 246, Status: status, DebitTransactionID: &debitID}}
			wallet := &fakeBillingWallet{creditTransactionID: 9101}
			service := NewServiceWithWallet(repo, wallet, fixedBillingNow)

			appErr := service.Refund(context.Background(), RefundInput{BillingRecordID: 701, Reason: "图片生成任务入队失败"})

			if appErr != nil {
				t.Fatalf("Refund returned error: %#v", appErr)
			}
			if !wallet.creditCalled || wallet.creditInput.SourceType != walletmodule.SourceAIRefund || wallet.creditInput.SourceID != 701 || wallet.creditInput.AmountCents != 246 {
				t.Fatalf("wallet credit mismatch: %#v", wallet.creditInput)
			}
			if repo.updatedFields["status"] != BillingStatusRefunded || repo.updatedFields["refund_transaction_id"] != int64(9101) {
				t.Fatalf("refund fields mismatch: %#v", repo.updatedFields)
			}
		})
	}
}

func TestRefundIsIdempotentWhenRefundTransactionAlreadyExists(t *testing.T) {
	debitID := int64(9001)
	refundID := int64(9101)
	repo := &fakeBillingRecordRepository{byID: &BillingRecord{ID: 702, UserID: 7, AmountCents: 246, Status: BillingStatusRefunded, DebitTransactionID: &debitID, RefundTransactionID: &refundID}}
	wallet := &fakeBillingWallet{}
	service := NewServiceWithWallet(repo, wallet, fixedBillingNow)

	appErr := service.Refund(context.Background(), RefundInput{BillingRecordID: 702, Reason: "retry"})

	if appErr != nil {
		t.Fatalf("idempotent Refund returned error: %#v", appErr)
	}
	if wallet.creditCalled || repo.updatedRecordID != 0 {
		t.Fatalf("idempotent refund must not credit or update: credit=%v updateID=%d", wallet.creditCalled, repo.updatedRecordID)
	}
}

func TestRefundRecordsFailureWhenWalletCreditFails(t *testing.T) {
	debitID := int64(9001)
	repo := &fakeBillingRecordRepository{byID: &BillingRecord{ID: 703, UserID: 7, AmountCents: 246, Status: BillingStatusCharged, DebitTransactionID: &debitID}}
	wallet := &fakeBillingWallet{creditErr: apperror.InternalKey("wallet.mutation.failed", nil, "钱包资金变动失败")}
	service := NewServiceWithWallet(repo, wallet, fixedBillingNow)

	appErr := service.Refund(context.Background(), RefundInput{BillingRecordID: 703, Reason: "provider failed"})

	if appErr == nil || appErr.MessageID != "wallet.mutation.failed" {
		t.Fatalf("expected wallet credit error, got %#v", appErr)
	}
	if repo.updatedRecordID != 703 || repo.updatedFields["status"] != BillingStatusFailed || repo.updatedFields["refund_transaction_id"] != nil {
		t.Fatalf("refund failure must be durable without pretending refunded: id=%d fields=%#v", repo.updatedRecordID, repo.updatedFields)
	}
}

func fixedBillingNow() time.Time { return time.Date(2026, 5, 30, 15, 0, 0, 0, time.UTC) }

type fakeBillingRecordRepository struct {
	fakeRepository
	enabled         *Rule
	byID            *BillingRecord
	nextRecordID    int64
	created         BillingRecord
	updatedRecordID int64
	updatedFields   map[string]any
	updateErr       error
}

func (f *fakeBillingRecordRepository) EnabledByScene(ctx context.Context, scene string) (*Rule, error) {
	return f.enabled, nil
}
func (f *fakeBillingRecordRepository) CreateRecord(ctx context.Context, row BillingRecord) (int64, error) {
	if f.nextRecordID == 0 {
		f.nextRecordID = 1
	}
	row.ID = f.nextRecordID
	f.created = row
	return row.ID, nil
}
func (f *fakeBillingRecordRepository) GetRecord(ctx context.Context, id int64) (*BillingRecord, error) {
	return f.byID, nil
}
func (f *fakeBillingRecordRepository) UpdateRecord(ctx context.Context, id int64, fields map[string]any) error {
	f.updatedRecordID = id
	f.updatedFields = fields
	if f.updateErr != nil {
		return f.updateErr
	}
	return nil
}

type fakeBillingWallet struct {
	debitCalled         bool
	creditCalled        bool
	debitInput          walletmodule.MutationInput
	creditInput         walletmodule.MutationInput
	debitTransactionID  int64
	creditTransactionID int64
	debitErr            *apperror.Error
	creditErr           *apperror.Error
}

func (f *fakeBillingWallet) Debit(ctx context.Context, input walletmodule.MutationInput) (*walletmodule.MutationResponse, *apperror.Error) {
	f.debitCalled = true
	f.debitInput = input
	if f.debitErr != nil {
		return nil, f.debitErr
	}
	return &walletmodule.MutationResponse{Transaction: walletmodule.TransactionItem{ID: f.debitTransactionID}}, nil
}
func (f *fakeBillingWallet) Credit(ctx context.Context, input walletmodule.MutationInput) (*walletmodule.MutationResponse, *apperror.Error) {
	f.creditCalled = true
	f.creditInput = input
	if f.creditErr != nil {
		return nil, f.creditErr
	}
	return &walletmodule.MutationResponse{Transaction: walletmodule.TransactionItem{ID: f.creditTransactionID}}, nil
}
