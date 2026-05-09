package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/aibao/server/internal/model"
)

// StoryRepo is the data-access surface for stories.
type StoryRepo interface {
	// CreateWithOutbox inserts story + elements + N outbox events in ONE transaction.
	// On success, story.ID, each element.ID, and each event.ID are populated;
	// every event.AggregateID is auto-set to story.ID if nil.
	CreateWithOutbox(ctx context.Context, story *model.Story, elements []*model.StoryElement, events []*model.OutboxEvent) error

	// FindByID returns the story with the given id, or ErrNotFound.
	FindByID(ctx context.Context, id int64) (*model.Story, error)

	// MarkAudioReady atomically updates a story to audio_status='ready' and
	// fills audio_object_key/format/size/duration.
	MarkAudioReady(ctx context.Context, storyID int64, objectKey, format string, sizeBytes int64, durationSec int) error

	// MarkAudioFailed sets audio_status='failed' and stamps audio_failed_at.
	MarkAudioFailed(ctx context.Context, storyID int64, errMsg string) error
}

type storyRepo struct {
	db *gorm.DB
}

// NewStoryRepo constructs a GORM-backed StoryRepo.
func NewStoryRepo(db *gorm.DB) StoryRepo { return &storyRepo{db: db} }

func (r *storyRepo) CreateWithOutbox(
	ctx context.Context,
	story *model.Story,
	elements []*model.StoryElement,
	events []*model.OutboxEvent,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(story).Error; err != nil {
			return err
		}
		for _, e := range elements {
			e.StoryID = story.ID
			if err := tx.Create(e).Error; err != nil {
				return err
			}
		}
		for _, ev := range events {
			if ev.AggregateID == nil {
				ev.AggregateID = &story.ID
			}
			if err := tx.Create(ev).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *storyRepo) MarkAudioReady(
	ctx context.Context, storyID int64, objectKey, format string, sizeBytes int64, durationSec int,
) error {
	return r.db.WithContext(ctx).
		Model(&model.Story{}).
		Where("id = ?", storyID).
		Updates(map[string]any{
			"audio_status":           model.AudioStatusReady,
			"audio_object_key":       objectKey,
			"audio_format":           format,
			"audio_size_bytes":       sizeBytes,
			"audio_duration_seconds": durationSec,
			"audio_failed_at":        nil,
		}).Error
}

func (r *storyRepo) MarkAudioFailed(ctx context.Context, storyID int64, errMsg string) error {
	now := time.Now()
	_ = errMsg // not persisted on stories table to keep schema slim; logged + emitted as metric label upstream.
	return r.db.WithContext(ctx).
		Model(&model.Story{}).
		Where("id = ?", storyID).
		Updates(map[string]any{
			"audio_status":    model.AudioStatusFailed,
			"audio_failed_at": now,
		}).Error
}

func (r *storyRepo) FindByID(ctx context.Context, id int64) (*model.Story, error) {
	var s model.Story
	err := r.db.WithContext(ctx).First(&s, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}
