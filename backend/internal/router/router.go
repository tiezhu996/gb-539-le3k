package router

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"timber-kiln-drying-optimizer/backend/internal/handler"
	"timber-kiln-drying-optimizer/backend/internal/middleware"
	"timber-kiln-drying-optimizer/backend/internal/service"
)

type Dependencies struct {
	Auth      service.AuthService
	Kilns     handler.KilnHandler
	Lots      handler.LotHandler
	Readings  handler.ReadingHandler
	Schedules handler.ScheduleHandler
	Audit     handler.AuditHandler
}

func New(deps Dependencies) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.RequestID(), middleware.Recovery(), middleware.AccessLog(), middleware.ErrorHandler())
	engine.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok", "service": "timber-kiln-drying-optimizer"}) })
	engine.GET("/api/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	engine.POST("/api/v1/auth/login", deps.AuthHandler().Login)
	return engine
}
func (d Dependencies) AuthHandler() handler.AuthHandler { return handler.AuthHandler{Service: d.Auth} }
func Mount(engine *gin.Engine, deps Dependencies) {
	auth := middleware.Auth(deps.Auth)
	api := engine.Group("/api/v1")
	api.Use(auth)
	api.GET("/auth/me", deps.AuthHandler().Me)
	api.GET("/kilns", deps.Kilns.List)
	api.POST("/kilns", middleware.Require("admin", "kiln_engineer"), deps.Kilns.Create)
	api.GET("/kilns/:id", deps.Kilns.Get)
	api.PUT("/kilns/:id", middleware.Require("admin", "kiln_engineer"), deps.Kilns.Update)
	api.GET("/lots", deps.Lots.List)
	api.POST("/lots", middleware.Require("admin", "kiln_engineer"), deps.Lots.Create)
	api.GET("/lots/:id", deps.Lots.Get)
	api.POST("/lots/:id/transition", middleware.Require("admin", "kiln_engineer"), deps.Lots.Transition)
	api.GET("/readings", deps.Readings.List)
	api.POST("/readings/import", middleware.Require("admin", "quality_analyst", "kiln_engineer"), deps.Readings.Import)
	api.POST("/readings/:id/void", middleware.Require("admin", "quality_analyst"), deps.Readings.Void)
	api.POST("/readings/:id/correct", middleware.Require("admin", "quality_analyst"), deps.Readings.Correct)
	api.GET("/schedules", deps.Schedules.List)
	api.GET("/schedules/:id", deps.Schedules.Get)
	api.POST("/schedules/calculate", middleware.Require("admin", "quality_analyst", "kiln_engineer"), deps.Schedules.Calculate)
	api.POST("/schedules/:id/review", middleware.Require("admin", "reviewer"), deps.Schedules.Review)
	api.POST("/schedules/:id/freeze", middleware.Require("admin", "reviewer"), deps.Schedules.Freeze)
	api.POST("/schedules/:id/compare", middleware.Require("admin", "reviewer", "quality_analyst"), deps.Schedules.Compare)
	api.GET("/audit", middleware.Require("admin", "auditor"), deps.Audit.List)
}

func NotFound() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "route not found"}})
	}
}
