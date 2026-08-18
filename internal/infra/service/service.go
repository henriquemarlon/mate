package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/anki"
	"github.com/henriquemarlon/mate/internal/infra/codex/paradigm"
	"github.com/henriquemarlon/mate/internal/infra/codex/transcriber"
	"github.com/henriquemarlon/mate/internal/infra/repository"
	"github.com/henriquemarlon/mate/internal/infra/repository/sqlite"
	"github.com/henriquemarlon/mate/pkg/codex"
)

type Summary struct {
	NotesProcessed int
	NotesFailed    int
	PagesProcessed int
	NeedsReview    int
}

type Service struct {
	config      configs.MateConfig
	repo        repository.Repository
	codex       codex.Codex
	transcriber *transcriber.Transcriber
	paradigm    *paradigm.Generator
	anki        *anki.Client
	logger      *slog.Logger
}

const serviceName = "mate"

func New(ctx context.Context, config configs.MateConfig) (*Service, error) {
	logger := NewServiceLogger(serviceName, config.LogLevel, config.LogColor)
	logger.Info("starting service",
		"study_dir", config.StudyDir,
		"output_dir", config.OutputDir,
		"state_db", config.StateDB,
		"codex_bin", config.CodexBin,
		"anki_endpoint", config.AnkiEndpoint,
		"anki_deck", config.AnkiDeck,
		"dpi", config.DPI,
		"log_level", config.LogLevel.String(),
		"log_color", config.LogColor,
	)
	info, err := os.Stat(config.StudyDir)
	if err != nil {
		return nil, fmt.Errorf("workflow: study directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workflow: study path is not a directory: %s", config.StudyDir)
	}
	if err := os.MkdirAll(config.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("workflow: create output directory: %w", err)
	}
	repo, err := sqlite.NewSQLiteRepository(ctx, config.StateDB)
	if err != nil {
		return nil, err
	}
	codexClient, err := codex.New(ctx, codex.Config{Binary: config.CodexBin, Logger: logger})
	if err != nil {
		repo.Close()
		return nil, fmt.Errorf("workflow: start codex app server (check MATE_CODEX_BIN or --codex-bin): %w", err)
	}
	ankiClient, err := anki.New(config.AnkiEndpoint, config.AnkiDeck)
	if err != nil {
		codexClient.Close()
		repo.Close()
		return nil, err
	}
	return &Service{
		config:      config,
		repo:        repo,
		codex:       codexClient,
		transcriber: transcriber.New(codexClient),
		paradigm:    paradigm.New(codexClient),
		anki:        ankiClient,
		logger:      logger,
	}, nil
}

func (s *Service) Close() error {
	return errors.Join(s.codex.Close(), s.repo.Close())
}

func (s *Service) Run(ctx context.Context) (Summary, error) {
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
			// the run itself is over (cancellation, dead app server).
			if ctx.Err() != nil || errors.Is(err, codex.ErrClosed) || errors.Is(err, codex.ErrStopped) {
				return err
			}
			s.logger.Error("note failed; continuing with next note", "note", path, "error", err)
			result.NotesFailed++
			return nil
		}
		if noteSummary.PagesProcessed > 0 || noteSummary.NeedsReview > 0 {
			result.NotesProcessed++
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("workflow: scan study directory: %w", err)
	}
	s.logger.Info("run complete",
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
	s.logger.Info("rendering note", "note", noteID)
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

		s.logger.Info("transcribing page", "note", noteID, "page", page.Number)
		transcription, err := s.transcriber.Transcribe(ctx, pagePNG)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, codex.ErrClosed) || errors.Is(err, codex.ErrStopped) {
				return result, err
			}
			// A failed turn is page-scoped: send the page to review and
			// keep going instead of dropping the rest of the note.
			s.logger.Warn("transcription failed; page sent to review", "note", noteID, "page", page.Number, "error", err)
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
		if ctx.Err() != nil || errors.Is(err, codex.ErrClosed) || errors.Is(err, codex.ErrStopped) {
			return result, err
		}
		// Pages stay transcribed, so the next run retries generation or sync.
		s.logger.Error("study material unavailable; will retry next run", "note", noteID, "error", err)
		return result, nil
	}
	if generated {
		s.logger.Info("study material generated", "note", noteID, "cards", len(material.Cards))
	}
	if err := writeMaterial(s.config.OutputDir, noteID, material); err != nil {
		return result, err
	}
	if !stored.IsSynced() {
		summary, err := s.anki.Sync(ctx, noteID, ankiCards(material.Cards))
		if err != nil {
			if ctx.Err() != nil {
				return result, err
			}
			// The material is already persisted. A later one-shot run can retry
			// Anki without spending another Codex turn.
			s.logger.Error("Anki sync failed; will retry next run", "note", noteID, "error", err)
			return result, nil
		}
		if err := stored.MarkSynced(); err != nil {
			return result, err
		}
		if err := s.repo.SaveMaterial(stored); err != nil {
			return result, err
		}
		s.logger.Info("cards synchronized with Anki", "note", noteID, "created", summary.Created, "updated", summary.Updated)
	}
	if err := s.markPagesDone(pendingGeneration); err != nil {
		return result, err
	}
	return result, nil
}

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
