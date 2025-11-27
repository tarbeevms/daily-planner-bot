package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"daily-planner/internal/model"
	"daily-planner/internal/repository"
)

// ReminderService builds human-readable summaries for daily notifications.
type ReminderService struct {
	taskRepo     *repository.TaskRepository
	categoryRepo *repository.CategoryRepository
}

func NewReminderService(taskRepo *repository.TaskRepository, categoryRepo *repository.CategoryRepository) *ReminderService {
	return &ReminderService{taskRepo: taskRepo, categoryRepo: categoryRepo}
}

func (s *ReminderService) DailySummary(user model.User, now time.Time) (string, error) {
	tasks, err := s.taskRepo.ListActiveOrRecurring(user.ID)
	if err != nil {
		return "", err
	}

	categories, err := s.categoryRepo.ListByUser(user.ID)
	if err != nil {
		return "", err
	}
	catNames := make(map[uint]string)
	for _, cat := range categories {
		catNames[cat.ID] = cat.Name
	}

	var pending []model.Task
	var recurringDue []model.Task

	for _, task := range tasks {
		if task.IsRecurring {
			if s.recurringDue(task, now) {
				recurringDue = append(recurringDue, task)
			}
			continue
		}
		if !task.IsCompleted {
			pending = append(pending, task)
		}
	}

	sort.SliceStable(pending, func(i, j int) bool {
		switch {
		case pending[i].Deadline == nil && pending[j].Deadline == nil:
			return pending[i].CreatedAt.After(pending[j].CreatedAt)
		case pending[i].Deadline == nil:
			return false
		case pending[j].Deadline == nil:
			return true
		default:
			return pending[i].Deadline.Before(*pending[j].Deadline)
		}
	})

	var builder strings.Builder
	builder.WriteString("📋 Ежедневный отчет\n\n")

	builder.WriteString("Текущие задачи:\n")
	if len(pending) == 0 {
		builder.WriteString("— нет активных задач\n")
	} else {
		for _, task := range pending {
			builder.WriteString(formatTask(task, catNames))
		}
	}

	builder.WriteString("\nРегулярные задачи этого месяца:\n")
	if len(recurringDue) == 0 {
		builder.WriteString("— нет задач в окне выполнения\n")
	} else {
		for _, task := range recurringDue {
			builder.WriteString(formatRecurring(task, now, catNames))
		}
	}

	return builder.String(), nil
}

func (s *ReminderService) recurringDue(task model.Task, now time.Time) bool {
	if !task.IsRecurring || strings.ToLower(task.RecurType) != "monthly" || task.RecurDay <= 0 {
		return false
	}

	year, month, _ := now.Date()
	dueDay := task.RecurDay
	endOfMonth := daysInMonth(month, year)
	if dueDay > endOfMonth {
		dueDay = endOfMonth
	}

	dueDate := time.Date(year, month, dueDay, 0, 0, 0, 0, now.Location())
	window := time.Duration(task.RecurWindow) * 24 * time.Hour
	start := dueDate.Add(-window)
	end := dueDate.Add(window)

	if now.Before(start) || now.After(end) {
		return false
	}

	if task.LastCompletedAt != nil {
		// If already completed inside the window for this month, skip.
		if !task.LastCompletedAt.Before(start) && !task.LastCompletedAt.After(end) &&
			task.LastCompletedAt.Month() == now.Month() && task.LastCompletedAt.Year() == now.Year() {
			return false
		}
	}

	return true
}

func formatTask(task model.Task, catNames map[uint]string) string {
	var sb strings.Builder
	sb.WriteString("• ")
	sb.WriteString(task.Title)
	if task.Description != "" {
		sb.WriteString(fmt.Sprintf(" — %s", task.Description))
	}
	if task.CategoryID != nil {
		if name, ok := catNames[*task.CategoryID]; ok {
			sb.WriteString(fmt.Sprintf(" [раздел: %s]", name))
		}
	}
	if task.Deadline != nil {
		sb.WriteString(fmt.Sprintf(" (дедлайн: %s)", task.Deadline.Format("2006-01-02")))
	}
	sb.WriteByte('\n')
	return sb.String()
}

func formatRecurring(task model.Task, now time.Time, catNames map[uint]string) string {
	var sb strings.Builder
	sb.WriteString("• ")
	sb.WriteString(task.Title)
	if task.CategoryID != nil {
		if name, ok := catNames[*task.CategoryID]; ok {
			sb.WriteString(fmt.Sprintf(" [раздел: %s]", name))
		}
	}

	year, month, _ := now.Date()
	dueDay := task.RecurDay
	endOfMonth := daysInMonth(month, year)
	if dueDay > endOfMonth {
		dueDay = endOfMonth
	}
	dueDate := time.Date(year, month, dueDay, 0, 0, 0, 0, now.Location())

	sb.WriteString(fmt.Sprintf(" (дата: %s, окно ±%d дн.)", dueDate.Format("2006-01-02"), task.RecurWindow))
	if task.LastCompletedAt != nil {
		sb.WriteString(fmt.Sprintf(" [выполнено: %s]", task.LastCompletedAt.Format("2006-01-02")))
	}
	sb.WriteByte('\n')
	return sb.String()
}

func daysInMonth(month time.Month, year int) int {
	// Move to next month, roll back a day.
	firstOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	firstOfNextMonth := firstOfMonth.AddDate(0, 1, 0)
	lastOfMonth := firstOfNextMonth.AddDate(0, 0, -1)
	return lastOfMonth.Day()
}
