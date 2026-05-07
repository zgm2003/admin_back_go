package enum

import "testing"

func TestExportTaskStatusLabelsAndValidation(t *testing.T) {
	cases := map[int]string{
		ExportTaskStatusPending: "处理中",
		ExportTaskStatusSuccess: "已完成",
		ExportTaskStatusFailed:  "失败",
	}

	for status, label := range cases {
		if !IsExportTaskStatus(status) {
			t.Fatalf("expected status %d to be valid", status)
		}
		if got := ExportTaskStatusLabels[status]; got != label {
			t.Fatalf("expected status %d label %q, got %q", status, label, got)
		}
	}

	if IsExportTaskStatus(0) {
		t.Fatalf("status 0 must be invalid")
	}
	if IsExportTaskStatus(4) {
		t.Fatalf("status 4 must be invalid")
	}
}
