package cost_test

import (
	"testing"

	"github.com/aibao/server/internal/pkg/cost"
)

func TestCalc_LLMTokens(t *testing.T) {
	entry := cost.PriceEntry{
		Unit:        "yuan_per_1m_tokens",
		InputPrice:  0.30,
		OutputPrice: 0.60,
	}
	yuan := cost.Calc(entry, cost.Usage{TokensIn: 600, TokensOut: 400})
	want := (600*0.30 + 400*0.60) / 1_000_000 // 0.000420
	if abs(yuan-want) > 1e-9 {
		t.Errorf("LLM calc: want %.9f, got %.9f", want, yuan)
	}
}

func TestCalc_TTSChars(t *testing.T) {
	entry := cost.PriceEntry{
		Unit:       "yuan_per_1k_chars",
		CharsPrice: 0.85,
	}
	yuan := cost.Calc(entry, cost.Usage{Chars: 1418})
	want := 1418 * 0.85 / 1000
	if abs(yuan-want) > 1e-9 {
		t.Errorf("TTS calc: want %.9f, got %.9f", want, yuan)
	}
}

func TestCalc_ZeroUsage(t *testing.T) {
	yuan := cost.Calc(cost.PriceEntry{Unit: "yuan_per_1m_tokens"}, cost.Usage{})
	if yuan != 0 {
		t.Errorf("zero usage should be 0, got %f", yuan)
	}
}

func TestCalc_UnknownUnit(t *testing.T) {
	yuan := cost.Calc(cost.PriceEntry{Unit: "bogus"}, cost.Usage{TokensIn: 100})
	if yuan != 0 {
		t.Errorf("unknown unit should be 0 (defensive), got %f", yuan)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
