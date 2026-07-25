package admin

import (
	"net/http"

	redeemcode "admin_back_go/internal/module/payment/redeemcode"
	"admin_back_go/internal/server/adminroute"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService, registries ...*adminroute.Registry) {
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, registries...)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet, Path: "/api/admin/v1/payment/redeem-codes/page-init",
		Access: adminroute.Permission("payment_redeem_code_list"), Audit: adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{Response: redeemcode.PageInitResponse{}},
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet, Path: "/api/admin/v1/payment/redeem-codes",
		Access: adminroute.Permission("payment_redeem_code_list"), Audit: adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{Query: listRequest{}, Response: redeemcode.ListResponse{}},
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost, Path: "/api/admin/v1/payment/redeem-code-lookups",
		Access: adminroute.Permission("payment_redeem_code_list"), Audit: adminroute.NoAudit("read-only exact lookup"),
		Contract: &adminroute.HTTPContract{Request: lookupRequest{}, Response: redeemcode.LookupResponse{}},
	}, handler.Lookup)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost, Path: "/api/admin/v1/payment/redeem-code-exports",
		Access: adminroute.Permission("payment_redeem_code_list"), Audit: audit("payment_redeem_code", "export", "导出兑换码", false, true, true),
		Contract: &adminroute.HTTPContract{Request: exportRequest{}, Response: redeemcode.ExportResponse{}},
	}, handler.Export)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost, Path: "/api/admin/v1/payment/redeem-code-batches",
		Access: adminroute.Permission("payment_redeem_code_generate"), Audit: audit("payment_redeem_code", "generate", "生成兑换码", true, true, true),
		Contract: &adminroute.HTTPContract{Request: generateBatchRequest{}, Response: redeemcode.GenerateBatchResponse{}},
	}, handler.GenerateBatch)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch, Path: "/api/admin/v1/payment/redeem-codes",
		Access: adminroute.Permission("payment_redeem_code_void"), Audit: audit("payment_redeem_code", "void", "作废兑换码", true, true, false),
		Contract: &adminroute.HTTPContract{Request: voidRequest{}, Response: redeemcode.VoidResponse{}},
	}, handler.Void)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost, Path: "/api/admin/v1/wallet/redemptions",
		Access: adminroute.Authenticated(), Audit: audit("wallet", "redeem", "兑换码充值", true, true, true),
		Contract: &adminroute.HTTPContract{Request: redemptionRequest{}, Response: redemptionResponse{}},
	}, handler.Redeem)
}

func audit(module, action, title string, required, skipRequest, skipResponse bool) adminroute.AuditDecision {
	d := adminroute.Audit(module, action, title)
	d.Required = required
	d.SkipRequestPayload = skipRequest
	d.SkipResponsePayload = skipResponse
	return d
}
