package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/aibao/server/internal/model"
)

// StorylineRepo is the data-access surface for the storylines table.
type StorylineRepo interface {
	Create(ctx context.Context, sl *model.Storyline) error
	FindByID(ctx context.Context, id int64) (*model.Storyline, error)
	ListActiveByChild(ctx context.Context, childID int64, limit int) ([]*model.Storyline, error)
	IncrementEpisode(ctx context.Context, id int64, hint string) error
}

type storylineRepo struct {
	db *gorm.DB
}

// NewStorylineRepo constructs a GORM-backed StorylineRepo.
func NewStorylineRepo(db *gorm.DB) StorylineRepo { return &storylineRepo{db: db} }

func (r *storylineRepo) Create(ctx context.Context, sl *model.Storyline) error {
	return r.db.WithContext(ctx).Create(sl).Error
}

func (r *storylineRepo) FindByID(ctx context.Context, id int64) (*model.Storyline, error) {
	var sl model.Storyline
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&sl).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sl, nil
}

func (r *storylineRepo) ListActiveByChild(ctx context.Context, childID int64, limit int) ([]*model.Storyline, error) {
	if limit <= 0 {
		limit = 10
	}
	var out []*model.Storyline
	err := r.db.WithContext(ctx).
		Where("child_id = ? AND status = ?", childID, model.StorylineStatusActive).
		Order("last_episode_at DESC NULLS LAST").
		Limit(limit).
		Find(&out).Error
	return out, err
}

func (r *storylineRepo) IncrementEpisode(ctx context.Context, id int64, hint string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&model.Storyline{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"episode_count":     gorm.Expr("episode_count + 1"),
			"next_episode_hint": hint,
			"last_episode_at":   now,
			"updated_at":        now,
		}).Error
}
