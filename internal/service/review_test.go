package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/anki"
	"github.com/henriquemarlon/mate/internal/infra/llm/paradigm"
)

type reviewRepository struct {
	pages    []entity.Page
	material *entity.Material
}

func (r *reviewRepository) CreatePage(page *entity.Page) error {
	r.pages = append(r.pages, *page)
	return nil
}

func (r *reviewRepository) FindPage(noteID string, pageNumber int) (entity.Page, error) {
	for _, page := range r.pages {
		if page.NoteID == noteID && page.PageNumber == pageNumber {
			return page, nil
		}
	}
	return entity.Page{}, entity.ErrPageNotFound
}

func (r *reviewRepository) UpdatePage(page *entity.Page) error {
	for index := range r.pages {
		if r.pages[index].NoteID == page.NoteID && r.pages[index].PageNumber == page.PageNumber {
			r.pages[index] = *page
			return nil
		}
	}
	return entity.ErrPageNotFound
}

func (r *reviewRepository) UpdatePages(pages []entity.Page) error {
	for index := range pages {
		if err := r.UpdatePage(&pages[index]); err != nil {
			return err
		}
	}
	return nil
}

func (r *reviewRepository) FindProcessedPages(noteID string) ([]entity.Page, error) {
	var result []entity.Page
	for _, page := range r.pages {
		if page.NoteID == noteID && page.ProcessedHash != "" {
			result = append(result, page)
		}
	}
	slices.SortFunc(result, func(left, right entity.Page) int { return left.PageNumber - right.PageNumber })
	return result, nil
}

func (r *reviewRepository) FindPagesByStatus(noteID string, status entity.PageStatus) ([]entity.Page, error) {
	var result []entity.Page
	for _, page := range r.pages {
		if page.NoteID == noteID && page.Status == status {
			result = append(result, page)
		}
	}
	slices.SortFunc(result, func(left, right entity.Page) int { return left.PageNumber - right.PageNumber })
	return result, nil
}

func (r *reviewRepository) FindAllPagesByStatus(status entity.PageStatus) ([]entity.Page, error) {
	var result []entity.Page
	for _, page := range r.pages {
		if page.Status == status {
			result = append(result, page)
		}
	}
	slices.SortFunc(result, func(left, right entity.Page) int {
		if left.NoteID != right.NoteID {
			if left.NoteID < right.NoteID {
				return -1
			}
			return 1
		}
		return left.PageNumber - right.PageNumber
	})
	return result, nil
}

func (r *reviewRepository) SaveMaterial(material *entity.Material) error {
	copy := *material
	r.material = &copy
	return nil
}

func (r *reviewRepository) FindMaterial(string) (entity.Material, error) {
	if r.material == nil {
		return entity.Material{}, entity.ErrMaterialNotFound
	}
	return *r.material, nil
}

type reviewAnki struct{ calls int }

func (a *reviewAnki) Sync(context.Context, anki.SyncInputDTO) (anki.SyncOutputDTO, error) {
	a.calls++
	return anki.SyncOutputDTO{Created: 1}, nil
}

func TestCorrectReviewCompletesMaterialAndRemovesTheImage(t *testing.T) {
	root := t.TempDir()
	repo := &reviewRepository{pages: []entity.Page{{
		NoteID:        "Distributed Systems.pdf",
		PageNumber:    3,
		ObservedHash:  "observed",
		Transcription: "Relógio [?]",
		Status:        entity.PageStatusNeedsReview,
	}}}
	model := &fakeModel{responses: []string{`{
		"feynman": [{"title": "Relógios", "pages": [3], "content": "Explique relógios."}],
		"cards": [{"type": "basic", "front": "O que é relógio lógico?", "back": "Uma ordem lógica.", "tags": []}]
	}`}}
	ankiClient := &reviewAnki{}
	mate := &Service{
		config:   configs.MateConfig{OutputDir: root},
		repo:     repo,
		paradigm: paradigm.New(model),
		anki:     ankiClient,
	}
	mate.Logger = slog.New(slog.DiscardHandler)
	reviewPath, err := reviewPagePath(root, "Distributed Systems.pdf", 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(reviewPath, []byte("image")); err != nil {
		t.Fatal(err)
	}

	item, err := mate.CorrectReview(context.Background(), "Distributed Systems.pdf", 3, "Relógio lógico")
	if err != nil {
		t.Fatalf("expected the review to be resolved: %v", err)
	}
	if item.Status != entity.PageStatusDone {
		t.Fatalf("expected the page to be done, got %s", item.Status)
	}
	page, _ := repo.FindPage("Distributed Systems.pdf", 3)
	if page.Transcription != "Relógio lógico" || page.ProcessedHash != page.ObservedHash {
		t.Fatalf("unexpected resolved page: %+v", page)
	}
	if ankiClient.calls != 1 {
		t.Fatalf("expected one Anki synchronization, got %d", ankiClient.calls)
	}
	if _, err := os.Stat(reviewPath); !os.IsNotExist(err) {
		t.Fatalf("expected the review image to be removed, got %v", err)
	}
	for _, relative := range []string{
		"Distributed Systems/transcript.md",
		"Distributed Systems/cards.json",
		"Distributed Systems/feynman/003-003-relogios.md",
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Fatalf("expected %s to be written: %v", relative, err)
		}
	}
}

func TestCorrectReviewRejectsRemainingUncertainty(t *testing.T) {
	mate := &Service{}
	if _, err := mate.CorrectReview(context.Background(), "note.pdf", 1, "Ainda [?]"); !errors.Is(err, ErrUnresolvedReview) {
		t.Fatalf("expected ErrUnresolvedReview, got %v", err)
	}
}
