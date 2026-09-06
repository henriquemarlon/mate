package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/llm/paradigm"
)

type sourcePage struct {
	Number        int    `json:"number"`
	ProcessedHash string `json:"processed_hash"`
	Markdown      string `json:"markdown"`
}

func (s *Service) material(ctx context.Context, noteID string, pages, pending []entity.Page) (paradigm.GenerateOutputDTO, *entity.Material, bool, error) {
	source, sourceHash, err := materialSource(pages)
	if err != nil {
		return paradigm.GenerateOutputDTO{}, nil, false, err
	}

	stored, err := s.repo.FindMaterial(noteID)
	if err == nil && stored.SourceHash == sourceHash {
		material, err := s.decodeMaterial(noteID, &stored)
		return material, &stored, false, err
	}
	if err != nil && !errors.Is(err, entity.ErrMaterialNotFound) {
		return paradigm.GenerateOutputDTO{}, nil, false, err
	}

	var previous paradigm.GenerateOutputDTO
	if err == nil {
		previous, err = s.decodeMaterial(noteID, &stored)
		if err != nil {
			return paradigm.GenerateOutputDTO{}, nil, false, err
		}
		source = source[:0]
		for _, page := range pending {
			if strings.TrimSpace(page.Transcription) != "" {
				source = append(source, paradigm.SourcePage{Number: page.PageNumber, Markdown: page.Transcription})
			}
		}
		if len(source) == 0 {
			return paradigm.GenerateOutputDTO{}, nil, false, errors.New("workflow: material source changed without transcribed pages")
		}
	}

	generated, err := s.paradigm.Generate(ctx, paradigm.GenerateInputDTO{NoteID: noteID, Pages: source})
	if err != nil {
		return paradigm.GenerateOutputDTO{}, nil, false, err
	}
	generated.Feynman = s.identifyPrompts(noteID, generated.Feynman, source)
	generated.Cards, _ = s.normalizeCards(noteID, generated.Cards)
	for index := range generated.Cards {
		generated.Cards[index].Tags = append(generated.Cards[index].Tags, "mate")
	}
	material := mergeMaterial(previous, generated)
	promptsJSON, err := json.Marshal(material.Feynman)
	if err != nil {
		return paradigm.GenerateOutputDTO{}, nil, false, fmt.Errorf("workflow: encode feynman prompts: %w", err)
	}
	cardsJSON, err := json.Marshal(material.Cards)
	if err != nil {
		return paradigm.GenerateOutputDTO{}, nil, false, fmt.Errorf("workflow: encode material cards: %w", err)
	}
	created, err := entity.NewMaterial(noteID, sourceHash, string(promptsJSON), string(cardsJSON))
	if err != nil {
		return paradigm.GenerateOutputDTO{}, nil, false, err
	}
	if err := s.repo.SaveMaterial(created); err != nil {
		return paradigm.GenerateOutputDTO{}, nil, false, err
	}
	return material, created, true, nil
}

// mergeMaterial keeps what earlier runs produced and appends only what is
// new. Later runs see just the pages transcribed since, so a session already
// written is never regenerated; dedup by ID guards the case where it is.
func mergeMaterial(previous, generated paradigm.GenerateOutputDTO) paradigm.GenerateOutputDTO {
	if len(previous.Feynman) == 0 && len(previous.Cards) == 0 {
		return sortPrompts(generated)
	}
	result := paradigm.GenerateOutputDTO{
		Feynman: append([]entity.FeynmanPrompt(nil), previous.Feynman...),
		Cards:   append([]entity.Card(nil), previous.Cards...),
	}
	knownPrompts := make(map[string]struct{}, len(result.Feynman))
	for _, prompt := range result.Feynman {
		knownPrompts[prompt.ID] = struct{}{}
	}
	for _, prompt := range generated.Feynman {
		if _, found := knownPrompts[prompt.ID]; found {
			continue
		}
		knownPrompts[prompt.ID] = struct{}{}
		result.Feynman = append(result.Feynman, prompt)
	}
	existing := make(map[string]struct{}, len(result.Cards))
	for _, card := range result.Cards {
		existing[cardKey(card)] = struct{}{}
	}
	for _, card := range generated.Cards {
		if _, found := existing[cardKey(card)]; found {
			continue
		}
		existing[cardKey(card)] = struct{}{}
		result.Cards = append(result.Cards, card)
	}
	return sortPrompts(result)
}

// sortPrompts orders sessions by the pages they cover, which is the order a
// reader expects and keeps both the stored JSON and the index stable.
func sortPrompts(material paradigm.GenerateOutputDTO) paradigm.GenerateOutputDTO {
	slices.SortStableFunc(material.Feynman, func(left, right entity.FeynmanPrompt) int {
		leftPage, rightPage := 0, 0
		if len(left.Pages) > 0 {
			leftPage = left.Pages[0]
		}
		if len(right.Pages) > 0 {
			rightPage = right.Pages[0]
		}
		if leftPage != rightPage {
			return leftPage - rightPage
		}
		return strings.Compare(left.ID, right.ID)
	})
	return material
}

// identifyPrompts derives the identity Mate owns and drops what the model got
// wrong. A single bad session must never cost a note its other sessions, so a
// failure here is a warning and a skip rather than an error.
func (s *Service) identifyPrompts(noteID string, prompts []entity.FeynmanPrompt, source []paradigm.SourcePage) []entity.FeynmanPrompt {
	available := make(map[int]string, len(source))
	for _, page := range source {
		available[page.Number] = page.Markdown
	}
	result := make([]entity.FeynmanPrompt, 0, len(prompts))
	for _, prompt := range prompts {
		if err := prompt.Identify(available); err != nil {
			s.Logger.Warn("feynman prompt discarded", "note", noteID, "title", prompt.Title, "error", err)
			continue
		}
		result = append(result, prompt)
	}
	return result
}

func cardKey(card entity.Card) string {
	return strings.ToLower(strings.TrimSpace(string(card.Type))) + "\x00" + strings.Join(strings.Fields(card.Front), " ")
}

func materialSource(pages []entity.Page) ([]paradigm.SourcePage, string, error) {
	source := make([]sourcePage, 0, len(pages))
	for _, page := range pages {
		if strings.TrimSpace(page.Transcription) == "" {
			continue
		}
		source = append(source, sourcePage{
			Number:        page.PageNumber,
			ProcessedHash: page.ProcessedHash,
			Markdown:      page.Transcription,
		})
	}
	if len(source) == 0 {
		return nil, "", errors.New("workflow: no transcribed pages available for study material")
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return nil, "", fmt.Errorf("workflow: encode material source: %w", err)
	}
	hash := sha256.Sum256(encoded)
	input := make([]paradigm.SourcePage, 0, len(source))
	for _, page := range source {
		input = append(input, paradigm.SourcePage{Number: page.Number, Markdown: page.Markdown})
	}
	return input, hex.EncodeToString(hash[:]), nil
}

// decodeMaterial also repairs what is already stored, so a note held back by
// one malformed card recovers on the next run without spending another model
// turn. The repaired JSON rides along on the caller's next SaveMaterial.
func (s *Service) decodeMaterial(noteID string, stored *entity.Material) (paradigm.GenerateOutputDTO, error) {
	var material paradigm.GenerateOutputDTO
	if err := json.Unmarshal([]byte(stored.CardsJSON), &material.Cards); err != nil {
		return paradigm.GenerateOutputDTO{}, fmt.Errorf("workflow: decode stored material cards: %w", err)
	}
	cards, repaired := s.normalizeCards(noteID, material.Cards)
	material.Cards = cards
	if repaired {
		encoded, err := json.Marshal(cards)
		if err != nil {
			return paradigm.GenerateOutputDTO{}, fmt.Errorf("workflow: encode repaired material cards: %w", err)
		}
		stored.CardsJSON = string(encoded)
	}

	prompts, migrated, err := decodeFeynmanPrompts(stored.FeynmanPromptsJSON)
	if err != nil {
		return paradigm.GenerateOutputDTO{}, err
	}
	material.Feynman = prompts
	if migrated {
		s.Logger.Warn("legacy feynman script wrapped as a single prompt", "note", noteID)
		encoded, err := json.Marshal(prompts)
		if err != nil {
			return paradigm.GenerateOutputDTO{}, fmt.Errorf("workflow: encode migrated feynman prompts: %w", err)
		}
		stored.FeynmanPromptsJSON = string(encoded)
	}
	return material, nil
}

// decodeFeynmanPrompts reads the "feynman" column, which held one Markdown
// script before sessions became a list. Anything that is not the new JSON is
// kept as a single legacy prompt so an existing note stays usable without
// spending another model turn. It reports whether that migration happened so
// the caller can persist it.
func decodeFeynmanPrompts(stored string) ([]entity.FeynmanPrompt, bool, error) {
	trimmed := strings.TrimSpace(stored)
	if trimmed == "" {
		return nil, false, nil
	}
	var prompts []entity.FeynmanPrompt
	if err := json.Unmarshal([]byte(trimmed), &prompts); err == nil {
		return prompts, false, nil
	}
	legacy, err := entity.NewLegacyFeynmanPrompt(trimmed)
	if err != nil {
		return nil, false, fmt.Errorf("workflow: decode stored feynman prompts: %w", err)
	}
	return []entity.FeynmanPrompt{legacy}, true, nil
}

// normalizeCards repairs mislabeled cards and drops the ones that are still
// unusable. A single bad card must never cost a note its other cards, so a
// failure here is a warning and a skip rather than an error.
func (s *Service) normalizeCards(noteID string, cards []entity.Card) ([]entity.Card, bool) {
	result := make([]entity.Card, 0, len(cards))
	repaired := false
	for _, card := range cards {
		if card.Normalize() {
			s.Logger.Warn("cloze card without a deletion converted to basic", "note", noteID, "front", card.Front)
			repaired = true
		}
		if err := card.Validate(); err != nil {
			s.Logger.Warn("card discarded", "note", noteID, "type", card.Type, "front", card.Front, "error", err)
			repaired = true
			continue
		}
		result = append(result, card)
	}
	return result, repaired
}
