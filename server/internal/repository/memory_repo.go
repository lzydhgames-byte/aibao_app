package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/aibao/server/internal/model"
)

// MemoryRepo is the data-access surface for the memories table.
type MemoryRepo interface {
	Create(ctx context.Context, m *model.Memory) error
	// RecentByChild returns up to limit recent memories of the given type.
	RecentByChild(ctx context.Context, childID int64, memoryType string, limit int) ([]*model.Memory, error)
}

type memoryRepo struct {
	db *gorm.DB
}

// NewMemoryRepo constructs a GORM-backed MemoryRepo.
func NewMemoryRepo(db *gorm.DB) MemoryRepo { return &memoryRepo{db: db} }

func (r *memoryRepo) Create(ctx context.Context, m *model.Memory) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *memoryRepo) RecentByChild(ctx context.Context, childID int64, memoryType string, limit int) ([]*model.Memory, error) {
	if limit <= 0 {
		limit = 10
	}
	var out []*model.Memory
	err := r.db.WithContext(ctx).
		Where("child_id = ? AND memory_type = ?", childID, memoryType).
		Order("created_at DESC").
		Limit(limit).
		Find(&out).Error
	return out, err
}
