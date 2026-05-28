package callback

import (
	"context"
	"net/http"

	"admin_back_go/internal/apperror"
	paymentmodule "admin_back_go/internal/module/payment"

	"github.com/gin-gonic/gin"
)

const (
	callbackResultSuccess = "success"
	callbackResultFail    = "fail"
)

type HTTPService interface {
	HandleAlipayCallback(ctx context.Context, input paymentmodule.AlipayCallbackInput) (*paymentmodule.AlipayCallbackResult, *apperror.Error)
}

type Handler struct{ service HTTPService }

func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

func (h *Handler) requireService() HTTPService {
	if h == nil || h.service == nil {
		return nilHTTPService{}
	}
	return h.service
}

type nilHTTPService struct{}

func (nilHTTPService) HandleAlipayCallback(ctx context.Context, input paymentmodule.AlipayCallbackInput) (*paymentmodule.AlipayCallbackResult, *apperror.Error) {
	return &paymentmodule.AlipayCallbackResult{Text: callbackResultFail}, nil
}

func (h *Handler) AlipayCallback(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(callbackResultFail))
		return
	}
	result, _ := h.requireService().HandleAlipayCallback(c.Request.Context(), paymentmodule.AlipayCallbackInput{Form: c.Request.PostForm})
	text := callbackResultFail
	if result != nil && result.Text == callbackResultSuccess {
		text = callbackResultSuccess
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(text))
}
