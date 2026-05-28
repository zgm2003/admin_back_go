package admin

import queuemonitormodule "admin_back_go/internal/module/queuemonitor"

const (
	UIPath           = queuemonitormodule.UIPath
	ErrUIUnavailable = queuemonitormodule.ErrUIUnavailable
)

type (
	MonitorUI          = queuemonitormodule.MonitorUI
	QueueItem          = queuemonitormodule.QueueItem
	FailedListQuery    = queuemonitormodule.FailedListQuery
	Page               = queuemonitormodule.Page
	FailedListResponse = queuemonitormodule.FailedListResponse
	FailedTaskItem     = queuemonitormodule.FailedTaskItem
	Inspector          = queuemonitormodule.Inspector
	QueueSnapshot      = queuemonitormodule.QueueSnapshot
	TaskSnapshot       = queuemonitormodule.TaskSnapshot
	Options            = queuemonitormodule.Options
	Service            = queuemonitormodule.Service
)
