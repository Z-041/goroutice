package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构，code 为 0 表示成功，非 0 表示对应错误（沿用 HTTP 状态码）。
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// PageData 分页数据包装。
type PageData struct {
	List  interface{} `json:"list"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}

// Success 返回 200 成功响应。
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "success", Data: data})
}

// Created 返回 201 创建成功响应。
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{Code: 0, Message: "success", Data: data})
}

// Page 返回分页响应。
func Page(c *gin.Context, list interface{}, total int64, page, size int) {
	Success(c, PageData{List: list, Total: total, Page: page, Size: size})
}

// Error 返回错误响应。
func Error(c *gin.Context, httpStatus int, message string) {
	c.JSON(httpStatus, Response{Code: httpStatus, Message: message})
}
