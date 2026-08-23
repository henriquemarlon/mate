package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/henriquemarlon/mate/internal/domain/entity"
)

// observePage runs the page state machine for one observed hash: a new page
// is created pending, an unchanged page is ignored, a changed unprocessed
// page is reprocessed, and a changed processed page is sent to review.
func (s *Service) observePage(noteID string, pageNumber int, hash string) (entity.PageAction, error) {
	if strings.TrimSpace(hash) == "" {
		return "", fmt.Errorf("%w: observed hash cannot be empty", entity.ErrInvalidPage)
	}

	page, err := s.repo.FindPage(noteID, pageNumber)
	if errors.Is(err, entity.ErrPageNotFound) {
		page, err := entity.NewPage(noteID, pageNumber, hash, entity.PageStatusPending)
		if err != nil {
			return "", err
		}
		if err := s.repo.CreatePage(page); err != nil {
			return "", err
		}
		return entity.PageActionProcess, nil
	}
	if err != nil {
		return "", err
	}
	if page.ProcessedHash == hash {
		return entity.PageActionIgnore, nil
	}
	if page.ProcessedHash == "" && page.Status == entity.PageStatusNeedsReview && page.ObservedHash == hash {
		return entity.PageActionIgnore, nil
	}

	page.ObservedHash = hash
	if page.ProcessedHash == "" {
		page.Status = entity.PageStatusPending
		if err := s.repo.UpdatePage(&page); err != nil {
			return "", err
		}
		return entity.PageActionProcess, nil
	}
	page.Status = entity.PageStatusNeedsReview
	if err := s.repo.UpdatePage(&page); err != nil {
		return "", err
	}
	return entity.PageActionNeedsReview, nil
}

func (s *Service) markPageProcessed(noteID string, pageNumber int, hash, transcription string, status entity.PageStatus) error {
	if strings.TrimSpace(hash) == "" {
		return fmt.Errorf("%w: processed hash cannot be empty", entity.ErrInvalidPage)
	}
	if status != entity.PageStatusTranscribed && status != entity.PageStatusSkipped {
		return fmt.Errorf("%w: cannot mark page as processed with status %q", entity.ErrInvalidPage, status)
	}

	page, err := s.repo.FindPage(noteID, pageNumber)
	if err != nil {
		return err
	}
	page.ObservedHash = hash
	page.ProcessedHash = hash
	page.Transcription = transcription
	page.Status = status
	return s.repo.UpdatePage(&page)
}

func (s *Service) markPageNeedsReview(noteID string, pageNumber int, hash, transcription string) error {
	if strings.TrimSpace(hash) == "" {
		return fmt.Errorf("%w: review hash cannot be empty", entity.ErrInvalidPage)
	}

	page, err := s.repo.FindPage(noteID, pageNumber)
	if err != nil {
		return err
	}
	page.ObservedHash = hash
	page.Transcription = transcription
	page.Status = entity.PageStatusNeedsReview
	return s.repo.UpdatePage(&page)
}

func (s *Service) markPagesDone(pages []entity.Page) error {
	for i := range pages {
		if pages[i].Status != entity.PageStatusTranscribed {
			return fmt.Errorf("%w: only a transcribed page can be completed", entity.ErrInvalidPage)
		}
		pages[i].Status = entity.PageStatusDone
	}
	return s.repo.UpdatePages(pages)
}
