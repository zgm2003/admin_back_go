package canvasrequest

import (
	"encoding/json"
	"strings"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

var agentOwnedConfigFields = [...]string{"model", "provider", "api_key", "base_url"}

// BindAgentOwnedJSON binds a Canvas AI JSON request and rejects client-owned
// provider/model configuration fields before the module service is called.
func BindAgentOwnedJSON(c *gin.Context, target any, invalidMessageID string, invalidFallback string) bool {
	var fields map[string]json.RawMessage
	if err := c.ShouldBindBodyWith(&fields, binding.JSON); err != nil {
		response.Error(c, apperror.BadRequestKey(invalidMessageID, nil, invalidFallback))
		return false
	}
	for _, field := range agentOwnedConfigFields {
		if _, ok := fields[field]; ok {
			response.Error(c, apperror.BadRequestKey("canvas.ai.request.model_override_forbidden", map[string]any{"field": field}, "客户端不能覆盖Canvas智能体模型"))
			return false
		}
	}
	if err := c.ShouldBindBodyWith(target, binding.JSON); err != nil {
		response.Error(c, apperror.BadRequestKey(invalidMessageID, nil, invalidFallback))
		return false
	}
	return true
}

// BindAgentOwnedJSONOrForm binds a Canvas AI request that is sent either as
// JSON or as browser FormData, while keeping provider/model configuration
// owned by the selected backend agent.
func BindAgentOwnedJSONOrForm(c *gin.Context, target any, invalidMessageID string, invalidFallback string) bool {
	switch c.ContentType() {
	case binding.MIMEJSON:
		return BindAgentOwnedJSON(c, target, invalidMessageID, invalidFallback)
	case binding.MIMEMultipartPOSTForm, binding.MIMEPOSTForm:
		return bindAgentOwnedForm(c, target, invalidMessageID, invalidFallback)
	default:
		response.Error(c, apperror.BadRequestKey(invalidMessageID, nil, invalidFallback))
		return false
	}
}

func bindAgentOwnedForm(c *gin.Context, target any, invalidMessageID string, invalidFallback string) bool {
	if err := parseForm(c); err != nil {
		response.Error(c, apperror.BadRequestKey(invalidMessageID, nil, invalidFallback))
		return false
	}
	for _, field := range agentOwnedConfigFields {
		if _, ok := c.Request.Form[field]; ok {
			response.Error(c, apperror.BadRequestKey("canvas.ai.request.model_override_forbidden", map[string]any{"field": field}, "客户端不能覆盖Canvas智能体模型"))
			return false
		}
	}
	if err := c.ShouldBind(target); err != nil {
		response.Error(c, apperror.BadRequestKey(invalidMessageID, nil, invalidFallback))
		return false
	}
	return true
}

func parseForm(c *gin.Context) error {
	if strings.EqualFold(c.ContentType(), binding.MIMEMultipartPOSTForm) {
		return c.Request.ParseMultipartForm(32 << 20)
	}
	return c.Request.ParseForm()
}
