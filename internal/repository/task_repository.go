package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"daily-planner/internal/model"
)

// TaskRepository handles CRUD for tasks.
type TaskRepository struct {
	db *gorm.DB
}

func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(ctx context.Context, task *model.Task) error {
	if err := r.db.WithContext(ctx).Create(task).Error; err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	return nil
}

func (r *TaskRepository) ListActiveOrRecurring(ctx context.Context, userID uint) ([]model.Task, error) {
	var tasks []model.Task
	if err := r.db.WithContext(ctx).Where("user_id = ? AND (is_completed = ? OR is_recurring = ?)", userID, false, true).
		Order("deadline NULLS LAST, created_at DESC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *TaskRepository) FindByID(ctx context.Context, userID, taskID uint) (*model.Task, error) {
	var task model.Task
	if err := r.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, taskID).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *TaskRepository) MarkCompleted(ctx context.Context, task *model.Task, completedAt time.Time) error {
	task.IsCompleted = true
	task.LastCompletedAt = &completedAt
	if err := r.db.WithContext(ctx).Save(task).Error; err != nil {
		return fmt.Errorf("complete task: %w", err)
	}
	return nil
}

func (r *TaskRepository) MarkRecurringDone(ctx context.Context, task *model.Task, completedAt time.Time) error {
	task.LastCompletedAt = &completedAt
	if err := r.db.WithContext(ctx).Save(task).Error; err != nil {
		return fmt.Errorf("mark recurring done: %w", err)
	}
	return nil
}

// Delete removes a task for the given user, regardless of it being recurring or not.
func (r *TaskRepository) Delete(ctx context.Context, userID, taskID uint) error {
	if err := r.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, taskID).
		Delete(&model.Task{}).Error; err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}
