package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"daily-planner/internal/bot"
	"daily-planner/internal/config"
	"daily-planner/internal/repository"
	"daily-planner/internal/service"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := repository.NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	sqlDB, err := db.DB()
	if err == nil {
		defer sqlDB.Close()
	}

	userRepo := repository.NewUserRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	taskRepo := repository.NewTaskRepository(db)

	categorySvc := service.NewCategoryService(categoryRepo)
	taskSvc := service.NewTaskService(taskRepo, categoryRepo)
	reminderSvc := service.NewReminderService(taskRepo, categoryRepo)

	telegramBot, err := bot.New(cfg.TelegramToken, userRepo, categorySvc, taskSvc, reminderSvc, &cfg)
	if err != nil {
		log.Fatalf("bot: %v", err)
	}

	scheduler := service.NewSchedulerService(time.Local)
	if cfg.ReportInterval > 0 {
		if _, err := scheduler.ScheduleInterval(cfg.ReportInterval, func() {
			jobCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := telegramBot.SendDailyReports(jobCtx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("report: %v", err)
			}
		}); err != nil {
			log.Fatalf("schedule reports: %v", err)
		}
		scheduler.Start()
		defer scheduler.Stop()
	}

	log.Println("Daily planner bot started.")
	if err := telegramBot.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("bot stopped with error: %v", err)
	}
	log.Println("Shutdown complete.")
}
