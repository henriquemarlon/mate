package sqlite

import (
	"errors"
	"fmt"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"gorm.io/gorm"
)

func (r *SQLiteRepository) SaveMaterial(material *entity.Material) error {
	if err := r.DB.Save(material).Error; err != nil {
		return fmt.Errorf("repository: save material: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) FindMaterial(noteID string) (entity.Material, error) {
	var material entity.Material
	err := r.DB.Where("note_id = ?", noteID).First(&material).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return material, fmt.Errorf("%w: note %q", entity.ErrMaterialNotFound, noteID)
	}
	if err != nil {
		return material, fmt.Errorf("repository: find material: %w", err)
	}
	return material, nil
}
