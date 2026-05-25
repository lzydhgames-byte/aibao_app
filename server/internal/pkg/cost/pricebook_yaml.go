package cost

import (
	"fmt"

	"github.com/spf13/viper"
)

type yamlEntry struct {
	Provider              string  `mapstructure:"provider"`
	Model                 string  `mapstructure:"model"`
	BillingMode           string  `mapstructure:"billing_mode"`
	Unit                  string  `mapstructure:"unit"`
	Input                 float64 `mapstructure:"input"`
	Output                float64 `mapstructure:"output"`
	Chars                 float64 `mapstructure:"chars"`
	PutYuanPer10kRequests float64 `mapstructure:"put_yuan_per_10k_requests"`
	BandwidthYuanPerGB    float64 `mapstructure:"bandwidth_yuan_per_gb"`
}

type yamlPriceBook struct {
	version string
	entries map[PriceBookKey]PriceEntry
}

func (b *yamlPriceBook) Version() string { return b.version }

func (b *yamlPriceBook) Lookup(key PriceBookKey) (PriceEntry, error) {
	if key.BillingMode == "" {
		key.BillingMode = "standard"
	}
	e, ok := b.entries[key]
	if !ok {
		return PriceEntry{}, fmt.Errorf("%w: %+v", ErrPriceMiss, key)
	}
	return e, nil
}

// LoadFromViper reads cost.price_book_version + cost.entries from the given viper instance.
// Hot-reload is NOT supported (spec §5.2) — caller restarts the process to apply changes.
func LoadFromViper(v *viper.Viper) (PriceBook, error) {
	version := v.GetString("cost.price_book_version")
	if version == "" {
		return nil, fmt.Errorf("cost.price_book_version is required")
	}
	var raw []yamlEntry
	if err := v.UnmarshalKey("cost.entries", &raw); err != nil {
		return nil, fmt.Errorf("decode cost.entries: %w", err)
	}
	pb := &yamlPriceBook{version: version, entries: map[PriceBookKey]PriceEntry{}}
	for _, r := range raw {
		key := PriceBookKey{Provider: r.Provider, Model: r.Model, BillingMode: r.BillingMode}
		if key.BillingMode == "" {
			key.BillingMode = "standard"
		}
		pb.entries[key] = PriceEntry{
			Key: key, Unit: r.Unit,
			InputPrice: r.Input, OutputPrice: r.Output,
			CharsPrice:         r.Chars,
			PutPer10kRequests:  r.PutYuanPer10kRequests,
			BandwidthYuanPerGB: r.BandwidthYuanPerGB,
		}
	}
	return pb, nil
}
