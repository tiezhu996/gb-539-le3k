package handler

import "github.com/gin-gonic/gin"

func actor(c *gin.Context) string     { return c.GetString("user_id") }
func requestID(c *gin.Context) string { return c.GetString("request_id") }
func bind(c *gin.Context, value any) bool {
	if err := c.ShouldBindJSON(value); err != nil {
		c.JSON(422, gin.H{"error": gin.H{"code": "validation_error", "message": err.Error(), "request_id": requestID(c)}})
		return false
	}
	return true
}
func fail(c *gin.Context, err error) { _ = c.Error(err) }
