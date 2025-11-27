package service

import (
	"context"

	"daily-planner/internal/model"
	"daily-planner/internal/repository"
)

// CategoryService provides helpers around categories.
type CategoryService struct {
	repo *repository.CategoryRepository
}

func NewCategoryService(repo *repository.CategoryRepository) *CategoryService {
	return &CategoryService{repo: repo}
}

func (s *CategoryService) List(ctx context.Context, user *model.User) ([]model.Category, error) {
	return s.repo.ListByUser(ctx, user.ID)
}
