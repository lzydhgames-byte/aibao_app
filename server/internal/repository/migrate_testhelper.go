//go:build integration

package repository

import (
	"gorm.io/gorm"

	"github.com/aibao/server/internal/model"
)

// autoMigrateForTest creates schema for User/Child by GORM's AutoMigrate.
// Used by integration tests so they don't depend on the migrate CLI.
// Production uses RunMigrations() with the SQL files instead.
func autoMigrateForTest(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{},
		&model.Child{},
		&model.Story{},
		&model.StoryElement{},
		&model.Memory{},
		&model.OutboxEvent{},
	)
}
