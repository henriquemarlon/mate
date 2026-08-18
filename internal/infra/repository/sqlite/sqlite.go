package sqlite

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/repository"
)

type SQLiteRepository struct {
	DB *gorm.DB
}

var _ repository.Repository = (*SQLiteRepository)(nil)

func (r *SQLiteRepository) Close() error {
	sqlDB, err := r.DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}
	return sqlDB.Close()
}

func NewSQLiteRepository(ctx context.Context, conn string) (*SQLiteRepository, error) {
	dbPath := strings.TrimPrefix(conn, "sqlite://")
	isMemory := dbPath == ":memory:"

	dsn := dbPath
	if !isMemory {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
		dsn = dbPath + "?_busy_timeout=5000&_journal_mode=WAL"
	}

	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Info,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:  newLogger,
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Add context to DB
	db = db.WithContext(ctx)

	// Optional: check DB connection
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping SQLite: %w", err)
	}

	if err := db.AutoMigrate(&entity.Page{}); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate tables: %w", err)
	}

	if !isMemory {
		if err := os.Chmod(dbPath, 0o600); err != nil {
			return nil, fmt.Errorf("failed to protect SQLite database: %w", err)
		}
	}

	return &SQLiteRepository{DB: db}, nil
}
