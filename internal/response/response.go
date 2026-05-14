package response

import (
	"admin_back_go/internal/apperror"
	projecti18n "admin_back_go/internal/i18n"

	"github.com/gin-gonic/gin"
)

type Body struct {
	Code int    `json:"code"`
	Data any    `json:"data"`
	Msg  string `json:"msg"`
}

func OK(c *gin.Context, data any) {
	OKWithMessage(c, data, "ok")
}

func OKWithMessage(c *gin.Context, data any, message string) {
	c.JSON(200, Body{
		Code: apperror.CodeOK,
		Data: data,
		Msg:  message,
	})
}

func Error(c *gin.Context, err *apperror.Error) {
	ErrorWithData(c, err, gin.H{})
}

func ErrorWithData(c *gin.Context, err *apperror.Error, data any) {
	if err == nil {
		err = apperror.InternalKey("common.internal_error", nil, "系统错误")
	}
	if data == nil {
		data = gin.H{}
	}

	message := err.Message
	if localized, localizeErr := projecti18n.Message(c, err.MessageID, err.TemplateData, err.Message); localizeErr == nil && localized != "" {
		message = localized
	}

	c.JSON(err.HTTPStatus, Body{
		Code: err.Code,
		Data: data,
		Msg:  message,
	})
}

func Abort(c *gin.Context, err *apperror.Error) {
	Error(c, err)
	c.Abort()
}
