//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/aibao/server/internal/pkg/config"
)

func startPG(t *testing.T) (*postgres.PostgresContainer, config.PostgresConfig) {
	t.Helper()
	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("aibao"),
		postgres.WithUsername("aibao"),
		postgres.WithPassword("aibao"),
		tc.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	host, _ := pg.Host(ctx)
	port, _ := pg.MappedPort(ctx, "5432/tcp")
	return pg, config.PostgresConfig{
		Host:     host,
		Port:     int(port.Num()),
		Database: "aibao",
		User:     "aibao",
		Password: "aibao",
		SSLMode:  "disable",
	}
}

func TestNewDB_Connects(t *testing.T) {
	pg, cfg := startPG(t)
	defer func() { _ = pg.Terminate(context.Background()) }()

	db, err := NewDB(cfg)
	require.NoError(t, err)
	defer Close(db)

	assert.NoError(t, Ping(context.Background(), db))
}

func TestNewDB_BadHost(t *testing.T) {
	cfg := config.PostgresConfig{Host: "127.0.0.1", Port: 1, Database: "x", User: "x", SSLMode: "disable"}
	_, err := NewDB(cfg)
	assert.Error(t, err)
}
