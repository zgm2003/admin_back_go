package payment

import "testing"

func TestAlipayConfigTableName(t *testing.T) {
	if (AlipayConfig{}).TableName() != "payment_alipay_configs" {
		t.Fatalf("unexpected table name: %s", (AlipayConfig{}).TableName())
	}
}

func TestConfigListQueryDefaults(t *testing.T) {
	query := ConfigListQuery{}
	page, size, offset := normalizePage(query.CurrentPage, query.PageSize)
	if page != 1 || size != 20 || offset != 0 {
		t.Fatalf("unexpected page defaults: page=%d size=%d offset=%d", page, size, offset)
	}
}
