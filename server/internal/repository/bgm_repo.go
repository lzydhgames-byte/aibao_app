package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/aibao/server/internal/model"
)

// BGMRepo is the read/write surface over bgm_assets.
type BGMRepo interface {
	// PickByMood randomly returns one active asset of the given mood.
	// Returns ErrNotFound when no active row matches.
	PickByMood(ctx context.Context, mood string) (*model.BGMAsset, error)
	// List returns all active assets (ordered by mood, filename).
	List(ctx context.Context) ([]*model.BGMAsset, error)
	// Upsert inserts or updates by filename (used by seed CLI).
	Upsert(ctx context.Context, a *model.BGMAsset) error
}

type bgmRepo struct{ db *gorm.DB }

// NewBGMRepo constructs a GORM-backed BGMRepo.
func NewBGMRepo(db *gorm.DB) BGMRepo { return &bgmRepo{db: db} }

func (r *bgmRepo) PickByMood(ctx context.Context, mood string) (*model.BGMAsset, error) {
	var a model.BGMAsset
	err := r.db.WithContext(ctx).
		Where("mood = ? AND active = TRUE", mood).
		Order("RANDOM()").
		Limit(1).
		Take(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *bgmRepo) List(ctx context.Context) ([]*model.BGMAsset, error) {
	var out []*model.BGMAsset
	err := r.db.WithContext(ctx).
		Where("active = TRUE").
		Order("mood, filename").
		Find(&out).Error
	return out, err
}

func (r *bgmRepo) Upsert(ctx context.Context, a *model.BGMAsset) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "filename"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"mood", "object_key", "duration_sec", "license", "active",
		}),
	}).Create(a).Error
}
