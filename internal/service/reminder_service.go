package service

import (
	"context"
	"fmt"
	"html"
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

func (s *ReminderService) DailySummary(ctx context.Context, user model.User, now time.Time) (string, error) {
	tasks, err := s.taskRepo.ListActiveOrRecurring(ctx, user.ID)
	if err != nil {
		return "", err
	}

	categories, err := s.categoryRepo.ListByUser(ctx, user.ID)
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
	builder.WriteString("📋 <b>Ежедневный отчёт</b>\n")
	builder.WriteString(fmt.Sprintf("🗓 %s\n\n", now.Format("02.01.2006")))

	builder.WriteString("🔥 <b>Текущие задачи</b>\n")
	if len(pending) == 0 {
		builder.WriteString("— нет открытых задач\n")
	} else {
		for _, task := range pending {
			builder.WriteString(formatTask(task, catNames, now))
		}
	}

	builder.WriteString("\n♻️ <b>Регулярные задачи</b>\n")
	if len(recurringDue) == 0 {
		builder.WriteString("— нет задач в окне выполнения\n")
	} else {
		for _, task := range recurringDue {
			builder.WriteString(formatRecurring(task, now, catNames))
		}
	}

	return strings.TrimSpace(builder.String()), nil
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
		if !task.LastCompletedAt.Before(start) && !task.LastCompletedAt.After(end) &&
			task.LastCompletedAt.Month() == now.Month() && task.LastCompletedAt.Year() == now.Year() {
			return false
		}
	}

	return true
}

func formatTask(task model.Task, catNames map[uint]string, now time.Time) string {
	var sb strings.Builder

	icon := "🟢"
	if task.Deadline != nil {
		d := task.Deadline.In(now.Location())
		switch {
		case now.After(d):
			icon = "⚠️"
		case d.Sub(now) <= 48*time.Hour:
			icon = "⏳"
		}
	}

	title := html.EscapeString(strings.TrimSpace(task.Title))
	sb.WriteString(fmt.Sprintf("%s %s", icon, title))

	if task.CategoryID != nil {
		if name, ok := catNames[*task.CategoryID]; ok {
			trimmed := strings.TrimSpace(name)
			if trimmed != "" {
				sb.WriteString(fmt.Sprintf(" <i>(%s)</i>", html.EscapeString(trimmed)))
			}
		}
	}

	if task.Deadline != nil {
		d := task.Deadline.In(now.Location())
		if now.After(d) {
			sb.WriteString(fmt.Sprintf("\n   ⏰ до %s — <b>просрочено</b>", d.Format("2006-01-02")))
		} else {
			daysLeft := int(d.Sub(now).Hours()/24) + 1
			sb.WriteString(fmt.Sprintf("\n   ⏰ до %s · осталось ≈%d дн.", d.Format("2006-01-02"), daysLeft))
		}
	}

	if task.Description != "" {
		sb.WriteString(fmt.Sprintf("\n   📝 %s", html.EscapeString(strings.TrimSpace(task.Description))))
	}

	sb.WriteByte('\n')
	return sb.String()
}

func formatRecurring(task model.Task, now time.Time, catNames map[uint]string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("♻️ %s", html.EscapeString(strings.TrimSpace(task.Title))))

	if task.CategoryID != nil {
		if name, ok := catNames[*task.CategoryID]; ok {
			trimmed := strings.TrimSpace(name)
			if trimmed != "" {
				sb.WriteString(fmt.Sprintf(" <i>(%s)</i>", html.EscapeString(trimmed)))
			}
		}
	}

	year, month, _ := now.Date()
	dueDay := task.RecurDay
	endOfMonth := daysInMonth(month, year)
	if dueDay > endOfMonth {
		dueDay = endOfMonth
	}
	dueDate := time.Date(year, month, dueDay, 0, 0, 0, 0, now.Location())

	sb.WriteString(fmt.Sprintf("\n   📆 Ближайшая дата: %s (окно ±%d дн.)", dueDate.Format("2006-01-02"), task.RecurWindow))
	if task.LastCompletedAt != nil {
		sb.WriteString(fmt.Sprintf("\n   ✅ Последнее выполнение: %s", task.LastCompletedAt.In(now.Location()).Format("2006-01-02")))
	} else {
		sb.WriteString("\n   ✅ Пока не выполнялась")
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
