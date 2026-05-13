package model

import "time"

type Storyline struct {
	ID              int64      `gorm:"primaryKey;column:id"`
	ChildID         int64      `gorm:"column:child_id;index"`
	Title           string     `gorm:"column:title"`
	Status          string     `gorm:"column:status"`
	NextEpisodeHint string     `gorm:"column:next_episode_hint"`
	EpisodeCount    int        `gorm:"column:episode_count"`
	LastEpisodeAt   *time.Time `gorm:"column:last_episode_at"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (Storyline) TableName() string { return "storylines" }

const (
	StorylineStatusActive    = "active"
	StorylineStatusCompleted = "completed"
	StorylineStatusAbandoned = "abandoned"
)

// PostCheckReasonNotContinuing is the PostCheck reason for sequel continuity failure
// (consumed by service/safety/postcheck.go in Task 8).
const PostCheckReasonNotContinuing = "not_continuing"
