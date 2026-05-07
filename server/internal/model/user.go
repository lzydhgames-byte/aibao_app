// Package model holds GORM-tagged data structs that mirror the database tables.
// Table-name conventions and JSON keys match the API contract documented in
// the Plan 2 spec.
package model

import "time"

// User maps to the `users` table.
type User struct {
	ID               int64     `gorm:"primaryKey;column:id" json:"id"`
	PhoneHash        string    `gorm:"column:phone_hash;uniqueIndex" json:"-"`
	PhoneEncrypted   []byte    `gorm:"column:phone_encrypted" json:"-"`
	Nickname         string    `gorm:"column:nickname" json:"nickname"`
	SubscriptionTier string    `gorm:"column:subscription_tier" json:"subscription_tier"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"-"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"-"`
}

// TableName returns the SQL table name for User. Required because GORM's
// default pluralization uses "users" already, but we make it explicit.
func (User) TableName() string { return "users" }
