package wallet

import (
	"reflect"
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
	identity := NewRedeemCodeCreditIdentity(time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC))
	if identity == nil || identity.TransactionNo() == "" {
		t.Fatalf("constructor returned uninitialized identity=%#v", identity)
	}
	typeOfIdentity := reflect.TypeOf(*identity)
	for index := 0; index < typeOfIdentity.NumField(); index++ {
		if typeOfIdentity.Field(index).PkgPath == "" {
			t.Fatalf("identity field %q must remain private", typeOfIdentity.Field(index).Name)
		}
	}
}
