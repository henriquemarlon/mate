package service

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gen2brain/beeep"

	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/anki"
	"github.com/henriquemarlon/mate/internal/infra/llm/paradigm"
	"github.com/henriquemarlon/mate/internal/infra/llm/transcriber"
	"github.com/henriquemarlon/mate/pkg/llm"
	"github.com/henriquemarlon/mate/pkg/service"
)

// Repository defines the persistence interface needed by the Mate service.
type Repository interface {
	CreatePage(page *entity.Page) error
	FindPage(noteID string, pageNumber int) (entity.Page, error)
	UpdatePage(page *entity.Page) error
	UpdatePages(pages []entity.Page) error
	FindProcessedPages(noteID string) ([]entity.Page, error)
	FindPagesByStatus(noteID string, status entity.PageStatus) ([]entity.Page, error)
	SaveMaterial(material *entity.Material) error
	FindMaterial(noteID string) (entity.Material, error)
}

type Summary struct {
	NotesProcessed int
	NotesFailed    int
	PagesProcessed int
	NeedsReview    int
}

type Service struct {
	service.TickServiceTemplate
	config      configs.MateConfig
	repo        Repository
	transcriber *transcriber.Transcriber
	paradigm    *paradigm.Generator
	anki        *anki.Client
}

var _ service.SupervisedService = (*Service)(nil)
var _ service.TickImpl = (*Service)(nil)

const ServiceName = "mate"

// CreateInfo contains the configuration for creating the Mate service.
type CreateInfo struct {
	Config     configs.MateConfig
	Logger     *slog.Logger
	Repository Repository
	LLM        llm.Model
	Anki       *anki.Client
}

// Create initializes the Mate service from already-acquired dependencies.
// Resource lifetimes stay with the caller.
func Create(ctx context.Context, c *CreateInfo) (*Service, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Repository == nil {
		return nil, fmt.Errorf("repository on mate service Create is nil")
	}
	if c.LLM == nil {
		return nil, fmt.Errorf("llm client on mate service Create is nil")
	}
	if c.Anki == nil {
		return nil, fmt.Errorf("anki client on mate service Create is nil")
	}
	info, err := os.Stat(c.Config.StudyDir)
	if err != nil {
		return nil, fmt.Errorf("workflow: study directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workflow: study path is not a directory: %s", c.Config.StudyDir)
	}
	if err := os.MkdirAll(c.Config.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("workflow: create output directory: %w", err)
	}

	s := &Service{
		config:      c.Config,
		repo:        c.Repository,
		transcriber: transcriber.New(c.LLM),
		paradigm:    paradigm.New(c.LLM),
		anki:        c.Anki,
	}
	if err := service.InitTickServiceTemplate(&s.TickServiceTemplate, &service.TickServiceConfigs{
		BaseConfigs: service.BaseConfigs{
			Name:     ServiceName,
			Logger:   c.Logger,
			LogLevel: c.Config.LogLevel,
			LogColor: c.Config.LogColor,
		},
		PollInterval: c.Config.PollIntervalSeconds,
	}, s); err != nil {
		return nil, err
	}

	s.Logger.Info("Created", "config", c.Config)
	return s, nil
}

// Tick scans the study directory once. Every model call is its own isolated
// request, so a failed call is scoped to its page and no error here is
// terminal for the daemon.
func (s *Service) Tick(ctx context.Context) (bool, error) {
	_, err := s.run(ctx)
	return false, err
}

func (s *Service) run(ctx context.Context) (Summary, error) {
	var result Summary
	err := filepath.WalkDir(s.config.StudyDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".pdf") {
			return nil
		}
		noteSummary, err := s.processNote(ctx, path)
		result.PagesProcessed += noteSummary.PagesProcessed
		result.NeedsReview += noteSummary.NeedsReview
		if err != nil {
			// One broken note must not abort the batch: only give up when
			// the run itself was cancelled.
			if ctx.Err() != nil {
				return err
			}
			s.Logger.Error("note failed; continuing with next note", "note", path, "error", err)
			result.NotesFailed++
			return nil
		}
		if noteSummary.PagesProcessed > 0 || noteSummary.NeedsReview > 0 {
			result.NotesProcessed++
		}
		if s.config.Notifications && noteSummary.NeedsReview > 0 {
			// Best-effort desktop notification: the daemon runs unattended,
			// so this is the only signal that a page is waiting for a human.
			// Headless hosts have no notification server; failures are debug
			// noise, never a tick error.
			message := fmt.Sprintf("%s: %d page(s) need review", filepath.Base(path), noteSummary.NeedsReview)
			if err := beeep.Notify("Mate", message, ""); err != nil {
				s.Logger.Debug("review notification failed", "note", path, "error", err)
			}
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("workflow: scan study directory: %w", err)
	}
	s.Logger.Info("run complete",
		"notes", result.NotesProcessed,
		"notes_failed", result.NotesFailed,
		"pages", result.PagesProcessed,
		"needs_review", result.NeedsReview,
	)
	return result, nil
}

func (s *Service) processNote(ctx context.Context, pdfPath string) (Summary, error) {
	var result Summary
	relative, err := filepath.Rel(s.config.StudyDir, pdfPath)
	if err != nil {
		return result, fmt.Errorf("workflow: relative note path: %w", err)
	}
	noteID := filepath.ToSlash(relative)
	s.Logger.Info("rendering note", "note", noteID)
	renderDir, pages, err := renderPDF(ctx, pdfPath, s.config.DPI)
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(renderDir)

	for _, page := range pages {
		action, err := s.observePage(noteID, page.Number, page.Hash)
		if err != nil {
			return result, err
		}
		if action == entity.PageActionIgnore {
			continue
		}
		pagePNG, err := os.ReadFile(page.Path)
		if err != nil {
			return result, fmt.Errorf("workflow: read rendered page %d: %w", page.Number, err)
		}
		if action == entity.PageActionNeedsReview {
			if err := writeReviewPage(s.config.OutputDir, noteID, page.Number, pagePNG, nil); err != nil {
				return result, err
			}
			result.NeedsReview++
			continue
		}

		s.Logger.Info("transcribing page", "note", noteID, "page", page.Number)
		transcription, err := s.transcriber.Transcribe(ctx, transcriber.TranscribeInputDTO{ImageData: pagePNG})
		if err != nil {
			if ctx.Err() != nil {
				return result, err
			}
			// A failed turn is page-scoped: send the page to review and
			// keep going instead of dropping the rest of the note.
			s.Logger.Warn("transcription failed; page sent to review", "note", noteID, "page", page.Number, "error", err)
			if err := s.markPageNeedsReview(noteID, page.Number, page.Hash, ""); err != nil {
				return result, err
			}
			if err := writeReviewPage(s.config.OutputDir, noteID, page.Number, pagePNG, nil); err != nil {
				return result, err
			}
			result.NeedsReview++
			continue
		}
		if transcription.Kind == "cover" || transcription.Kind == "blank" {
			if err := s.markPageProcessed(noteID, page.Number, page.Hash, "", entity.PageStatusSkipped); err != nil {
				return result, err
			}
			continue
		}
		if transcription.NeedsReview || transcription.Kind == "unknown" {
			if err := s.markPageNeedsReview(noteID, page.Number, page.Hash, transcription.Markdown); err != nil {
				return result, err
			}
			boxes := make([][]int, 0, len(transcription.Uncertainties))
			for _, uncertainty := range transcription.Uncertainties {
				boxes = append(boxes, uncertainty.BBox)
			}
			if err := writeReviewPage(s.config.OutputDir, noteID, page.Number, pagePNG, boxes); err != nil {
				return result, err
			}
			result.NeedsReview++
			continue
		}
		if err := s.markPageProcessed(noteID, page.Number, page.Hash, transcription.Markdown, entity.PageStatusTranscribed); err != nil {
			return result, err
		}
		result.PagesProcessed++
	}

	pendingGeneration, err := s.repo.FindPagesByStatus(noteID, entity.PageStatusTranscribed)
	if err != nil {
		return result, err
	}
	if len(pendingGeneration) == 0 {
		return result, nil
	}
	processed, err := s.repo.FindProcessedPages(noteID)
	if err != nil {
		return result, err
	}
	if err := writeTranscript(s.config.OutputDir, noteID, processed); err != nil {
		return result, err
	}
	material, stored, generated, err := s.material(ctx, noteID, processed, pendingGeneration)
	if err != nil {
		if ctx.Err() != nil {
			return result, err
		}
		// Pages stay transcribed, so the next run retries generation or sync.
		s.Logger.Error("study material unavailable; will retry next run", "note", noteID, "error", err)
		return result, nil
	}
	if generated {
		s.Logger.Info("study material generated", "note", noteID, "cards", len(material.Cards))
	}
	if err := writeMaterial(s.config.OutputDir, noteID, material); err != nil {
		return result, err
	}
	if !stored.IsSynced() {
		summary, err := s.anki.Sync(ctx, anki.SyncInputDTO{NoteID: noteID, Cards: material.Cards})
		if err != nil {
			if ctx.Err() != nil {
				return result, err
			}
			// The material is already persisted. A later one-shot run can retry
			// Anki without spending another model turn.
			s.Logger.Error("Anki sync failed; will retry next run", "note", noteID, "error", err)
			return result, nil
		}
		if err := stored.MarkSynced(); err != nil {
			return result, err
		}
		if err := s.repo.SaveMaterial(stored); err != nil {
			return result, err
		}
		s.Logger.Info("cards synchronized with Anki", "note", noteID, "created", summary.Created, "updated", summary.Updated)
	}
	if err := s.markPagesDone(pendingGeneration); err != nil {
		return result, err
	}
	return result, nil
}
