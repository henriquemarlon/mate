package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/llm/transcriber"
)

var (
	ErrInvalidReview    = errors.New("invalid review")
	ErrUnresolvedReview = errors.New("unresolved review")
)

// ReviewItem is the command-facing view of one page waiting for a person.
// Changed distinguishes an uncertain new transcription from a page that had
// already produced material before its PDF pixels changed.
type ReviewItem struct {
	NoteID        string
	PageNumber    int
	Transcription string
	ImagePath     string
	Status        entity.PageStatus
	Changed       bool
}

// PendingReviews returns review work in deterministic notebook/page order.
// Empty filters mean all notebooks and all pages.
func (s *Service) PendingReviews(noteID string, pageNumber int) ([]ReviewItem, error) {
	if pageNumber < 0 {
		return nil, fmt.Errorf("%w: page number cannot be negative", ErrInvalidReview)
	}
	pages, err := s.repo.FindAllPagesByStatus(entity.PageStatusNeedsReview)
	if err != nil {
		return nil, err
	}
	noteID = filepath.ToSlash(strings.TrimSpace(noteID))
	items := make([]ReviewItem, 0, len(pages))
	for _, page := range pages {
		if noteID != "" && page.NoteID != noteID {
			continue
		}
		if pageNumber > 0 && page.PageNumber != pageNumber {
			continue
		}
		item, err := s.reviewItem(page)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// UpdateReviewDraft persists partial human corrections without allowing the
// page into generated material while an uncertainty marker remains.
func (s *Service) UpdateReviewDraft(noteID string, pageNumber int, markdown string) (ReviewItem, error) {
	page, err := s.reviewPage(noteID, pageNumber)
	if err != nil {
		return ReviewItem{}, err
	}
	page.Transcription = strings.TrimSpace(markdown)
	if err := s.repo.UpdatePage(&page); err != nil {
		return ReviewItem{}, err
	}
	return s.reviewItem(page)
}

// CorrectReview accepts a complete human transcription and resumes the same
// material/Anki completion path used by the unattended workflow.
func (s *Service) CorrectReview(ctx context.Context, noteID string, pageNumber int, markdown string) (ReviewItem, error) {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return ReviewItem{}, fmt.Errorf("%w: corrected transcription cannot be empty", ErrInvalidReview)
	}
	if strings.Contains(markdown, "[?]") {
		return ReviewItem{}, fmt.Errorf("%w: corrected transcription still contains [?]", ErrUnresolvedReview)
	}
	page, err := s.reviewPage(noteID, pageNumber)
	if err != nil {
		return ReviewItem{}, err
	}
	if err := s.markPageProcessed(page.NoteID, page.PageNumber, page.ObservedHash, markdown, entity.PageStatusTranscribed); err != nil {
		return ReviewItem{}, err
	}
	s.discardReviewPage(page)
	if err := s.completeNote(ctx, page.NoteID); err != nil {
		return ReviewItem{}, err
	}
	return s.currentReviewItem(page.NoteID, page.PageNumber)
}

// RetryReview asks the transcriber to inspect the current PDF pixels again.
// A still-uncertain response replaces the draft and annotated image; a clean
// response immediately resumes generation and synchronization.
func (s *Service) RetryReview(ctx context.Context, noteID string, pageNumber int) (ReviewItem, error) {
	page, err := s.reviewPage(noteID, pageNumber)
	if err != nil {
		return ReviewItem{}, err
	}
	rendered, pagePNG, err := s.renderReviewPage(ctx, page)
	if err != nil {
		return ReviewItem{}, err
	}
	output, err := s.transcriber.Transcribe(ctx, transcriber.TranscribeInputDTO{ImageData: pagePNG})
	if err != nil {
		return ReviewItem{}, err
	}

	if output.NeedsReview || output.Kind == "unknown" || strings.Contains(output.Markdown, "[?]") {
		if err := s.markPageNeedsReview(page.NoteID, page.PageNumber, rendered.Hash, output.Markdown); err != nil {
			return ReviewItem{}, err
		}
		boxes := make([][]int, 0, len(output.Uncertainties))
		for _, uncertainty := range output.Uncertainties {
			boxes = append(boxes, uncertainty.BBox)
		}
		if err := writeReviewPage(s.config.OutputDir, page.NoteID, page.PageNumber, pagePNG, boxes); err != nil {
			return ReviewItem{}, err
		}
		return s.currentReviewItem(page.NoteID, page.PageNumber)
	}

	switch output.Kind {
	case "cover", "blank":
		if page.ProcessedHash != "" {
			return ReviewItem{}, fmt.Errorf("%w: a previously processed page is now %s; edit its transcription or keep the previous version", ErrInvalidReview, output.Kind)
		}
		if err := s.markPageProcessed(page.NoteID, page.PageNumber, rendered.Hash, "", entity.PageStatusSkipped); err != nil {
			return ReviewItem{}, err
		}
	case "content":
		if err := s.markPageProcessed(page.NoteID, page.PageNumber, rendered.Hash, output.Markdown, entity.PageStatusTranscribed); err != nil {
			return ReviewItem{}, err
		}
	default:
		return ReviewItem{}, fmt.Errorf("%w: unsupported transcription kind %q", ErrInvalidReview, output.Kind)
	}

	s.discardReviewPage(page)
	if err := s.completeNote(ctx, page.NoteID); err != nil {
		return ReviewItem{}, err
	}
	return s.currentReviewItem(page.NoteID, page.PageNumber)
}

// SkipReview deliberately excludes a page that never produced material. A
// changed processed page cannot be skipped because its earlier cards cannot
// yet be attributed and removed safely.
func (s *Service) SkipReview(ctx context.Context, noteID string, pageNumber int) (ReviewItem, error) {
	page, err := s.reviewPage(noteID, pageNumber)
	if err != nil {
		return ReviewItem{}, err
	}
	if page.ProcessedHash != "" {
		return ReviewItem{}, fmt.Errorf("%w: a previously processed page cannot be skipped; retry, edit, or keep it", ErrInvalidReview)
	}
	if err := s.markPageProcessed(page.NoteID, page.PageNumber, page.ObservedHash, "", entity.PageStatusSkipped); err != nil {
		return ReviewItem{}, err
	}
	s.discardReviewPage(page)
	if err := s.completeNote(ctx, page.NoteID); err != nil {
		return ReviewItem{}, err
	}
	return s.currentReviewItem(page.NoteID, page.PageNumber)
}

// KeepReview acknowledges a pixel-only edit to a processed page while
// preserving the transcription and all material derived from it.
func (s *Service) KeepReview(noteID string, pageNumber int) (ReviewItem, error) {
	page, err := s.reviewPage(noteID, pageNumber)
	if err != nil {
		return ReviewItem{}, err
	}
	if page.ProcessedHash == "" {
		return ReviewItem{}, fmt.Errorf("%w: an unprocessed page has no previous transcription to keep", ErrInvalidReview)
	}
	page.ProcessedHash = page.ObservedHash
	page.Status = entity.PageStatusDone
	if err := s.repo.UpdatePage(&page); err != nil {
		return ReviewItem{}, err
	}
	s.discardReviewPage(page)
	return s.reviewItem(page)
}

// discardReviewPage drops a resolved page's annotated image. The page is
// already resolved in storage, so a leftover file is untidy, never a failure.
func (s *Service) discardReviewPage(page entity.Page) {
	if err := removeReviewPage(s.config.OutputDir, page.NoteID, page.PageNumber); err != nil {
		s.Logger.Warn("resolved review image could not be removed", "note", page.NoteID, "page", page.PageNumber, "error", err)
	}
}

func (s *Service) reviewPage(noteID string, pageNumber int) (entity.Page, error) {
	page, err := s.repo.FindPage(filepath.ToSlash(strings.TrimSpace(noteID)), pageNumber)
	if err != nil {
		return entity.Page{}, err
	}
	if page.Status != entity.PageStatusNeedsReview {
		return entity.Page{}, fmt.Errorf("%w: %s page %d is %s", ErrInvalidReview, page.NoteID, page.PageNumber, page.Status)
	}
	return page, nil
}

func (s *Service) currentReviewItem(noteID string, pageNumber int) (ReviewItem, error) {
	page, err := s.repo.FindPage(noteID, pageNumber)
	if err != nil {
		return ReviewItem{}, err
	}
	return s.reviewItem(page)
}

func (s *Service) reviewItem(page entity.Page) (ReviewItem, error) {
	path, err := reviewPagePath(s.config.OutputDir, page.NoteID, page.PageNumber)
	if err != nil {
		return ReviewItem{}, err
	}
	return ReviewItem{
		NoteID:        page.NoteID,
		PageNumber:    page.PageNumber,
		Transcription: page.Transcription,
		ImagePath:     path,
		Status:        page.Status,
		Changed:       page.ProcessedHash != "",
	}, nil
}

func (s *Service) renderReviewPage(ctx context.Context, page entity.Page) (renderedPage, []byte, error) {
	clean := filepath.Clean(filepath.FromSlash(page.NoteID))
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return renderedPage{}, nil, fmt.Errorf("%w: invalid note path %q", ErrInvalidReview, page.NoteID)
	}
	pdfPath := filepath.Join(s.config.StudyDir, clean)
	dir, pages, err := renderPDF(ctx, pdfPath, s.config.DPI)
	if err != nil {
		return renderedPage{}, nil, err
	}
	defer os.RemoveAll(dir)
	for _, rendered := range pages {
		if rendered.Number != page.PageNumber {
			continue
		}
		content, err := os.ReadFile(rendered.Path)
		if err != nil {
			return renderedPage{}, nil, fmt.Errorf("review: read rendered page: %w", err)
		}
		return rendered, content, nil
	}
	return renderedPage{}, nil, fmt.Errorf("%w: %s has no page %d", ErrInvalidReview, page.NoteID, page.PageNumber)
}
