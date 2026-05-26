package cost_test

import (
	"context"
	"strings"
	"testing"

	pkgcost "github.com/aibao/server/internal/pkg/cost"
	"github.com/aibao/server/internal/service/cost"
	"github.com/spf13/viper"
)

// newTestPB returns a PriceBook with a single doubao/lite entry,
// suitable for recorder happy/sad path tests without testcontainers.
func newTestPB(t *testing.T) pkgcost.PriceBook {
	t.Helper()
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(`
cost:
  price_book_version: v-test
  entries:
    - provider: doubao
      model: lite
      billing_mode: standard
      unit: yuan_per_1m_tokens
      input: 0.30
      output: 0.60
`)); err != nil {
		t.Fatalf("read config: %v", err)
	}
	pb, err := pkgcost.LoadFromViper(v)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return pb
}

func TestRecorder_BadEventID(t *testing.T) {
	r := cost.NewRecorder(newTestPB(t), nil)
	err := r.Record(context.Background(), cost.RecordInput{
		EventID:  "bogus",
		Provider: "doubao", Model: "lite",
		Purpose: "outline", Outcome: "ok",
	})
	if err != cost.ErrBadEventID {
		t.Fatalf("want ErrBadEventID, got %v", err)
	}
}

func TestRecorder_PriceMiss_BusinessContinues(t *testing.T) {
	// price_miss must NEVER break business — Record returns nil and the
	// failure is reflected only in cost_event_record_failed_total metric.
	r := cost.NewRecorder(newTestPB(t), nil)
	err := r.Record(context.Background(), cost.RecordInput{
		EventID:  "abcdef12:outline:llm_call:1",
		Provider: "unknown", Model: "xyz",
		Purpose: "outline", Outcome: "ok",
	})
	if err != nil {
		t.Fatalf("price miss must not break business: %v", err)
	}
}

func TestRecorder_Enqueue(t *testing.T) {
	r := cost.NewRecorder(newTestPB(t), nil)
	_ = r.Record(context.Background(), cost.RecordInput{
		EventID:  "abcdef12:outline:llm_call:1",
		Provider: "doubao", Model: "lite",
		Purpose: "outline", Outcome: "ok",
		Usage:   pkgcost.Usage{TokensIn: 600, TokensOut: 400},
	})
	select {
	case evt := <-r.Drain():
		if evt.EventID != "abcdef12:outline:llm_call:1" {
			t.Errorf("unexpected event_id: %s", evt.EventID)
		}
		want := (600*0.30 + 400*0.60) / 1_000_000.0
		if diff := evt.CostYuan - want; diff < -1e-9 || diff > 1e-9 {
			t.Errorf("cost want %.9f got %.9f", want, evt.CostYuan)
		}
		if evt.PriceVersion != "v-test" {
			t.Errorf("price_version snapshot: want v-test, got %s", evt.PriceVersion)
		}
		if evt.UnitPriceSnapshot["input"] != 0.30 {
			t.Errorf("unit_price_snapshot.input: %v", evt.UnitPriceSnapshot["input"])
		}
	default:
		t.Fatalf("expected event in queue")
	}
}
