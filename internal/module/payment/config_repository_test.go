package payment

import "testing"

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
