package wallet

import (
	"strings"
	"time"
)

type redeemCodeCreditBinding struct {
	userID      int64
	codeID      int64
	amountUnits int64
	batchNo     string
}

type RedeemCodeCreditIdentity struct {
	binding        redeemCodeCreditBinding
	transactionNos [maxTransactionNoInsertAttempts]string
	self           *RedeemCodeCreditIdentity
}

func NewRedeemCodeCreditIdentity(input RedeemCodeCreditInput, now time.Time) *RedeemCodeCreditIdentity {
	input = normalizeRedeemCodeCreditInput(input)
	identity := &RedeemCodeCreditIdentity{binding: redeemCodeCreditBinding{
		userID: input.UserID, codeID: input.CodeID, amountUnits: input.AmountUnits, batchNo: input.BatchNo,
	}}
	for index := range identity.transactionNos {
		identity.transactionNos[index] = newTransactionNo(now)
	}
	identity.self = identity
	return identity
}

func (identity *RedeemCodeCreditIdentity) TransactionNo() string {
	if !identity.valid() {
		return ""
	}
	return identity.transactionNos[0]
}

func (identity *RedeemCodeCreditIdentity) Matches(transactionNo string) bool {
	if !identity.valid() || transactionNo == "" {
		return false
	}
	for _, candidate := range identity.transactionNos {
		if candidate == transactionNo {
			return true
		}
	}
	return false
}

func (identity *RedeemCodeCreditIdentity) valid() bool {
	if identity == nil || identity.self != identity {
		return false
	}
	seen := make(map[string]struct{}, len(identity.transactionNos))
	for _, transactionNo := range identity.transactionNos {
		if transactionNo == "" {
			return false
		}
		if _, exists := seen[transactionNo]; exists {
			return false
		}
		seen[transactionNo] = struct{}{}
	}
	return true
}

func (identity *RedeemCodeCreditIdentity) matchesInput(input RedeemCodeCreditInput) bool {
	if !identity.valid() {
		return false
	}
	input = normalizeRedeemCodeCreditInput(input)
	return identity.binding == (redeemCodeCreditBinding{
		userID: input.UserID, codeID: input.CodeID, amountUnits: input.AmountUnits, batchNo: input.BatchNo,
	})
}

func normalizeRedeemCodeCreditInput(input RedeemCodeCreditInput) RedeemCodeCreditInput {
	input.BatchNo = strings.TrimSpace(input.BatchNo)
	return input
}
