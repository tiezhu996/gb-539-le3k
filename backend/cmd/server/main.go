package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"timber-kiln-drying-optimizer/backend/internal/config"
	"timber-kiln-drying-optimizer/backend/internal/handler"
	"timber-kiln-drying-optimizer/backend/internal/model"
	"timber-kiln-drying-optimizer/backend/internal/repository"
	"timber-kiln-drying-optimizer/backend/internal/router"
	"timber-kiln-drying-optimizer/backend/internal/service"
	"time"
)

func main() {
	cfg := config.Load()
	db, err := model.Open(cfg.DBDriver, cfg.DBDSN)
	if err != nil {
		log.Fatal(err)
	}
	if err = model.Seed(db); err != nil {
		log.Fatal(err)
	}
	audit := service.AuditService{Repo: repository.AuditRepository{DB: db}}
	auth := service.AuthService{Users: repository.UserRepository{DB: db}, Secret: cfg.JWTSecret}
	kilns := service.KilnService{Repo: repository.KilnRepository{DB: db}, Audit: audit}
	scheduleRepo := repository.ScheduleRepository{DB: db}
	lots := service.LotService{Repo: repository.LotRepository{DB: db}, Kilns: repository.KilnRepository{DB: db}, Schedules: scheduleRepo, Audit: audit}
	readings := service.ReadingService{Repo: repository.ReadingRepository{DB: db}, Lots: repository.LotRepository{DB: db}, Audit: audit}
	schedules := service.ScheduleService{Repo: scheduleRepo, Lots: repository.LotRepository{DB: db}, Kilns: repository.KilnRepository{DB: db}, Readings: repository.ReadingRepository{DB: db}, Audit: audit}
	readings.Schedules = schedules
	deps := router.Dependencies{Auth: auth, Kilns: handler.KilnHandler{Service: kilns}, Lots: handler.LotHandler{Service: lots}, Readings: handler.ReadingHandler{Service: readings}, Schedules: handler.ScheduleHandler{Service: schedules}, Audit: handler.AuditHandler{Service: audit}}
	engine := router.New(deps)
	router.Mount(engine, deps)
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: engine}
	go func() {
		log.Printf("KilnCurve listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
