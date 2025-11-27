package repository

import (
	"fmt"

	"gorm.io/gorm"

	"daily-planner/internal/model"
)

// CategoryRepository manages task categories.
type CategoryRepository struct {
	db *gorm.DB
}

func NewCategoryRepository(db *gorm.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

func (r *CategoryRepository) GetOrCreate(userID uint, name string) (*model.Category, error) {
	if name == "" {
		return nil, nil
	}

	var category model.Category
	err := r.db.Where("user_id = ? AND name = ?", userID, name).First(&category).Error
	switch {
	case err == nil:
		return &category, nil
	case err == gorm.ErrRecordNotFound:
		category = model.Category{UserID: userID, Name: name}
		if err := r.db.Create(&category).Error; err != nil {
			return nil, fmt.Errorf("create category: %w", err)
		}
		return &category, nil
	default:
		return nil, fmt.Errorf("find category: %w", err)
	}
}

func (r *CategoryRepository) ListByUser(userID uint) ([]model.Category, error) {
	var categories []model.Category
	if err := r.db.Where("user_id = ?", userID).Order("name ASC").Find(&categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *CategoryRepository) GetByID(id uint) (*model.Category, error) {
	var category model.Category
	if err := r.db.First(&category, id).Error; err != nil {
		return nil, err
	}
	return &category, nil
}
