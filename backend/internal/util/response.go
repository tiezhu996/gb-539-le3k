package util

import "github.com/gin-gonic/gin"

func OK(c *gin.Context, data any)      { c.JSON(200, gin.H{"data": data}) }
func Created(c *gin.Context, data any) { c.JSON(201, gin.H{"data": data}) }
func Fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message, "request_id": c.GetString("request_id")}})
}
