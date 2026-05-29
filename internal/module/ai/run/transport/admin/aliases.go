package admin

import airunmodule "admin_back_go/internal/module/ai/run"

type (
	JSONObject             = airunmodule.JSONObject
	InitResponse           = airunmodule.InitResponse
	InitDict               = airunmodule.InitDict
	ListQuery              = airunmodule.ListQuery
	Page                   = airunmodule.Page
	ListResponse           = airunmodule.ListResponse
	ListItem               = airunmodule.ListItem
	MessageSummary         = airunmodule.MessageSummary
	EventItem              = airunmodule.EventItem
	ToolCallItem           = airunmodule.ToolCallItem
	KnowledgeRetrievalItem = airunmodule.KnowledgeRetrievalItem
	KnowledgeHitItem       = airunmodule.KnowledgeHitItem
	DetailResponse         = airunmodule.DetailResponse
	StatsFilter            = airunmodule.StatsFilter
	StatsResponse          = airunmodule.StatsResponse
	DateRange              = airunmodule.DateRange
	StatsSummary           = airunmodule.StatsSummary
	StatsMetricItem        = airunmodule.StatsMetricItem
	StatsByDateItem        = airunmodule.StatsByDateItem
	StatsByAgentItem       = airunmodule.StatsByAgentItem
	StatsByUserItem        = airunmodule.StatsByUserItem
	StatsByDateResponse    = airunmodule.StatsByDateResponse
	StatsByAgentResponse   = airunmodule.StatsByAgentResponse
	StatsByUserResponse    = airunmodule.StatsByUserResponse
	OptionRow              = airunmodule.OptionRow
	ListRow                = airunmodule.ListRow
	RunDetailRow           = airunmodule.RunDetailRow
	EventRow               = airunmodule.EventRow
	ToolCallRow            = airunmodule.ToolCallRow
	KnowledgeRetrievalRow  = airunmodule.KnowledgeRetrievalRow
	KnowledgeHitRow        = airunmodule.KnowledgeHitRow
	StatsSummaryRow        = airunmodule.StatsSummaryRow
	StatsMetricRow         = airunmodule.StatsMetricRow
	StatsListQuery         = airunmodule.StatsListQuery
	StatsByDateRow         = airunmodule.StatsByDateRow
	StatsByAgentRow        = airunmodule.StatsByAgentRow
	StatsByUserRow         = airunmodule.StatsByUserRow
	Repository             = airunmodule.Repository
	HTTPService            = airunmodule.HTTPService
	Run                    = airunmodule.Run
	RunEvent               = airunmodule.RunEvent
	GormRepository         = airunmodule.GormRepository
	Service                = airunmodule.Service
)
