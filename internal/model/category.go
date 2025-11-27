package model

import "time"

// Category groups tasks by area (work, health, study, etc.).
type Category struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"index"`
	Name      string `gorm:"index:idx_user_category_name,unique"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Tasks     []Task `gorm:"foreignKey:CategoryID"`
}
