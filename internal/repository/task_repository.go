package repository

import (
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

func (r *TaskRepository) Create(task *model.Task) error {
	if err := r.db.Create(task).Error; err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	return nil
}

func (r *TaskRepository) ListActiveOrRecurring(userID uint) ([]model.Task, error) {
	var tasks []model.Task
	if err := r.db.Where("user_id = ? AND (is_completed = ? OR is_recurring = ?)", userID, false, true).
		Order("deadline NULLS LAST, created_at DESC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *TaskRepository) ListAll(userID uint) ([]model.Task, error) {
	var tasks []model.Task
	if err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *TaskRepository) FindByID(userID, taskID uint) (*model.Task, error) {
	var task model.Task
	if err := r.db.Where("user_id = ? AND id = ?", userID, taskID).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *TaskRepository) MarkCompleted(task *model.Task, completedAt time.Time) error {
	task.IsCompleted = true
	task.LastCompletedAt = &completedAt
	if err := r.db.Save(task).Error; err != nil {
		return fmt.Errorf("complete task: %w", err)
	}
	return nil
}

func (r *TaskRepository) MarkRecurringDone(task *model.Task, completedAt time.Time) error {
	task.LastCompletedAt = &completedAt
	if err := r.db.Save(task).Error; err != nil {
		return fmt.Errorf("mark recurring done: %w", err)
	}
	return nil
}
