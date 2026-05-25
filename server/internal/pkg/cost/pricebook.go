package cost

import "errors"

var ErrPriceMiss = errors.New("pricebook: entry not found")

type Usage struct {
	TokensIn     int
	TokensOut    int
	TokensCached int
	Chars        int
	Bytes        int64
	AudioSeconds float64
}

type PriceBookKey struct {
	Provider    string
	Model       string
	BillingMode string // "standard" / "cached" / "batch" / "reasoning"
}

// PriceEntry is what gets snapshotted into cost_events.unit_price_snapshot.
type PriceEntry struct {
	Key                PriceBookKey
	Unit               string  // "yuan_per_1m_tokens" / "yuan_per_1k_chars" / etc.
	InputPrice         float64 // for LLM
	OutputPrice        float64 // for LLM
	CharsPrice         float64 // for TTS by chars
	PutPer10kRequests  float64 // for storage
	BandwidthYuanPerGB float64 // for storage
}

type PriceBook interface {
	Lookup(key PriceBookKey) (PriceEntry, error)
	Version() string
}
