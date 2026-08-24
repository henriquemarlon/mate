package entity

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidPage  = errors.New("invalid page")
	ErrPageNotFound = errors.New("page not found")
)

type PageStatus string

type PageAction string

const (
	PageStatusPending     PageStatus = "pending"
	PageStatusTranscribed PageStatus = "transcribed"
	PageStatusDone        PageStatus = "done"
	PageStatusSkipped     PageStatus = "skipped"
	PageStatusNeedsReview PageStatus = "needs_review"
)

const (
	PageActionIgnore      PageAction = "ignore"
	PageActionProcess     PageAction = "process"
	PageActionNeedsReview PageAction = "needs_review"
)

type Page struct {
	NoteID        string     `json:"note_id" gorm:"primaryKey;not null;index:idx_pages_note_status,priority:1"`
	PageNumber    int        `json:"page_number" gorm:"primaryKey;autoIncrement:false;not null"`
	ObservedHash  string     `json:"observed_hash" gorm:"not null"`
	ProcessedHash string     `json:"processed_hash,omitempty" gorm:"not null;default:''"`
	Transcription string     `json:"transcription,omitempty" gorm:"not null;default:''"`
	Status        PageStatus `json:"status" gorm:"type:text;not null;index:idx_pages_note_status,priority:2"`
	UpdatedAt     time.Time  `json:"updated_at" gorm:"not null;autoUpdateTime"`
}

func NewPage(noteID string, pageNumber int, observedHash string, status PageStatus) (*Page, error) {
	page := &Page{
		NoteID:       noteID,
		PageNumber:   pageNumber,
		ObservedHash: observedHash,
		Status:       status,
	}
	if err := page.validate(); err != nil {
		return nil, err
	}
	return page, nil
}

func (p *Page) validate() error {
	if strings.TrimSpace(p.NoteID) == "" {
		return fmt.Errorf("%w: note ID cannot be empty", ErrInvalidPage)
	}
	if p.PageNumber <= 0 {
		return fmt.Errorf("%w: page number must be positive", ErrInvalidPage)
	}
	if strings.TrimSpace(p.ObservedHash) == "" {
		return fmt.Errorf("%w: observed hash cannot be empty", ErrInvalidPage)
	}
	if !p.Status.valid() {
		return fmt.Errorf("%w: unknown status %q", ErrInvalidPage, p.Status)
	}
	return nil
}

func (s PageStatus) valid() bool {
	switch s {
	case PageStatusPending, PageStatusTranscribed, PageStatusDone, PageStatusSkipped, PageStatusNeedsReview:
		return true
	default:
		return false
	}
}

func (Page) TableName() string {
	return "pages"
}
