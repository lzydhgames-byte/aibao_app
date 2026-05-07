// Package main is the aibao-server entrypoint. It loads config, initializes
// logger / DB / Redis / metrics, builds the HTTP router, starts the server,
// and blocks until SIGTERM/SIGINT triggers a graceful shutdown.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/aibao/server/internal/api"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/pkg/config"
	"github.com/aibao/server/internal/pkg/logger"
	"github.com/aibao/server/internal/repository"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := os.Getenv("AIBAO_CONFIG")
	if configPath == "" {
		configPath = "config/config.dev.yaml"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	lg, closeLog, err := logger.NewFromConfig(cfg.Server.LogDir, cfg.Server.LogLevel)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() { _ = closeLog() }()
	logger.SetDefault(lg)
	lg.Info("server.starting", "port", cfg.Server.Port, "log_level", cfg.Server.LogLevel)

	db, err := repository.NewDB(cfg.Postgres)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer repository.Close(db)

	rdb, err := repository.NewRedis(cfg.Redis)
	if err != nil {
		return fmt.Errorf("init redis: %w", err)
	}
	defer func() { _ = rdb.Close() }()

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	m := metrics.New(reg)

	router := api.NewRouter(api.RouterDeps{
		Metrics: m,
		Reg:     reg,
		PG:      pgChecker{db: db},
		Redis:   redisChecker{c: rdb},
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		lg.Info("server.listen", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-stop:
		lg.Info("server.shutdown.signal")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		lg.Error("server.shutdown.error", "err", err)
		return err
	}
	lg.Info("server.shutdown.done")
	return nil
}

// pgChecker adapts *gorm.DB to the api.Checker interface for /ready.
type pgChecker struct{ db *gorm.DB }

// Check pings the underlying Postgres connection.
func (p pgChecker) Check(ctx context.Context) error { return repository.Ping(ctx, p.db) }

// redisChecker adapts *redis.Client to the api.Checker interface for /ready.
type redisChecker struct{ c *redis.Client }

// Check pings the underlying Redis connection.
func (r redisChecker) Check(ctx context.Context) error { return repository.PingRedis(ctx, r.c) }
