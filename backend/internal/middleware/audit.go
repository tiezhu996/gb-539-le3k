package middleware

import (
	"github.com/gin-gonic/gin"
	"log"
	"time"
)

func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("request_id=%s method=%s path=%s status=%d duration_ms=%d", c.GetString("request_id"), c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start).Milliseconds())
	}
}
