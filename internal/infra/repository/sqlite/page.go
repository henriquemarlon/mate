package sqlite

import (
	"errors"
	"fmt"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"gorm.io/gorm"
)

func (r *SQLiteRepository) CreatePage(page *entity.Page) error {
	if err := r.DB.Create(page).Error; err != nil {
		return fmt.Errorf("repository: create page: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) FindPage(noteID string, pageNumber int) (entity.Page, error) {
	var page entity.Page
	err := r.DB.Where("note_id = ? AND page_number = ?", noteID, pageNumber).First(&page).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return page, fmt.Errorf("%w: note %q page %d", entity.ErrPageNotFound, noteID, pageNumber)
	}
	if err != nil {
		return page, fmt.Errorf("repository: find page: %w", err)
	}
	return page, nil
}

func (r *SQLiteRepository) UpdatePage(page *entity.Page) error {
	if err := r.DB.Save(page).Error; err != nil {
		return fmt.Errorf("repository: update page: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) UpdatePages(pages []entity.Page) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		for i := range pages {
			if err := tx.Save(&pages[i]).Error; err != nil {
				return fmt.Errorf("repository: update page %d: %w", pages[i].PageNumber, err)
			}
		}
		return nil
	})
}

func (r *SQLiteRepository) FindProcessedPages(noteID string) ([]entity.Page, error) {
	var pages []entity.Page
	err := r.DB.Where("note_id = ? AND processed_hash <> ?", noteID, "").Order("page_number").Find(&pages).Error
	if err != nil {
		return nil, fmt.Errorf("repository: find processed pages: %w", err)
	}
	return pages, nil
}

func (r *SQLiteRepository) FindPagesByStatus(noteID string, status entity.PageStatus) ([]entity.Page, error) {
	var pages []entity.Page
	err := r.DB.Where("note_id = ? AND status = ?", noteID, status).Order("page_number").Find(&pages).Error
	if err != nil {
		return nil, fmt.Errorf("repository: find pages by status: %w", err)
	}
	return pages, nil
}
