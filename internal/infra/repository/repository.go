package repository

import "github.com/henriquemarlon/mate/internal/domain/entity"

type PageRepository interface {
	CreatePage(page *entity.Page) error
	FindPage(noteID string, pageNumber int) (entity.Page, error)
	UpdatePage(page *entity.Page) error
	UpdatePages(pages []entity.Page) error
	FindProcessedPages(noteID string) ([]entity.Page, error)
	FindPagesByStatus(noteID string, status entity.PageStatus) ([]entity.Page, error)
}

type Repository interface {
	PageRepository
	Close() error
}
