package middleware

import (
	"github.com/gin-gonic/gin"
	"strings"
	"timber-kiln-drying-optimizer/backend/internal/service"
)

func Auth(auth service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(401, gin.H{"error": gin.H{"code": "unauthorized", "message": "Bearer token required"}})
			return
		}
		claims, err := auth.Parse(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": gin.H{"code": "unauthorized", "message": "invalid token"}})
			return
		}
		c.Set("user_id", claims["sub"])
		c.Set("role", claims["role"])
		c.Set("user_email", claims["email"])
		c.Next()
	}
}
