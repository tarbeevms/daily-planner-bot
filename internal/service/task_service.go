package service

import (
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

func (s *TaskService) CreateTask(user *model.User, input TaskInput) (*model.Task, error) {
	if input.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	var categoryID *uint
	if input.Category != "" {
		category, err := s.categoryRepo.GetOrCreate(user.ID, input.Category)
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

	if err := s.taskRepo.Create(&task); err != nil {
		return nil, err
	}

	return &task, nil
}

func (s *TaskService) ListActive(user *model.User) ([]model.Task, error) {
	return s.taskRepo.ListActiveOrRecurring(user.ID)
}

func (s *TaskService) ListAll(user *model.User) ([]model.Task, error) {
	return s.taskRepo.ListAll(user.ID)
}

func (s *TaskService) GetTask(user *model.User, taskID uint) (*model.Task, error) {
	return s.taskRepo.FindByID(user.ID, taskID)
}

// CompleteTask marks a task as done. For recurring tasks, it stores completion time without closing the task forever.
func (s *TaskService) CompleteTask(user *model.User, taskID uint, completedAt time.Time) (*model.Task, error) {
	task, err := s.taskRepo.FindByID(user.ID, taskID)
	if err != nil {
		return nil, err
	}

	if task.IsRecurring {
		if err := s.taskRepo.MarkRecurringDone(task, completedAt); err != nil {
			return nil, err
		}
		return task, nil
	}

	if err := s.taskRepo.MarkCompleted(task, completedAt); err != nil {
		return nil, err
	}
	return task, nil
}
