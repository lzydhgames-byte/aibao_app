package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/aibao/server/internal/model"
)

// StoryRepo is the data-access surface for stories.
type StoryRepo interface {
	// CreateWithOutbox inserts story + elements + outbox event in ONE transaction.
	// On success, story.ID, each element.ID, and event.ID are populated.
	CreateWithOutbox(ctx context.Context, story *model.Story, elements []*model.StoryElement, event *model.OutboxEvent) error

	// FindByID returns the story with the given id, or ErrNotFound.
	FindByID(ctx context.Context, id int64) (*model.Story, error)
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
	event *model.OutboxEvent,
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
		// AggregateID points to the story for traceability.
		if event.AggregateID == nil {
			event.AggregateID = &story.ID
		}
		if err := tx.Create(event).Error; err != nil {
			return err
		}
		return nil
	})
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
