package model

import "time"

type OutlineEvent struct {
	ID                   int64     `gorm:"primaryKey"`
	OccurredAt           time.Time `gorm:"column:occurred_at"`
	OutlineID            string    `gorm:"column:outline_id"`
	OutlineGroupID       string    `gorm:"column:outline_group_id"`
	UserID               int64     `gorm:"column:user_id"`
	ChildIDHash          string    `gorm:"column:child_id_hash"`
	Outcome              string    `gorm:"column:outcome"`
	OutlinePromptVersion string    `gorm:"column:outline_prompt_version"`
	DurationMin          int       `gorm:"column:duration_min"`
	TraceID              string    `gorm:"column:trace_id"`
}

func (OutlineEvent) TableName() string { return "outline_events" }
