package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/config"
	applog "github.com/iv-tunate/PAS/internal/logger"
	"github.com/iv-tunate/PAS/internal/server"
	"github.com/iv-tunate/PAS/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	log := applog.New(cfg.Env)

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Error("invalid DATABASE_URL", "error", err)
		os.Exit(1)
	}
	poolConfig.MaxConns = 20
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		log.Error("could not connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Error("database ping failed", "error", err)
		os.Exit(1)
	}
	log.Info("connected to database")

	if len(os.Args) >= 2 && os.Args[1] == "create-admin" {
		createAdmin(cfg, log, pool, os.Args[2:])
		return
	}
	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := server.New(appCtx, cfg, log, pool)
	app.Start(appCtx)

	go func() {
		if err := app.Echo.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
			log.Error("server failed to start", "error", err)
			os.Exit(1)
		}
	}()
	log.Info("server started", "port", cfg.Port, "env", cfg.Env)

	<-appCtx.Done()

	log.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := app.Shutdown(ctx, pool); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
	log.Info("shutdown complete")
}

func createAdmin(cfg *config.Config, log *slog.Logger, pool *pgxpool.Pool, args []string) {
	if len(args) < 2 {
		fmt.Println(`usage: go run . create-admin <email> "<password>" ["Full Name"]`)
		os.Exit(1)
	}
	name := ""
	if len(args) >= 3 {
		name = args[2]
	}
	if len(args[1]) < 10 {
		fmt.Println("the password must be at least 10 characters")
		os.Exit(1)
	}

	auth := service.NewAuthService(pool, cfg, log, nil)
	user, err := auth.CreateAdmin(context.Background(), name, args[0], args[1])
	if err != nil {
		fmt.Println("could not create the admin:", err)
		os.Exit(1)
	}
	fmt.Println("admin created:", user.Email)
}
