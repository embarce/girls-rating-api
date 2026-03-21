package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIResponse 统一的 API 返回格式：
//
//	{
//	  "msg": "success",
//	  "code": 200,
//	  "data": object
//	}
type APIResponse struct {
	Msg  string `json:"msg"`
	Code int    `json:"code"`
	Data any    `json:"data"`
}

// Success 统一成功响应。
func Success(c *gin.Context, code int, data any) {
	c.JSON(code, APIResponse{
		Msg:  "success",
		Code: code,
		Data: data,
	})
}

// Error 统一错误响应。
func Error(c *gin.Context, code int, msg string) {
	// data 统一返回空对象，避免前端根据类型判断。
	// code 同时用于 HTTP 状态码与响应体字段。
	c.JSON(code, APIResponse{
		Msg:  msg,
		Code: code,
		Data: gin.H{},
	})
}

// StatusOK 便捷方法：success + 200。
func StatusOK(c *gin.Context, data any) {
	Success(c, http.StatusOK, data)
}
