package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log/slog"
	"strings"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/drive"
	"github.com/henriquemarlon/mate/internal/infra/embeddings"
	"github.com/henriquemarlon/mate/internal/infra/store"
	"github.com/henriquemarlon/mate/internal/infra/vision"
)

// SyncNotes orchestrates: Drive → Vision → Embed → Qdrant (pipeline steps 1-3).
type SyncNotes struct {
	drive      *drive.Client
	vision     *vision.Client
	embeddings *embeddings.Client
	store      *store.Client
	logger     *slog.Logger
}

// NewSyncNotes creates a new SyncNotes usecase.
func NewSyncNotes(
	driveClient *drive.Client,
	visionClient *vision.Client,
	embeddingsClient *embeddings.Client,
	storeClient *store.Client,
	logger *slog.Logger,
) *SyncNotes {
	return &SyncNotes{
		drive:      driveClient,
		vision:     visionClient,
		embeddings: embeddingsClient,
		store:      storeClient,
		logger:     logger,
	}
}

// Execute downloads PDFs from Drive, transcribes each page, generates embeddings,
// and upserts them into Qdrant. Returns all processed pages.
func (s *SyncNotes) Execute(ctx context.Context) ([]entity.Page, error) {
	// Step 1: List PDFs from Drive
	files, err := s.drive.ListPDFs(ctx)
	if err != nil {
		return nil, fmt.Errorf("sync: list PDFs: %w", err)
	}
	s.logger.Info("found PDFs in Drive", "count", len(files))

	var allPages []entity.Page
	var vectorSize uint64

	for _, f := range files {
		s.logger.Info("processing notebook", "name", f.Name, "id", f.ID)

		// Step 2: Download PDF
		var buf bytes.Buffer
		if err := s.drive.Download(ctx, f.ID, &buf); err != nil {
			s.logger.Error("failed to download", "file", f.Name, "error", err)
			continue
		}

		// Step 2b: Split PDF into per-page images
		pages, err := splitPDFToImages(buf.Bytes())
		if err != nil {
			s.logger.Error("failed to split PDF", "file", f.Name, "error", err)
			continue
		}
		s.logger.Info("split PDF into pages", "file", f.Name, "pages", len(pages))

		for pageNum, imgData := range pages {
			pageIdx := pageNum + 1 // 1-based
			s.logger.Info("processing page", "notebook", f.Name, "page", pageIdx)

			// Step 3a: Transcribe via vision model
			transcription, err := s.vision.Transcribe(ctx, imgData, "image/png")
			if err != nil {
				s.logger.Error("transcription failed", "notebook", f.Name, "page", pageIdx, "error", err)
				continue
			}

			if strings.TrimSpace(transcription) == "" {
				s.logger.Warn("empty transcription, skipping", "notebook", f.Name, "page", pageIdx)
				continue
			}

			// Compute content hash
			hash := sha256.Sum256([]byte(transcription))
			contentHash := hex.EncodeToString(hash[:])

			// Step 3b: Generate embedding
			vector, err := s.embeddings.Embed(ctx, transcription)
			if err != nil {
				s.logger.Error("embedding failed", "notebook", f.Name, "page", pageIdx, "error", err)
				continue
			}

			if vectorSize == 0 {
				vectorSize = uint64(len(vector))
				if err := s.store.EnsureCollection(ctx, vectorSize); err != nil {
					return nil, fmt.Errorf("sync: ensure collection: %w", err)
				}
			}

			page := entity.Page{
				NotebookID:    f.ID,
				NotebookName:  strings.TrimSuffix(f.Name, ".pdf"),
				PageNumber:    pageIdx,
				Transcription: transcription,
				ContentHash:   contentHash,
				Vector:        vector,
			}

			// Step 3c: Upsert into Qdrant
			pointID := page.PageID()
			err = s.store.Upsert(ctx, store.PagePoint{
				ID:            pointID,
				NotebookID:    page.NotebookID,
				NotebookName:  page.NotebookName,
				PageNumber:    int64(page.PageNumber),
				Transcription: page.Transcription,
				ContentHash:   page.ContentHash,
				Vector:        vector,
			})
			if err != nil {
				s.logger.Error("upsert failed", "notebook", f.Name, "page", pageIdx, "error", err)
				continue
			}

			allPages = append(allPages, page)
			s.logger.Info("page synced", "notebook", f.Name, "page", pageIdx)
		}
	}

	s.logger.Info("sync complete", "total_pages", len(allPages))
	return allPages, nil
}

// splitPDFToImages splits a PDF into per-page PNG images.
// TODO: This is a placeholder — implement with a real PDF renderer (pdfium, poppler, or pdfcpu).
// For now, if the input is already a single image (PNG/JPEG), return it as a single page.
func splitPDFToImages(data []byte) ([][]byte, error) {
	// Try to decode as a single image first (for testing with image files)
	reader := bytes.NewReader(data)
	_, format, err := image.DecodeConfig(reader)
	if err == nil && (format == "png" || format == "jpeg") {
		return [][]byte{data}, nil
	}

	// For actual PDF splitting, we need a PDF rendering library.
	// This will be implemented with pdfcpu or an external tool like pdftoppm.
	// For now, return the raw bytes as a single "page" — the vision model
	// can handle PDF documents directly.
	return [][]byte{data}, nil
}

// Ensure image codecs are registered for image.DecodeConfig
func init() {
	image.RegisterFormat("png", "\x89PNG", png.Decode, png.DecodeConfig)
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
}
