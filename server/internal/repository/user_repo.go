package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/aibao/server/internal/model"
)

// ErrNotFound is returned when a row lookup yields no result.
var ErrNotFound = errors.New("not found")

// UserRepo is the data-access surface the auth service depends on.
type UserRepo interface {
	// CreateOrGet inserts u when no row with the same PhoneHash exists,
	// otherwise loads the existing row. The returned bool is true on creation.
	CreateOrGet(ctx context.Context, u *model.User) (*model.User, bool, error)

	// FindByID returns the user with the given id, or ErrNotFound.
	FindByID(ctx context.Context, id int64) (*model.User, error)
}

type userRepo struct {
	db *gorm.DB
}

// NewUserRepo returns a GORM-backed UserRepo.
func NewUserRepo(db *gorm.DB) UserRepo { return &userRepo{db: db} }

func (r *userRepo) CreateOrGet(ctx context.Context, u *model.User) (*model.User, bool, error) {
	tx := r.db.WithContext(ctx)

	var existing model.User
	err := tx.Where("phone_hash = ?", u.PhoneHash).First(&existing).Error
	if err == nil {
		return &existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	// not found, create
	if err := tx.Create(u).Error; err != nil {
		// Could be a race — another concurrent insert won. Re-fetch.
		var existing2 model.User
		if e2 := tx.Where("phone_hash = ?", u.PhoneHash).First(&existing2).Error; e2 == nil {
			return &existing2, false, nil
		}
		return nil, false, err
	}
	return u, true, nil
}

func (r *userRepo) FindByID(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
