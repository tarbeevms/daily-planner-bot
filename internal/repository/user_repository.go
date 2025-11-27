package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"daily-planner/internal/model"
)

// UserRepository handles CRUD for users.
type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// UpsertFromTelegram finds or creates a user based on TelegramID and updates basic profile info.
func (r *UserRepository) UpsertFromTelegram(ctx context.Context, telegramID int64, firstName, lastName, username string) (*model.User, error) {
	var user model.User
	db := r.db.WithContext(ctx)
	err := db.Where("telegram_id = ?", telegramID).First(&user).Error
	switch {
	case err == nil:
		updates := map[string]interface{}{
			"first_name": firstName,
			"last_name":  lastName,
			"username":   username,
		}
		if err := db.Model(&user).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("update user: %w", err)
		}
		return &user, nil
	case err == gorm.ErrRecordNotFound:
		user = model.User{
			TelegramID: telegramID,
			FirstName:  firstName,
			LastName:   lastName,
			Username:   username,
		}
		if err := db.Create(&user).Error; err != nil {
			return nil, fmt.Errorf("create user: %w", err)
		}
		return &user, nil
	default:
		return nil, fmt.Errorf("find user: %w", err)
	}
}

func (r *UserRepository) FindByTelegramID(ctx context.Context, telegramID int64) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).Where("telegram_id = ?", telegramID).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) ListAll(ctx context.Context) ([]model.User, error) {
	var users []model.User
	if err := r.db.WithContext(ctx).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}
