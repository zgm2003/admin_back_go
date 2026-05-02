package response

import "github.com/gin-gonic/gin"

type Body struct {
	Code int    `json:"code"`
	Data any    `json:"data"`
	Msg  string `json:"msg"`
}

func OK(c *gin.Context, data any) {
	c.JSON(200, Body{
		Code: 0,
		Data: data,
		Msg:  "ok",
	})
}
