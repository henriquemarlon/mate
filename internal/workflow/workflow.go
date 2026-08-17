package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/artifacts"
	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/codex"
	"github.com/henriquemarlon/mate/internal/infra/codex/paradigm"
	"github.com/henriquemarlon/mate/internal/infra/codex/transcriber"
	"github.com/henriquemarlon/mate/internal/infra/repository"
	"github.com/henriquemarlon/mate/internal/infra/repository/sqlite"
)

type Summary struct {
	NotesProcessed int
	PagesProcessed int
	NeedsReview    int
}

type Runner struct {
	config      configs.MateConfig
	repo        repository.Repository
	transcriber *transcriber.Transcriber
	paradigm    *paradigm.Generator
	logger      *slog.Logger
}

func New(cfg configs.MateConfig, logger *slog.Logger) (*Runner, error) {
	info, err := os.Stat(cfg.StudyDir)
	if err != nil {
		return nil, fmt.Errorf("workflow: study directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workflow: study path is not a directory: %s", cfg.StudyDir)
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("workflow: create output directory: %w", err)
	}
	repo, err := sqlite.NewSQLiteRepository(cfg.StateDB)
	if err != nil {
		return nil, err
	}
	codexClient, err := codex.NewClient(cfg.CodexBin)
	if err != nil {
		repo.Close()
		return nil, err
	}
	return &Runner{
		config:      cfg,
		repo:        repo,
		transcriber: transcriber.New(codexClient),
		paradigm:    paradigm.New(codexClient),
		logger:      logger,
	}, nil
}

func (r *Runner) Close() error {
	return r.repo.Close()
}

func (r *Runner) Run(ctx context.Context) (Summary, error) {
	var summary Summary
	err := filepath.WalkDir(r.config.StudyDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".pdf") {
			return nil
		}
		noteSummary, err := r.processNote(ctx, path)
		if err != nil {
			return err
		}
		if noteSummary.PagesProcessed > 0 || noteSummary.NeedsReview > 0 {
			summary.NotesProcessed++
		}
		summary.PagesProcessed += noteSummary.PagesProcessed
		summary.NeedsReview += noteSummary.NeedsReview
		return nil
	})
	if err != nil {
		return summary, fmt.Errorf("workflow: scan study directory: %w", err)
	}
	r.logger.Info("run complete", "notes", summary.NotesProcessed, "pages", summary.PagesProcessed, "needs_review", summary.NeedsReview)
	return summary, nil
}

func (r *Runner) processNote(ctx context.Context, pdfPath string) (Summary, error) {
	var summary Summary
	relative, err := filepath.Rel(r.config.StudyDir, pdfPath)
	if err != nil {
		return summary, fmt.Errorf("workflow: relative note path: %w", err)
	}
	noteID := filepath.ToSlash(relative)
	r.logger.Info("rendering note", "note", noteID)
	pages, err := renderPDF(ctx, pdfPath, r.config.DPI)
	if err != nil {
		return summary, err
	}

	for _, page := range pages {
		action, err := r.observePage(noteID, page.Number, page.Hash)
		if err != nil {
			return summary, err
		}
		switch action {
		case entity.PageActionIgnore:
			continue
		case entity.PageActionNeedsReview:
			if err := artifacts.WriteReviewPage(r.config.OutputDir, noteID, page.Number, page.PNG, nil); err != nil {
				return summary, err
			}
			summary.NeedsReview++
			continue
		}

		r.logger.Info("transcribing page", "note", noteID, "page", page.Number)
		result, err := r.transcriber.Transcribe(ctx, page.PNG)
		if err != nil {
			return summary, err
		}
		if result.Kind == "cover" || result.Kind == "blank" {
			if err := r.markPageProcessed(noteID, page.Number, page.Hash, "", entity.PageStatusSkipped); err != nil {
				return summary, err
			}
			continue
		}
		if result.NeedsReview || result.Kind == "unknown" {
			if err := r.markPageNeedsReview(noteID, page.Number, page.Hash, result.Markdown); err != nil {
				return summary, err
			}
			boxes := make([][]int, 0, len(result.Uncertainties))
			for _, uncertainty := range result.Uncertainties {
				boxes = append(boxes, uncertainty.BBox)
			}
			if err := artifacts.WriteReviewPage(r.config.OutputDir, noteID, page.Number, page.PNG, boxes); err != nil {
				return summary, err
			}
			summary.NeedsReview++
			continue
		}
		if err := r.markPageProcessed(noteID, page.Number, page.Hash, result.Markdown, entity.PageStatusTranscribed); err != nil {
			return summary, err
		}
		summary.PagesProcessed++
	}

	pendingGeneration, err := r.repo.FindPagesByStatus(noteID, entity.PageStatusTranscribed)
	if err != nil {
		return summary, err
	}
	if len(pendingGeneration) == 0 {
		return summary, nil
	}
	processed, err := r.repo.FindProcessedPages(noteID)
	if err != nil {
		return summary, err
	}
	if err := artifacts.WriteTranscript(r.config.OutputDir, noteID, processed); err != nil {
		return summary, err
	}
	sourcePages := make([]paradigm.SourcePage, 0, len(pendingGeneration))
	for _, page := range pendingGeneration {
		sourcePages = append(sourcePages, paradigm.SourcePage{Number: page.PageNumber, Markdown: page.Transcription})
	}
	material, err := r.paradigm.Generate(ctx, noteID, sourcePages)
	if err != nil {
		return summary, err
	}
	for i := range material.Cards {
		material.Cards[i].Tags = append(material.Cards[i].Tags, "mate")
	}
	if err := artifacts.WriteMaterial(r.config.OutputDir, noteID, material); err != nil {
		return summary, err
	}
	if err := r.markPagesDone(pendingGeneration); err != nil {
		return summary, err
	}
	return summary, nil
}

func (r *Runner) observePage(noteID string, pageNumber int, hash string) (entity.PageAction, error) {
	if strings.TrimSpace(hash) == "" {
		return "", fmt.Errorf("%w: observed hash cannot be empty", entity.ErrInvalidPage)
	}

	page, err := r.repo.FindPage(noteID, pageNumber)
	if errors.Is(err, entity.ErrPageNotFound) {
		page, err := entity.NewPage(noteID, pageNumber, hash, entity.PageStatusPending)
		if err != nil {
			return "", err
		}
		if err := r.repo.CreatePage(page); err != nil {
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
		if err := r.repo.UpdatePage(&page); err != nil {
			return "", err
		}
		return entity.PageActionProcess, nil
	}
	page.Status = entity.PageStatusNeedsReview
	if err := r.repo.UpdatePage(&page); err != nil {
		return "", err
	}
	return entity.PageActionNeedsReview, nil
}

func (r *Runner) markPageProcessed(noteID string, pageNumber int, hash, transcription string, status entity.PageStatus) error {
	if strings.TrimSpace(hash) == "" {
		return fmt.Errorf("%w: processed hash cannot be empty", entity.ErrInvalidPage)
	}
	if status != entity.PageStatusTranscribed && status != entity.PageStatusSkipped {
		return fmt.Errorf("%w: cannot mark page as processed with status %q", entity.ErrInvalidPage, status)
	}

	page, err := r.repo.FindPage(noteID, pageNumber)
	if err != nil {
		return err
	}
	page.ObservedHash = hash
	page.ProcessedHash = hash
	page.Transcription = transcription
	page.Status = status
	return r.repo.UpdatePage(&page)
}

func (r *Runner) markPageNeedsReview(noteID string, pageNumber int, hash, transcription string) error {
	if strings.TrimSpace(hash) == "" {
		return fmt.Errorf("%w: review hash cannot be empty", entity.ErrInvalidPage)
	}

	page, err := r.repo.FindPage(noteID, pageNumber)
	if err != nil {
		return err
	}
	page.ObservedHash = hash
	page.Transcription = transcription
	page.Status = entity.PageStatusNeedsReview
	return r.repo.UpdatePage(&page)
}

func (r *Runner) markPagesDone(pages []entity.Page) error {
	for i := range pages {
		if pages[i].Status != entity.PageStatusTranscribed {
			return fmt.Errorf("%w: only a transcribed page can be completed", entity.ErrInvalidPage)
		}
		pages[i].Status = entity.PageStatusDone
	}
	return r.repo.UpdatePages(pages)
}

type renderedPage struct {
	Number int
	PNG    []byte
	Hash   string
}

func renderPDF(ctx context.Context, pdfPath string, dpi int) ([]renderedPage, error) {
	dir, err := os.MkdirTemp("", "mate-render-")
	if err != nil {
		return nil, fmt.Errorf("workflow: create render directory: %w", err)
	}
	defer os.RemoveAll(dir)

	prefix := filepath.Join(dir, "page")
	output, err := exec.CommandContext(ctx, "pdftoppm", "-png", "-r", strconv.Itoa(dpi), pdfPath, prefix).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("workflow: render PDF: %w: %s", err, strings.TrimSpace(string(output)))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("workflow: list rendered pages: %w", err)
	}
	var pages []renderedPage
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "page-") || !strings.HasSuffix(name, ".png") {
			continue
		}
		number, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "page-"), ".png"))
		if err != nil {
			continue
		}
		png, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("workflow: read rendered page %d: %w", number, err)
		}
		digest := sha256.Sum256(png)
		pages = append(pages, renderedPage{Number: number, PNG: png, Hash: hex.EncodeToString(digest[:])})
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("workflow: no pages rendered from %s", pdfPath)
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Number < pages[j].Number })
	return pages, nil
}
