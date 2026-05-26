//go:build integration

package cost_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aibao/server/internal/model"
	pkgcost "github.com/aibao/server/internal/pkg/cost"
	"github.com/aibao/server/internal/service/cost"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// integrationDB connects to the local aibao-postgres-dev container.
// DSN can be overridden via AIBAO_TEST_DB_DSN env var.
func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AIBAO_TEST_DB_DSN")
	if dsn == "" {
		dsn = "postgres://aibao:aibao@127.0.0.1:5432/aibao?sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func loadPB(t *testing.T) pkgcost.PriceBook {
	t.Helper()
	v := viper.New()
	v.SetConfigType("yaml")
	_ = v.ReadConfig(strings.NewReader(`
cost:
  price_book_version: v-flushtest
  entries:
    - provider: doubao
      model: lite
      billing_mode: standard
      unit: yuan_per_1m_tokens
      input: 0.30
      output: 0.60
`))
	pb, err := pkgcost.LoadFromViper(v)
	require.NoError(t, err)
	return pb
}

// TestFlusher_Idempotent — record the same event_id twice and shut down;
// final flush must INSERT exactly one row (ON CONFLICT DO NOTHING).
func TestFlusher_Idempotent(t *testing.T) {
	db := integrationDB(t)
	// Use unique event_id per-run to avoid colliding across test runs.
	eventID := "fa11ce8a:outline:llm_call:1001"

	// Clean previous test rows for this event_id.
	db.Exec("DELETE FROM cost_events WHERE event_id = ?", eventID)
	defer db.Exec("DELETE FROM cost_events WHERE event_id = ?", eventID)

	r := cost.NewRecorder(loadPB(t), nil)
	f := cost.NewFlusher(r, db, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go f.Run(ctx)
	time.Sleep(50 * time.Millisecond) // let Run goroutine reach select loop

	in := cost.RecordInput{
		EventID:  eventID,
		Provider: "doubao", Model: "lite",
		Purpose: "outline", Outcome: "ok",
		Usage: pkgcost.Usage{TokensIn: 100, TokensOut: 50},
	}
	_ = r.Record(context.Background(), in)
	_ = r.Record(context.Background(), in) // duplicate — must be deduped by ON CONFLICT

	// Cancel → triggers final flush within shutdownGrace.
	cancel()
	time.Sleep(1 * time.Second) // give the goroutine time to finish flushing

	var cnt int64
	db.Model(&model.CostEvent{}).Where("event_id = ?", eventID).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("expected 1 row after duplicate Record (idempotent), got %d", cnt)
	}
}

// TestFlusher_HappyPath_PersistsCostYuan — one record → one row with the
// computed cost_yuan + snapshot.
func TestFlusher_HappyPath_PersistsCostYuan(t *testing.T) {
	db := integrationDB(t)
	eventID := "fa11ce8a:tts:synthesize:1002"
	db.Exec("DELETE FROM cost_events WHERE event_id = ?", eventID)
	defer db.Exec("DELETE FROM cost_events WHERE event_id = ?", eventID)

	r := cost.NewRecorder(loadPB(t), nil)
	f := cost.NewFlusher(r, db, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go f.Run(ctx)
	time.Sleep(50 * time.Millisecond) // let Run goroutine reach select loop

	_ = r.Record(context.Background(), cost.RecordInput{
		EventID:  eventID,
		Provider: "doubao", Model: "lite",
		Purpose: "story", Outcome: "ok",
		Usage:   pkgcost.Usage{TokensIn: 1000, TokensOut: 500},
	})
	cancel()
	time.Sleep(1 * time.Second)

	var evt model.CostEvent
	err := db.Where("event_id = ?", eventID).First(&evt).Error
	require.NoError(t, err)

	want := (1000*0.30 + 500*0.60) / 1_000_000.0
	if diff := evt.CostYuan - want; diff < -1e-6 || diff > 1e-6 {
		t.Errorf("cost_yuan: want %.9f got %.9f", want, evt.CostYuan)
	}
	if evt.PriceVersion != "v-flushtest" {
		t.Errorf("price_version: want v-flushtest, got %s", evt.PriceVersion)
	}
	if evt.UnitPriceSnapshot["input"] != 0.30 {
		t.Errorf("snapshot input: %v", evt.UnitPriceSnapshot["input"])
	}
}
