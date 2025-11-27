package repository

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"daily-planner/internal/model"
)

// NewDB opens a SQLite database and runs migrations.
func NewDB(dsn string) (*gorm.DB, error) {
	if dsn == "" {
		dsn = "daily_planner.db"
	}

	if err := ensureDirForSQLite(dsn); err != nil {
		return nil, err
	}

	dbLogger := logger.New(
		log.New(os.Stdout, "", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: dbLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.AutoMigrate(&model.User{}, &model.Category{}, &model.Task{}); err != nil {
		return nil, fmt.Errorf("migrate db: %w", err)
	}

	return db, nil
}

// ensureDirForSQLite creates parent dir for SQLite file if needed.
func ensureDirForSQLite(dsn string) error {
	// Ignore DSNs with explicit mode=memory or network.
	if strings.Contains(dsn, ":memory:") || strings.Contains(dsn, "mode=memory") {
		return nil
	}
	// Strip file: prefix if present.
	clean := strings.TrimPrefix(dsn, "file:")
	clean = strings.Split(clean, "?")[0]
	dir := filepath.Dir(clean)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create db dir %q: %w", dir, err)
	}
	return nil
}
