package model

import "time"

// User stores Telegram user metadata.
type User struct {
	ID         uint  `gorm:"primaryKey"`
	TelegramID int64 `gorm:"uniqueIndex"`
	FirstName  string
	LastName   string
	Username   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
