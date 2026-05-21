package payment

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) AlipayCallback(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(callbackResultFail))
		return
	}
	result, _ := h.requireService().HandleAlipayCallback(c.Request.Context(), AlipayCallbackInput{Form: c.Request.PostForm})
	text := callbackResultFail
	if result != nil && result.Text == callbackResultSuccess {
		text = callbackResultSuccess
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(text))
}
