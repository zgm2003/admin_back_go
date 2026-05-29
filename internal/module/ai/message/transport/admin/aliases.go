package admin

import aimessagemodule "admin_back_go/internal/module/ai/message"

type (
	ListQuery      = aimessagemodule.ListQuery
	SendInput      = aimessagemodule.SendInput
	CancelInput    = aimessagemodule.CancelInput
	Attachment     = aimessagemodule.Attachment
	SendRecord     = aimessagemodule.SendRecord
	ReplyPayload   = aimessagemodule.ReplyPayload
	ReplyEnqueuer  = aimessagemodule.ReplyEnqueuer
	ReplyCanceler  = aimessagemodule.ReplyCanceler
	MessageItem    = aimessagemodule.MessageItem
	ListResponse   = aimessagemodule.ListResponse
	SendResponse   = aimessagemodule.SendResponse
	CancelResponse = aimessagemodule.CancelResponse
	AgentRuntime   = aimessagemodule.AgentRuntime
	Repository     = aimessagemodule.Repository
	HTTPService    = aimessagemodule.HTTPService
	Conversation   = aimessagemodule.Conversation
	Message        = aimessagemodule.Message
	GormRepository = aimessagemodule.GormRepository
	Service        = aimessagemodule.Service
	Option         = aimessagemodule.Option
)
