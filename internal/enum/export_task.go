package enum

const (
	ExportTaskStatusPending = 1
	ExportTaskStatusSuccess = 2
	ExportTaskStatusFailed  = 3
)

var ExportTaskStatusLabels = map[int]string{
	ExportTaskStatusPending: "处理中",
	ExportTaskStatusSuccess: "已完成",
	ExportTaskStatusFailed:  "失败",
}

func IsExportTaskStatus(value int) bool {
	_, ok := ExportTaskStatusLabels[value]
	return ok
}
