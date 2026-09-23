package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireRejectsUnapprovedRoleAndAllowsApprovedRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		role string
		want int
	}{{"auditor", http.StatusForbidden}, {"kiln_engineer", http.StatusNoContent}} {
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set("role", tc.role); c.Next() })
		router.POST("/calculate", Require("admin", "kiln_engineer"), func(c *gin.Context) { c.Status(http.StatusNoContent) })
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/calculate", nil))
		if response.Code != tc.want {
			t.Fatalf("role %q got %d, want %d", tc.role, response.Code, tc.want)
		}
	}
}
