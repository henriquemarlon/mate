package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/anki"
	"github.com/henriquemarlon/mate/internal/infra/codex/paradigm"
)

type sourcePage struct {
	Number        int    `json:"number"`
	ProcessedHash string `json:"processed_hash"`
	Markdown      string `json:"markdown"`
}

func (s *Service) material(ctx context.Context, noteID string, pages, pending []entity.Page) (paradigm.GenerateOutput, *entity.Material, bool, error) {
	source, sourceHash, err := materialSource(pages)
	if err != nil {
		return paradigm.GenerateOutput{}, nil, false, err
	}

	stored, err := s.repo.FindMaterial(noteID)
	if err == nil && stored.SourceHash == sourceHash {
		material, err := decodeMaterial(stored)
		return material, &stored, false, err
	}
	if err != nil && !errors.Is(err, entity.ErrMaterialNotFound) {
		return paradigm.GenerateOutput{}, nil, false, err
	}

	var previous paradigm.GenerateOutput
	if err == nil {
		previous, err = decodeMaterial(stored)
		if err != nil {
			return paradigm.GenerateOutput{}, nil, false, err
		}
		source = transcribedSource(pending)
		if len(source) == 0 {
			return paradigm.GenerateOutput{}, nil, false, errors.New("workflow: material source changed without transcribed pages")
		}
	}

	generated, err := s.paradigm.Generate(ctx, paradigm.GenerateInput{NoteID: noteID, Pages: source})
	if err != nil {
		return paradigm.GenerateOutput{}, nil, false, err
	}
	for index := range generated.Cards {
		generated.Cards[index].Tags = append(generated.Cards[index].Tags, "mate")
	}
	material := mergeMaterial(previous, generated)
	cardsJSON, err := json.Marshal(material.Cards)
	if err != nil {
		return paradigm.GenerateOutput{}, nil, false, fmt.Errorf("workflow: encode material cards: %w", err)
	}
	created, err := entity.NewMaterial(noteID, sourceHash, material.Feynman, string(cardsJSON))
	if err != nil {
		return paradigm.GenerateOutput{}, nil, false, err
	}
	if err := s.repo.SaveMaterial(created); err != nil {
		return paradigm.GenerateOutput{}, nil, false, err
	}
	return material, created, true, nil
}

func transcribedSource(pages []entity.Page) []paradigm.SourcePage {
	result := make([]paradigm.SourcePage, 0, len(pages))
	for _, page := range pages {
		if strings.TrimSpace(page.Transcription) != "" {
			result = append(result, paradigm.SourcePage{Number: page.PageNumber, Markdown: page.Transcription})
		}
	}
	return result
}

func mergeMaterial(previous, generated paradigm.GenerateOutput) paradigm.GenerateOutput {
	if strings.TrimSpace(previous.Feynman) == "" {
		return generated
	}
	result := paradigm.GenerateOutput{
		Feynman: strings.TrimSpace(previous.Feynman) + "\n\n---\n\n" + strings.TrimSpace(generated.Feynman),
		Cards:   append([]paradigm.Card(nil), previous.Cards...),
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
	return result
}

func cardKey(card paradigm.Card) string {
	return strings.ToLower(strings.TrimSpace(card.Type)) + "\x00" + strings.Join(strings.Fields(card.Front), " ")
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

func decodeMaterial(stored entity.Material) (paradigm.GenerateOutput, error) {
	material := paradigm.GenerateOutput{Feynman: stored.Feynman}
	if err := json.Unmarshal([]byte(stored.CardsJSON), &material.Cards); err != nil {
		return paradigm.GenerateOutput{}, fmt.Errorf("workflow: decode stored material cards: %w", err)
	}
	return material, nil
}

func ankiCards(cards []paradigm.Card) []anki.Card {
	result := make([]anki.Card, 0, len(cards))
	for _, card := range cards {
		result = append(result, anki.Card{
			Type:  card.Type,
			Front: card.Front,
			Back:  card.Back,
			Tags:  card.Tags,
		})
	}
	return result
}
