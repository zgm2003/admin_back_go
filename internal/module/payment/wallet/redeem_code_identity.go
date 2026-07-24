package wallet

import "time"

type RedeemCodeCreditIdentity struct {
	transactionNo string
	self          *RedeemCodeCreditIdentity
}

func NewRedeemCodeCreditIdentity(now time.Time) *RedeemCodeCreditIdentity {
	identity := &RedeemCodeCreditIdentity{transactionNo: newTransactionNo(now)}
	identity.self = identity
	return identity
}

func (identity *RedeemCodeCreditIdentity) TransactionNo() string {
	if !identity.valid() {
		return ""
	}
	return identity.transactionNo
}

func (identity *RedeemCodeCreditIdentity) valid() bool {
	return identity != nil && identity.self == identity && identity.transactionNo != ""
}

func (identity *RedeemCodeCreditIdentity) rotate(now time.Time) {
	identity.transactionNo = newTransactionNo(now)
}
