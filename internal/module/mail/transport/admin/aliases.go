package admin

import mailmodule "admin_back_go/internal/module/mail"

type (
	PageInitResponse  = mailmodule.PageInitResponse
	PageInitDict      = mailmodule.PageInitDict
	ConfigResponse    = mailmodule.ConfigResponse
	SaveConfigInput   = mailmodule.SaveConfigInput
	TestInput         = mailmodule.TestInput
	TemplateDTO       = mailmodule.TemplateDTO
	SaveTemplateInput = mailmodule.SaveTemplateInput
	LogQuery          = mailmodule.LogQuery
	LogListResponse   = mailmodule.LogListResponse
	LogDTO            = mailmodule.LogDTO
	Service           = mailmodule.Service
)
