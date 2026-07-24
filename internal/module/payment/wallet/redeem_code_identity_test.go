package wallet

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestRedeemCodeCreditInputHasOriginalFourPublicFields(t *testing.T) {
	typeOfInput := reflect.TypeOf(RedeemCodeCreditInput{})
	if typeOfInput.NumField() != 4 {
		t.Fatalf("RedeemCodeCreditInput field count=%d want 4", typeOfInput.NumField())
	}
	want := []string{"UserID", "CodeID", "AmountCents", "BatchNo"}
	for index, name := range want {
		field := typeOfInput.Field(index)
		if field.Name != name || field.PkgPath != "" {
			t.Fatalf("RedeemCodeCreditInput field %d=%s exported=%t want exported %s", index, field.Name, field.PkgPath == "", name)
		}
	}
}

func TestNewRedeemCodeCreditIdentityIsOpaqueAndInitialized(t *testing.T) {
	input := RedeemCodeCreditInput{UserID: 7, CodeID: 88, AmountCents: 100, BatchNo: "  RCB202607240001  "}
	identity := NewRedeemCodeCreditIdentity(input, time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC))
	if identity == nil || identity.TransactionNo() == "" {
		t.Fatalf("constructor returned uninitialized identity=%#v", identity)
	}
	if len(identity.transactionNos) != maxTransactionNoInsertAttempts {
		t.Fatalf("candidate count=%d want %d", len(identity.transactionNos), maxTransactionNoInsertAttempts)
	}
	seen := make(map[string]struct{}, len(identity.transactionNos))
	for _, transactionNo := range identity.transactionNos {
		if transactionNo == "" || !identity.Matches(transactionNo) {
			t.Fatalf("identity does not own candidate %q", transactionNo)
		}
		if _, exists := seen[transactionNo]; exists {
			t.Fatalf("duplicate identity candidate %q", transactionNo)
		}
		seen[transactionNo] = struct{}{}
	}
	typeOfIdentity := reflect.TypeOf(*identity)
	for index := 0; index < typeOfIdentity.NumField(); index++ {
		if typeOfIdentity.Field(index).PkgPath == "" {
			t.Fatalf("identity field %q must remain private", typeOfIdentity.Field(index).Name)
		}
	}
}

func TestRedeemCodeCreditIdentityRejectsStructuralCopyAndSupportsConcurrentReads(t *testing.T) {
	input := RedeemCodeCreditInput{UserID: 7, CodeID: 88, AmountCents: 100, BatchNo: "RCB202607240001"}
	identity := NewRedeemCodeCreditIdentity(input, time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC))
	transactionNo := identity.TransactionNo()
	copied := *identity
	if copied.TransactionNo() != "" || copied.Matches(transactionNo) {
		t.Fatalf("structural copy retained identity authority: %#v", copied)
	}

	const readers = 32
	const readsPerReader = 100
	errs := make(chan string, readers)
	var wait sync.WaitGroup
	for reader := 0; reader < readers; reader++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for read := 0; read < readsPerReader; read++ {
				if identity.TransactionNo() != transactionNo || !identity.Matches(transactionNo) {
					errs <- "identity read changed"
					return
				}
			}
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
