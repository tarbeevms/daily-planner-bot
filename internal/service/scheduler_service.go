package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// SchedulerService wraps cron-based jobs.
type SchedulerService struct {
	cron *cron.Cron
}

func NewSchedulerService(loc *time.Location) *SchedulerService {
	return &SchedulerService{
		cron: cron.New(cron.WithLocation(loc), cron.WithSeconds()),
	}
}

// ScheduleDaily registers a daily job at the given HH:MM time string.
func (s *SchedulerService) ScheduleDaily(timeStr string, job func()) (cron.EntryID, error) {
	spec, err := buildDailySpec(timeStr)
	if err != nil {
		return 0, err
	}
	return s.cron.AddFunc(spec, job)
}

func (s *SchedulerService) Start() {
	s.cron.Start()
}

func (s *SchedulerService) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// ScheduleInterval registers a periodic job every given duration.
func (s *SchedulerService) ScheduleInterval(interval time.Duration, job func()) (cron.EntryID, error) {
	if interval <= 0 {
		return 0, fmt.Errorf("interval must be positive")
	}
	// Convert to cron spec: every N seconds.
	seconds := int(interval.Seconds())
	if seconds <= 0 {
		seconds = 1
	}
	spec := fmt.Sprintf("@every %ds", seconds)
	return s.cron.AddFunc(spec, job)
}

func buildDailySpec(timeStr string) (string, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid time %q, expected HH:MM", timeStr)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return "", fmt.Errorf("invalid hour in %q", timeStr)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return "", fmt.Errorf("invalid minute in %q", timeStr)
	}
	// cron format: second minute hour dom month dow
	return fmt.Sprintf("0 %d %d * * *", minute, hour), nil
}
