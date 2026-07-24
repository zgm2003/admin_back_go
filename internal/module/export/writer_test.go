package exporttask

import (
	"bytes"
	"encoding/csv"
	"io"
	"strings"
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

func TestCSVWriterUsesBOMCRLFAndNeutralizesFormulaCells(t *testing.T) {
	body, err := (CSVWriter{}).Write(FileData{
		Headers: []Column{{Key: "code", Title: "Code"}, {Key: "note", Title: "Note"}},
		Rows: []map[string]string{
			{"code": "ZHR-2222-2222-2222-2222-2222", "note": "  =SUM(A1:A2)"},
			{"code": "+cmd", "note": "plain,quoted"},
		},
	})
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if !bytes.HasPrefix(body, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatalf("CSV must start with UTF-8 BOM: %x", body[:min(3, len(body))])
	}
	text := string(body[3:])
	if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") || !strings.Contains(text, "\r\n") {
		t.Fatalf("CSV must use CRLF only: %q", text)
	}

	reader := csv.NewReader(strings.NewReader(text))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	want := [][]string{
		{"Code", "Note"},
		{"ZHR-2222-2222-2222-2222-2222", "'  =SUM(A1:A2)"},
		{"'+cmd", "plain,quoted"},
	}
	if len(records) != len(want) {
		t.Fatalf("records=%v", records)
	}
	for row := range want {
		for column := range want[row] {
			if records[row][column] != want[row][column] {
				t.Fatalf("record[%d][%d]=%q want %q", row, column, records[row][column], want[row][column])
			}
		}
	}
}

func TestCSVWriterRejectsInvalidHeaders(t *testing.T) {
	tests := []FileData{
		{},
		{Headers: []Column{{Key: "", Title: "Code"}}},
		{Headers: []Column{{Key: "code", Title: ""}}},
		{Headers: []Column{{Key: "code", Title: "Code"}, {Key: "code", Title: "Again"}}},
	}
	for _, data := range tests {
		if body, err := (CSVWriter{}).Write(data); err == nil || body != nil {
			t.Fatalf("Write(%+v)=(%q,%v), want nil error", data.Headers, body, err)
		}
	}
}

func TestCSVWriterPropagatesCSVWriteFailure(t *testing.T) {
	writer := CSVWriter{newWriter: func(io.Writer) *csv.Writer {
		return csv.NewWriter(failingWriter{})
	}}
	if _, err := writer.Write(FileData{Headers: []Column{{Key: "code", Title: "Code"}}}); err == nil {
		t.Fatal("Write error=nil")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
