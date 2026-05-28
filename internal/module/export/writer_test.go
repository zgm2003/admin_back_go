package exporttask

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestXLSXWriterWritesIDAndPhoneAsStrings(t *testing.T) {
	body, err := (XLSXWriter{}).Write(FileData{
		Headers: []Column{{Key: "id", Title: "用户ID"}, {Key: "phone", Title: "手机号"}},
		Rows: []map[string]string{{
			"id":    "100000000000000001",
			"phone": "15671628271",
		}},
	})
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	file, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	defer file.Close()
	id, err := file.GetCellValue("Sheet1", "A2")
	if err != nil {
		t.Fatalf("read id: %v", err)
	}
	phone, err := file.GetCellValue("Sheet1", "B2")
	if err != nil {
		t.Fatalf("read phone: %v", err)
	}
	if id != "100000000000000001" || phone != "15671628271" {
		t.Fatalf("expected string values preserved, id=%q phone=%q", id, phone)
	}
}
