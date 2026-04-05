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

type SyncNotes struct {
	drive      *drive.Client
	vision     *vision.Client
	embeddings *embeddings.Client
	store      *store.Client
	logger     *slog.Logger
}

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

func (s *SyncNotes) Execute(ctx context.Context) ([]entity.Page, error) {
	files, err := s.drive.ListPDFs(ctx)
	if err != nil {
		return nil, fmt.Errorf("sync: list PDFs: %w", err)
	}
	s.logger.Info("found PDFs in Drive", "count", len(files))

	var allPages []entity.Page
	var vectorSize uint64

	for _, f := range files {
		s.logger.Info("processing notebook", "name", f.Name, "id", f.ID)

		var buf bytes.Buffer
		if err := s.drive.Download(ctx, f.ID, &buf); err != nil {
			s.logger.Error("failed to download", "file", f.Name, "error", err)
			continue
		}

		pages, err := splitPDFToImages(buf.Bytes())
		if err != nil {
			s.logger.Error("failed to split PDF", "file", f.Name, "error", err)
			continue
		}
		s.logger.Info("split PDF into pages", "file", f.Name, "pages", len(pages))

		for pageNum, imgData := range pages {
			pageIdx := pageNum + 1
			s.logger.Info("processing page", "notebook", f.Name, "page", pageIdx)

			transcription, err := s.vision.Transcribe(ctx, imgData, "image/png")
			if err != nil {
				s.logger.Error("transcription failed", "notebook", f.Name, "page", pageIdx, "error", err)
				continue
			}

			if strings.TrimSpace(transcription) == "" {
				s.logger.Warn("empty transcription, skipping", "notebook", f.Name, "page", pageIdx)
				continue
			}

			hash := sha256.Sum256([]byte(transcription))
			contentHash := hex.EncodeToString(hash[:])

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

func splitPDFToImages(data []byte) ([][]byte, error) {
	reader := bytes.NewReader(data)
	_, format, err := image.DecodeConfig(reader)
	if err == nil && (format == "png" || format == "jpeg") {
		return [][]byte{data}, nil
	}

	return [][]byte{data}, nil
}

func init() {
	image.RegisterFormat("png", "\x89PNG", png.Decode, png.DecodeConfig)
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
}
