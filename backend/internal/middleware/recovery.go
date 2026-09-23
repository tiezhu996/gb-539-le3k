package middleware

import "github.com/gin-gonic/gin"

func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		c.AbortWithStatusJSON(500, gin.H{"error": gin.H{"code": "internal_error", "message": "internal server error", "request_id": c.GetString("request_id")}})
	})
}
