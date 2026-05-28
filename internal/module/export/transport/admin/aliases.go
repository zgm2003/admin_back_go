package admin

import exporttaskmodule "admin_back_go/internal/module/export"

type (
	StatusCountQuery           = exporttaskmodule.StatusCountQuery
	StatusCountItem            = exporttaskmodule.StatusCountItem
	ListQuery                  = exporttaskmodule.ListQuery
	Page                       = exporttaskmodule.Page
	ListResponse               = exporttaskmodule.ListResponse
	ListItem                   = exporttaskmodule.ListItem
	CreatePendingInput         = exporttaskmodule.CreatePendingInput
	CreatePendingResponse      = exporttaskmodule.CreatePendingResponse
	DeleteInput                = exporttaskmodule.DeleteInput
	SuccessResult              = exporttaskmodule.SuccessResult
	HTTPService                = exporttaskmodule.HTTPService
	RunPayload                 = exporttaskmodule.RunPayload
	RunInput                   = exporttaskmodule.RunInput
	JobService                 = exporttaskmodule.JobService
	Task                       = exporttaskmodule.Task
	NotificationTaskCreator    = exporttaskmodule.NotificationTaskCreator
	NotificationTaskNotifier   = exporttaskmodule.NotificationTaskNotifier
	Repository                 = exporttaskmodule.Repository
	GormRepository             = exporttaskmodule.GormRepository
	Service                    = exporttaskmodule.Service
	Option                     = exporttaskmodule.Option
	ExportDataProvider         = exporttaskmodule.ExportDataProvider
	FileWriter                 = exporttaskmodule.FileWriter
	FileUploader               = exporttaskmodule.FileUploader
	Notifier                   = exporttaskmodule.Notifier
	NotifyInput                = exporttaskmodule.NotifyInput
	UploadConfig               = exporttaskmodule.UploadConfig
	UploadConfigRepository     = exporttaskmodule.UploadConfigRepository
	GormUploadConfigRepository = exporttaskmodule.GormUploadConfigRepository
	SecretDecrypter            = exporttaskmodule.SecretDecrypter
	UploadInput                = exporttaskmodule.UploadInput
	UploadResult               = exporttaskmodule.UploadResult
	COSUploader                = exporttaskmodule.COSUploader
	UploadOption               = exporttaskmodule.UploadOption
	Column                     = exporttaskmodule.Column
	FileData                   = exporttaskmodule.FileData
	XLSXWriter                 = exporttaskmodule.XLSXWriter
)
