package admin

import aiconversationmodule "admin_back_go/internal/module/aiconversation"

type (
	ListQuery          = aiconversationmodule.ListQuery
	ListRow            = aiconversationmodule.ListRow
	ConversationItem   = aiconversationmodule.ConversationItem
	ConversationDetail = aiconversationmodule.ConversationDetail
	ListResponse       = aiconversationmodule.ListResponse
	CreateInput        = aiconversationmodule.CreateInput
	UpdateInput        = aiconversationmodule.UpdateInput
	CreateResponse     = aiconversationmodule.CreateResponse
	Repository         = aiconversationmodule.Repository
	HTTPService        = aiconversationmodule.HTTPService
	Conversation       = aiconversationmodule.Conversation
	GormRepository     = aiconversationmodule.GormRepository
	Service            = aiconversationmodule.Service
)
