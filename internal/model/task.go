package model

import "time"

// Task represents a single item in the planner.
type Task struct {
	ID              uint  `gorm:"primaryKey"`
	UserID          uint  `gorm:"index"`
	CategoryID      *uint `gorm:"index"`
	Title           string
	Description     string
	Deadline        *time.Time
	IsCompleted     bool   `gorm:"default:false"`
	IsRecurring     bool   `gorm:"default:false"`
	RecurType       string // e.g. monthly
	RecurDay        int
	RecurWindow     int
	LastCompletedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
