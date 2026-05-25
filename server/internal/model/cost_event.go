package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type PriceSnapshot map[string]any

func (p PriceSnapshot) Value() (driver.Value, error) { return json.Marshal(p) }
func (p *PriceSnapshot) Scan(v any) error {
	if v == nil {
		*p = nil
		return nil
	}
	b, ok := v.([]byte)
	if !ok {
		return errors.New("PriceSnapshot.Scan: expected []byte")
	}
	return json.Unmarshal(b, p)
}

type CostEvent struct {
	ID                   int64         `gorm:"primaryKey"`
	EventID              string        `gorm:"column:event_id;uniqueIndex"`
	OccurredAt           time.Time     `gorm:"column:occurred_at"`
	UserID               *int64        `gorm:"column:user_id"`
	ChildIDHash          string        `gorm:"column:child_id_hash"`
	Purpose              string        `gorm:"column:purpose"`
	Provider             string        `gorm:"column:provider"`
	Model                string        `gorm:"column:model"`
	BillingMode          string        `gorm:"column:billing_mode"`
	TokensIn             int           `gorm:"column:tokens_in"`
	TokensOut            int           `gorm:"column:tokens_out"`
	TokensCached         int           `gorm:"column:tokens_cached"`
	Chars                int           `gorm:"column:chars"`
	Bytes                int64         `gorm:"column:bytes"`
	AudioSeconds         float64       `gorm:"column:audio_seconds"`
	CostYuan             float64       `gorm:"column:cost_yuan"`
	Currency             string        `gorm:"column:currency"`
	PriceVersion         string        `gorm:"column:price_version"`
	UnitPriceSnapshot    PriceSnapshot `gorm:"column:unit_price_snapshot;type:jsonb"`
	Outcome              string        `gorm:"column:outcome"`
	DurationMs           int           `gorm:"column:duration_ms"`
	StoryID              *int64        `gorm:"column:story_id"`
	OutlineID            string        `gorm:"column:outline_id"`
	OutlineGroupID       string        `gorm:"column:outline_group_id"`
	OutlinePromptVersion string        `gorm:"column:outline_prompt_version"`
	TraceID              string        `gorm:"column:trace_id"`
}

func (CostEvent) TableName() string { return "cost_events" }
