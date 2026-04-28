package response

import "github.com/gin-gonic/gin"

type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(200, Response{Success: true, Data: data})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(201, Response{Success: true, Data: data})
}

func Error(c *gin.Context, code int, msg string) {
	c.JSON(code, Response{Success: false, Message: msg})
}

func Page(c *gin.Context, content interface{}, total int64, page, size int) {
	c.JSON(200, Response{Success: true, Data: gin.H{
		"content": content, "totalElements": total,
		"page": page, "size": size,
		"totalPages": (total + int64(size) - 1) / int64(size),
	}})
}
