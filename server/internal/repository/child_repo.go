package repository

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/aibao/server/internal/model"
)

// ErrAlreadyExists is returned when an INSERT violates a UNIQUE constraint.
var ErrAlreadyExists = errors.New("already exists")

// ChildRepo is the data-access surface the child service depends on.
type ChildRepo interface {
	Create(ctx context.Context, c *model.Child) error
	FindByUserID(ctx context.Context, userID int64) (*model.Child, error)
	FindByID(ctx context.Context, id int64) (*model.Child, error)
	Update(ctx context.Context, c *model.Child) error
}

type childRepo struct {
	db *gorm.DB
}

// NewChildRepo returns a GORM-backed ChildRepo.
func NewChildRepo(db *gorm.DB) ChildRepo { return &childRepo{db: db} }

func (r *childRepo) Create(ctx context.Context, c *model.Child) error {
	err := r.db.WithContext(ctx).Create(c).Error
	if err == nil {
		return nil
	}
	// PG unique-violation error contains "duplicate key" / "unique constraint"
	if isUniqueViolation(err) {
		return ErrAlreadyExists
	}
	return err
}

func (r *childRepo) FindByUserID(ctx context.Context, userID int64) (*model.Child, error) {
	var c model.Child
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *childRepo) FindByID(ctx context.Context, id int64) (*model.Child, error) {
	var c model.Child
	err := r.db.WithContext(ctx).First(&c, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *childRepo) Update(ctx context.Context, c *model.Child) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func isUniqueViolation(err error) bool {
	// We don't depend on lib/pq error codes — match by message substring,
	// which is robust across pgx and lib/pq drivers.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}
