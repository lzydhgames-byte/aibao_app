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
	"github.com/aibao/server/internal/gateway/sms"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/pkg/config"
	"github.com/aibao/server/internal/pkg/jwtauth"
	"github.com/aibao/server/internal/pkg/logger"
	"github.com/aibao/server/internal/pkg/phonecrypt"
	"github.com/aibao/server/internal/pkg/safehash"
	"github.com/aibao/server/internal/repository"
	authsvc "github.com/aibao/server/internal/service/auth"
	childsvc "github.com/aibao/server/internal/service/child"
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

	if err := repository.RunMigrations(db, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	rdb, err := repository.NewRedis(cfg.Redis)
	if err != nil {
		return fmt.Errorf("init redis: %w", err)
	}
	defer func() { _ = rdb.Close() }()

	// Build domain dependencies (Plan 2).
	hasher := safehash.New(cfg.Crypto.SafehashSalt)
	pcipher, err := phonecrypt.New(cfg.Crypto.PhoneAESKey)
	if err != nil {
		return fmt.Errorf("init phone cipher: %w", err)
	}
	jwtMgr := jwtauth.New(
		cfg.Auth.JWTSecret,
		time.Duration(cfg.Auth.AccessTTLMinutes)*time.Minute,
		time.Duration(cfg.Auth.RefreshTTLMinutes)*time.Minute,
	)

	userRepo := repository.NewUserRepo(db)
	childRepo := repository.NewChildRepo(db)
	codeStore := authsvc.NewRedisCodeStore(rdb)

	mockSMS := sms.NewMock()
	var smsSender authsvc.SMS
	switch cfg.SMS.Provider {
	case "mock", "":
		smsSender = mockSMS
	default:
		return fmt.Errorf("unknown sms provider: %s", cfg.SMS.Provider)
	}

	authService := authsvc.New(authsvc.Deps{
		Users:        userRepo,
		CodeStore:    codeStore,
		SMS:          smsSender,
		JWT:          jwtMgr,
		PhoneCipher:  pcipher,
		Hasher:       hasher,
		FixedDevCode: mockSMS.FixedCode(),
		CodeTTL:      time.Duration(cfg.SMS.CodeTTLSeconds) * time.Second,
		Cooldown:     time.Duration(cfg.SMS.ResendCooldownSec) * time.Second,
	})
	childService := childsvc.New(childRepo)

	authHandler := api.NewAuthHandler(authService)
	meHandler := api.NewMeHandler(userRepo)
	childHandler := api.NewChildHandler(childService)

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
		JWT:     jwtMgr,
		Auth:    authHandler,
		Me:      meHandler,
		Child:   childHandler,
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
