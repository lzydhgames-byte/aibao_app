package cost_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/aibao/server/internal/pkg/cost"
	"github.com/spf13/viper"
)

func TestPriceBook_LoadAndLookup(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(`
cost:
  price_book_version: v-test-1
  entries:
    - provider: doubao
      model: doubao-1.5-lite-32k
      billing_mode: standard
      unit: yuan_per_1m_tokens
      input: 0.30
      output: 0.60
`)); err != nil {
		t.Fatalf("read config: %v", err)
	}
	pb, err := cost.LoadFromViper(v)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if pb.Version() != "v-test-1" {
		t.Errorf("expected version v-test-1, got %s", pb.Version())
	}
	e, err := pb.Lookup(cost.PriceBookKey{Provider: "doubao", Model: "doubao-1.5-lite-32k", BillingMode: "standard"})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if e.InputPrice != 0.30 || e.OutputPrice != 0.60 {
		t.Errorf("unexpected prices: %+v", e)
	}
}

func TestPriceBook_Miss(t *testing.T) {
	v := viper.New()
	v.Set("cost.price_book_version", "v-test-1")
	pb, _ := cost.LoadFromViper(v)
	_, err := pb.Lookup(cost.PriceBookKey{Provider: "unknown", Model: "x"})
	if !errors.Is(err, cost.ErrPriceMiss) {
		t.Fatalf("expected ErrPriceMiss, got %v", err)
	}
}
