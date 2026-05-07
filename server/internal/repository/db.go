// Package repository contains data-access primitives: PG and Redis client
// initialization plus per-table repository implementations (added in later
// plans).
package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/aibao/server/internal/pkg/config"
)

// NewDB opens a GORM connection to PostgreSQL using the provided config.
// It tunes the underlying connection pool (max 20 open / 5 idle / 1h lifetime)
// and verifies connectivity with a ping before returning. The lifetime cap
// guards against NAT/firewall idle timeouts dropping long-lived connections.
func NewDB(cfg config.PostgresConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, sslMode(cfg.SSLMode),
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("pg ping: %w", err)
	}
	return db, nil
}

// Ping verifies the database is reachable. Used by the /ready endpoint.
func Ping(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close closes the underlying database/sql pool. Safe to call with nil.
func Close(db *gorm.DB) {
	if db == nil {
		return
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

func sslMode(s string) string {
	if s == "" {
		return "disable"
	}
	return s
}
