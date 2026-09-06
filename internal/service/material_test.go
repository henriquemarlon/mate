package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/llm/paradigm"
	"github.com/henriquemarlon/mate/pkg/llm"
)

type fakeModel struct {
	responses []string
	calls     int
}

func (f *fakeModel) Execute(context.Context, llm.Request) ([]byte, error) {
	if f.calls >= len(f.responses) {
		return nil, errors.New("unexpected model call")
	}
	response := f.responses[f.calls]
	f.calls++
	return []byte(response), nil
}

// fakeRepository serves the material half of Repository; the page methods are
// unused by these tests and fail loudly if that ever stops being true.
type fakeRepository struct {
	material *entity.Material
	saves    int
}

func (r *fakeRepository) SaveMaterial(material *entity.Material) error {
	stored := *material
	r.material = &stored
	r.saves++
	return nil
}

func (r *fakeRepository) FindMaterial(noteID string) (entity.Material, error) {
	if r.material == nil {
		return entity.Material{}, entity.ErrMaterialNotFound
	}
	return *r.material, nil
}

func (r *fakeRepository) CreatePage(*entity.Page) error   { panic("unused") }
func (r *fakeRepository) UpdatePage(*entity.Page) error   { panic("unused") }
func (r *fakeRepository) UpdatePages([]entity.Page) error { panic("unused") }
func (r *fakeRepository) FindPage(string, int) (entity.Page, error) {
	panic("unused")
}
func (r *fakeRepository) FindProcessedPages(string) ([]entity.Page, error) { panic("unused") }
func (r *fakeRepository) FindPagesByStatus(string, entity.PageStatus) ([]entity.Page, error) {
	panic("unused")
}

func newTestService(model llm.Model) (*Service, *fakeRepository) {
	repo := &fakeRepository{}
	service := &Service{repo: repo, paradigm: paradigm.New(model)}
	service.Logger = slog.New(slog.DiscardHandler)
	return service, repo
}

func transcribedPage(number int, markdown string) entity.Page {
	return entity.Page{
		NoteID:        "note.pdf",
		PageNumber:    number,
		ObservedHash:  markdown,
		ProcessedHash: markdown,
		Transcription: markdown,
		Status:        entity.PageStatusTranscribed,
	}
}

func TestMaterialSplitsBatchIntoTopicPrompts(t *testing.T) {
	model := &fakeModel{responses: []string{`{
		"feynman": [
			{"title": "Modelos fundamentais", "pages": [3, 4], "content": "Explique os modelos."},
			{"title": "Relógios lógicos", "pages": [8], "content": "Explique os relógios."}
		],
		"cards": [{"type": "basic", "front": "O que é um modelo?", "back": "Uma abstração.", "tags": []}]
	}`}}
	service, _ := newTestService(model)
	pages := []entity.Page{
		transcribedPage(3, "modelos"),
		transcribedPage(4, "mais modelos"),
		transcribedPage(8, "relogios"),
	}

	material, _, generated, err := service.material(context.Background(), "note.pdf", pages, pages)
	if err != nil {
		t.Fatalf("expected material to be generated: %v", err)
	}
	if !generated {
		t.Fatal("expected the first run to report generated material")
	}
	if len(material.Feynman) != 2 {
		t.Fatalf("expected two prompts, got %d", len(material.Feynman))
	}
	if material.Feynman[0].ID != "003-004-modelos-fundamentais" {
		t.Fatalf("unexpected first prompt ID %q", material.Feynman[0].ID)
	}
	if material.Feynman[1].ID != "008-008-relogios-logicos" {
		t.Fatalf("unexpected second prompt ID %q", material.Feynman[1].ID)
	}
	if len(material.Cards) != 1 {
		t.Fatalf("expected the cards to survive unchanged, got %d", len(material.Cards))
	}
}

func TestMaterialDropsPromptsCitingPagesOutsideTheBatch(t *testing.T) {
	model := &fakeModel{responses: []string{`{
		"feynman": [
			{"title": "Inventado", "pages": [99], "content": "Explique."},
			{"title": "Modelos fundamentais", "pages": [3], "content": "Explique os modelos."}
		],
		"cards": []
	}`}}
	service, _ := newTestService(model)
	pages := []entity.Page{transcribedPage(3, "modelos")}

	material, _, _, err := service.material(context.Background(), "note.pdf", pages, pages)
	if err != nil {
		t.Fatalf("expected material to be generated: %v", err)
	}
	if len(material.Feynman) != 1 {
		t.Fatalf("expected the invented prompt to be dropped, got %d prompts", len(material.Feynman))
	}
	if material.Feynman[0].Title != "Modelos fundamentais" {
		t.Fatalf("expected the valid prompt to survive, got %q", material.Feynman[0].Title)
	}
}

func TestMaterialIsIdempotentWhenNothingChanged(t *testing.T) {
	model := &fakeModel{responses: []string{`{
		"feynman": [{"title": "Modelos fundamentais", "pages": [3], "content": "Explique."}],
		"cards": []
	}`}}
	service, repo := newTestService(model)
	pages := []entity.Page{transcribedPage(3, "modelos")}

	if _, _, _, err := service.material(context.Background(), "note.pdf", pages, pages); err != nil {
		t.Fatalf("expected the first run to succeed: %v", err)
	}
	material, _, generated, err := service.material(context.Background(), "note.pdf", pages, pages)
	if err != nil {
		t.Fatalf("expected the second run to succeed: %v", err)
	}

	if generated {
		t.Fatal("expected the second run to reuse stored material")
	}
	if model.calls != 1 {
		t.Fatalf("expected exactly one model call across both runs, got %d", model.calls)
	}
	if repo.saves != 1 {
		t.Fatalf("expected exactly one save across both runs, got %d", repo.saves)
	}
	if len(material.Feynman) != 1 {
		t.Fatalf("expected the stored prompt to be returned once, got %d", len(material.Feynman))
	}
}

func TestMaterialAppendsPromptsForNewPagesOnly(t *testing.T) {
	model := &fakeModel{responses: []string{
		`{"feynman": [{"title": "Modelos fundamentais", "pages": [3], "content": "Explique os modelos."}],
		  "cards": [{"type": "basic", "front": "O que é um modelo?", "back": "Uma abstração.", "tags": []}]}`,
		`{"feynman": [{"title": "Relógios lógicos", "pages": [8], "content": "Explique os relógios."}],
		  "cards": [{"type": "basic", "front": "O que é um relógio lógico?", "back": "Uma ordem causal.", "tags": []}]}`,
	}}
	service, _ := newTestService(model)
	first := []entity.Page{transcribedPage(3, "modelos")}
	if _, _, _, err := service.material(context.Background(), "note.pdf", first, first); err != nil {
		t.Fatalf("expected the first run to succeed: %v", err)
	}

	added := transcribedPage(8, "relogios")
	material, _, generated, err := service.material(context.Background(), "note.pdf", append(first, added), []entity.Page{added})
	if err != nil {
		t.Fatalf("expected the second run to succeed: %v", err)
	}

	if !generated {
		t.Fatal("expected new pages to produce new material")
	}
	if len(material.Feynman) != 2 {
		t.Fatalf("expected the earlier prompt to be kept and the new one appended, got %d", len(material.Feynman))
	}
	if material.Feynman[0].ID != "003-003-modelos-fundamentais" || material.Feynman[1].ID != "008-008-relogios-logicos" {
		t.Fatalf("unexpected prompt order: %q, %q", material.Feynman[0].ID, material.Feynman[1].ID)
	}
	if len(material.Cards) != 2 {
		t.Fatalf("expected cards from both runs to merge, got %d", len(material.Cards))
	}
}

func TestMaterialKeepsLegacyMarkdownScript(t *testing.T) {
	service, repo := newTestService(&fakeModel{})
	pages := []entity.Page{transcribedPage(3, "modelos")}
	_, sourceHash, err := materialSource(pages)
	if err != nil {
		t.Fatalf("expected a material source: %v", err)
	}
	repo.material = &entity.Material{
		NoteID:             "note.pdf",
		SourceHash:         sourceHash,
		FeynmanPromptsJSON: "# Roteiro\n\nExplique tudo.",
		CardsJSON:          "[]",
	}

	material, stored, generated, err := service.material(context.Background(), "note.pdf", pages, pages)
	if err != nil {
		t.Fatalf("expected the legacy material to be readable: %v", err)
	}

	if generated {
		t.Fatal("expected legacy material to be reused rather than regenerated")
	}
	if len(material.Feynman) != 1 || material.Feynman[0].ID != "revisao-geral" {
		t.Fatalf("expected one legacy prompt, got %+v", material.Feynman)
	}
	if material.Feynman[0].Content != "# Roteiro\n\nExplique tudo." {
		t.Fatalf("expected the legacy script to be preserved verbatim, got %q", material.Feynman[0].Content)
	}

	var migrated []entity.FeynmanPrompt
	if err := json.Unmarshal([]byte(stored.FeynmanPromptsJSON), &migrated); err != nil {
		t.Fatalf("expected the migration to be staged for the next save: %v", err)
	}
	if len(migrated) != 1 || migrated[0].ID != "revisao-geral" {
		t.Fatalf("expected the staged migration to hold the legacy prompt, got %+v", migrated)
	}
}
