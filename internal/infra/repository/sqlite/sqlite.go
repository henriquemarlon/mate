package sqlite

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/repository"
	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type SQLiteRepository struct {
	db *gorm.DB
}

var _ repository.Repository = (*SQLiteRepository)(nil)

func NewSQLiteRepository(path string) (*SQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("repository: create database directory: %w", err)
	}

	database, err := gorm.Open(gormsqlite.Open(path+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{
		Logger:  logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("repository: open SQLite: %w", err)
	}
	if err := database.AutoMigrate(&entity.Page{}); err != nil {
		closeDatabase(database)
		return nil, fmt.Errorf("repository: migrate SQLite schema: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		closeDatabase(database)
		return nil, fmt.Errorf("repository: protect SQLite database: %w", err)
	}
	return &SQLiteRepository{db: database}, nil
}

func (r *SQLiteRepository) Close() error {
	database, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("repository: access SQLite connection: %w", err)
	}
	return database.Close()
}

func closeDatabase(database *gorm.DB) {
	connection, err := database.DB()
	if err == nil {
		_ = connection.Close()
	}
}
