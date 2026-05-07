package model

import "time"

// Child maps to the `children` table.
type Child struct {
	ID        int64     `gorm:"primaryKey;column:id" json:"id"`
	UserID    int64     `gorm:"column:user_id;uniqueIndex:children_user_unique" json:"user_id"`
	Nickname  string    `gorm:"column:nickname" json:"nickname"`
	Gender    string    `gorm:"column:gender" json:"gender"`
	Birthday  time.Time `gorm:"column:birthday;type:date" json:"birthday"`
	Profile   []byte    `gorm:"column:profile;type:jsonb" json:"-"`
	CreatedAt time.Time `gorm:"column:created_at" json:"-"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"-"`
}

// TableName returns the SQL table name for Child.
func (Child) TableName() string { return "children" }
