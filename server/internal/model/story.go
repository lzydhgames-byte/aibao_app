package model

import "time"

// Story maps to the `stories` table.
type Story struct {
	ID                   int64     `gorm:"primaryKey;column:id" json:"id"`
	ChildID              int64     `gorm:"column:child_id;index" json:"child_id"`
	Title                string    `gorm:"column:title" json:"title"`
	TextContent          string    `gorm:"column:text_content" json:"text"`
	AudioObjectKey       string    `gorm:"column:audio_object_key" json:"audio_object_key"`
	AudioFormat          string    `gorm:"column:audio_format" json:"-"`
	AudioSizeBytes       int64     `gorm:"column:audio_size_bytes" json:"-"`
	AudioDurationSeconds int       `gorm:"column:audio_duration_seconds" json:"-"`
	AudioStatus          string     `gorm:"column:audio_status" json:"audio_status"`
	AudioFailedAt        *time.Time `gorm:"column:audio_failed_at" json:"-"`
	DurationMinutes      int       `gorm:"column:duration_minutes" json:"duration_minutes"`
	Style                string    `gorm:"column:style" json:"style"`
	Topic                string    `gorm:"column:topic" json:"topic"`
	StorylineID          *int64    `gorm:"column:storyline_id" json:"-"`
	EpisodeNo            *int      `gorm:"column:episode_no" json:"-"`
	HasBGM               bool      `gorm:"column:has_bgm" json:"has_bgm"`
	PromptVersion        string    `gorm:"column:prompt_version" json:"-"`
	LLMModel             string    `gorm:"column:llm_model" json:"-"`
	LLMInputTokens       int       `gorm:"column:llm_input_tokens" json:"-"`
	LLMOutputTokens      int       `gorm:"column:llm_output_tokens" json:"-"`
	CreatedAt            time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName returns the SQL table name.
func (Story) TableName() string { return "stories" }

// StoryElement maps to story_elements.
type StoryElement struct {
	ID           int64   `gorm:"primaryKey;column:id"`
	StoryID      int64   `gorm:"column:story_id;index"`
	ElementType  string  `gorm:"column:element_type"`
	Name         string  `gorm:"column:name"`
	Description  string  `gorm:"column:description"`
	RecallWeight float64 `gorm:"column:recall_weight"`
}

// TableName returns the SQL table name.
func (StoryElement) TableName() string { return "story_elements" }

// Memory maps to memories.
type Memory struct {
	ID         int64     `gorm:"primaryKey;column:id"`
	ChildID    int64     `gorm:"column:child_id;index"`
	MemoryType string    `gorm:"column:memory_type"`
	Payload    []byte    `gorm:"column:payload;type:jsonb"`
	Weight     float64   `gorm:"column:weight"`
	StoryID    *int64    `gorm:"column:story_id" json:"story_id,omitempty"` // Plan 6: nullable FK
	CreatedAt  time.Time `gorm:"column:created_at"`
}

// TableName returns the SQL table name.
func (Memory) TableName() string { return "memories" }

// Memory type constants.
const (
	MemoryTypeStorySummary = "story_summary"
	MemoryTypeInterest     = "interest"
	MemoryTypePreference   = "preference"
)

// OutboxEvent maps to outbox_events.
type OutboxEvent struct {
	ID            int64     `gorm:"primaryKey;column:id"`
	EventType     string    `gorm:"column:event_type"`
	AggregateID   *int64    `gorm:"column:aggregate_id"`
	Payload       []byte    `gorm:"column:payload;type:jsonb"`
	Status        string    `gorm:"column:status"`
	Attempts      int       `gorm:"column:attempts"`
	LastError     string    `gorm:"column:last_error"`
	NextAttemptAt time.Time `gorm:"column:next_attempt_at"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

// TableName returns the SQL table name.
func (OutboxEvent) TableName() string { return "outbox_events" }

// Outbox status constants.
const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusDone       = "done"
	OutboxStatusDead       = "dead"
)

// Outbox event types.
const (
	EventTypeMemoryUpdate = "memory_update"
	EventTypeTTSSynthesis = "tts_synthesis"
)

// Audio status constants for Story.AudioStatus.
const (
	AudioStatusPending = "pending"
	AudioStatusReady   = "ready"
	AudioStatusFailed  = "failed"
)
