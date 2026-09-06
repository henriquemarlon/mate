package entity

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidMaterial  = errors.New("invalid material")
	ErrMaterialNotFound = errors.New("material not found")
)

type Material struct {
	NoteID     string `json:"note_id" gorm:"primaryKey;not null"`
	SourceHash string `json:"source_hash" gorm:"not null"`
	SyncedHash string `json:"synced_hash,omitempty" gorm:"not null;default:''"`
	// FeynmanPromptsJSON keeps the existing "feynman" column, which held a
	// single Markdown script before sessions became a per-topic list.
	FeynmanPromptsJSON string    `json:"feynman" gorm:"column:feynman;type:text;not null"`
	CardsJSON          string    `json:"cards_json" gorm:"type:text;not null"`
	UpdatedAt          time.Time `json:"updated_at" gorm:"not null;autoUpdateTime"`
}

func NewMaterial(noteID, sourceHash, feynmanPromptsJSON, cardsJSON string) (*Material, error) {
	material := &Material{
		NoteID:             noteID,
		SourceHash:         sourceHash,
		FeynmanPromptsJSON: feynmanPromptsJSON,
		CardsJSON:          cardsJSON,
	}
	if err := material.validate(); err != nil {
		return nil, err
	}
	return material, nil
}

func (m *Material) MarkSynced() error {
	if err := m.validate(); err != nil {
		return err
	}
	m.SyncedHash = m.SourceHash
	return nil
}

func (m Material) IsSynced() bool {
	return m.SourceHash != "" && m.SourceHash == m.SyncedHash
}

func (m *Material) validate() error {
	if strings.TrimSpace(m.NoteID) == "" {
		return fmt.Errorf("%w: note ID cannot be empty", ErrInvalidMaterial)
	}
	if strings.TrimSpace(m.SourceHash) == "" {
		return fmt.Errorf("%w: source hash cannot be empty", ErrInvalidMaterial)
	}
	if strings.TrimSpace(m.FeynmanPromptsJSON) == "" {
		return fmt.Errorf("%w: Feynman prompts cannot be empty", ErrInvalidMaterial)
	}
	if strings.TrimSpace(m.CardsJSON) == "" {
		return fmt.Errorf("%w: cards cannot be empty", ErrInvalidMaterial)
	}
	return nil
}

func (Material) TableName() string {
	return "materials"
}
