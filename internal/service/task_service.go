package service

import (
	"context"
	"fmt"
	"time"

	"daily-planner/internal/model"
	"daily-planner/internal/repository"
)

// TaskInput represents data required to create a task.
type TaskInput struct {
	Title       string
	Description string
	Category    string
	Deadline    *time.Time
	IsRecurring bool
	RecurDay    int
	RecurWindow int
}

// TaskService wraps task-related business logic.
type TaskService struct {
	taskRepo     *repository.TaskRepository
	categoryRepo *repository.CategoryRepository
}

func NewTaskService(taskRepo *repository.TaskRepository, categoryRepo *repository.CategoryRepository) *TaskService {
	return &TaskService{taskRepo: taskRepo, categoryRepo: categoryRepo}
}

func (s *TaskService) CreateTask(ctx context.Context, user *model.User, input TaskInput) (*model.Task, error) {
	if input.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	var categoryID *uint
	if input.Category != "" {
		category, err := s.categoryRepo.GetOrCreate(ctx, user.ID, input.Category)
		if err != nil {
			return nil, err
		}
		if category != nil {
			categoryID = &category.ID
		}
	}

	task := model.Task{
		UserID:      user.ID,
		CategoryID:  categoryID,
		Title:       input.Title,
		Description: input.Description,
		Deadline:    input.Deadline,
		IsRecurring: input.IsRecurring,
	}

	if input.IsRecurring {
		task.RecurType = "monthly"
		task.RecurDay = input.RecurDay
		task.RecurWindow = input.RecurWindow
	}

	if err := s.taskRepo.Create(ctx, &task); err != nil {
		return nil, err
	}

	return &task, nil
}

func (s *TaskService) ListActive(ctx context.Context, user *model.User) ([]model.Task, error) {
	return s.taskRepo.ListActiveOrRecurring(ctx, user.ID)
}

func (s *TaskService) GetTask(ctx context.Context, user *model.User, taskID uint) (*model.Task, error) {
	return s.taskRepo.FindByID(ctx, user.ID, taskID)
}

// CompleteTask marks a task as done. For recurring tasks, it stores completion time without closing the task forever.
func (s *TaskService) CompleteTask(ctx context.Context, user *model.User, taskID uint, completedAt time.Time) (*model.Task, error) {
	task, err := s.taskRepo.FindByID(ctx, user.ID, taskID)
	if err != nil {
		return nil, err
	}

	if task.IsRecurring {
		if err := s.taskRepo.MarkRecurringDone(ctx, task, completedAt); err != nil {
			return nil, err
		}
		return task, nil
	}

	if err := s.taskRepo.MarkCompleted(ctx, task, completedAt); err != nil {
		return nil, err
	}
	return task, nil
}

// DeleteTask removes a task completely (for both one-time and recurring tasks).
func (s *TaskService) DeleteTask(ctx context.Context, user *model.User, taskID uint) error {
	return s.taskRepo.Delete(ctx, user.ID, taskID)
}
