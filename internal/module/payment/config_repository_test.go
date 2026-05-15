package payment

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfigTableName(t *testing.T) {
	if (Config{}).TableName() != "payment_configs" {
		t.Fatalf("unexpected table name: %s", (Config{}).TableName())
	}
}

func TestConfigListQueryDefaults(t *testing.T) {
	query := ConfigListQuery{}
	page, size, offset := normalizePage(query.CurrentPage, query.PageSize)
	if page != 1 || size != 20 || offset != 0 {
		t.Fatalf("unexpected page defaults: page=%d size=%d offset=%d", page, size, offset)
	}
}

func TestPaymentConfigContractDoesNotCarryReturnURL(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(Config{}),
		reflect.TypeOf(configMutationRequest{}),
		reflect.TypeOf(ConfigMutationInput{}),
		reflect.TypeOf(ConfigListItem{}),
	} {
		if _, ok := typ.FieldByName("ReturnURL"); ok {
			t.Fatalf("%s must not carry ReturnURL; return_url belongs to each payment request", typ.Name())
		}
	}

	for _, migration := range []string{
		"20260515_payment_config_rebuild_v1.sql",
		"20260515_payment_config_naming_canonicalization.sql",
	} {
		content, err := os.ReadFile(filepath.Join("..", "..", "..", "database", "migrations", migration))
		if err != nil {
			t.Fatalf("read migration %s: %v", migration, err)
		}
		if strings.Contains(string(content), "return_url") {
			t.Fatalf("%s must not define/copy return_url for payment_configs", migration)
		}
	}
}
