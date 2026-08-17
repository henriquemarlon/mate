package paradigm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/henriquemarlon/mate/assets"
	"github.com/henriquemarlon/mate/internal/infra/codex"
)

type SourcePage struct {
	Number   int
	Markdown string
}

type Card struct {
	Type  string   `json:"type"`
	Front string   `json:"front"`
	Back  string   `json:"back"`
	Tags  []string `json:"tags"`
}

type Material struct {
	Feynman string `json:"feynman"`
	Cards   []Card `json:"cards"`
}

type Generator struct {
	client *codex.Client
}

func New(client *codex.Client) *Generator {
	return &Generator{client: client}
}

func (g *Generator) Generate(ctx context.Context, noteID string, pages []SourcePage) (Material, error) {
	var source strings.Builder
	fmt.Fprintf(&source, "NOTE: %s\n\n", noteID)
	for _, page := range pages {
		fmt.Fprintf(&source, "--- PAGE %d ---\n%s\n\n", page.Number, page.Markdown)
	}
	result, err := g.client.Execute(ctx, codex.Request{
		Prompt: paradigmPrompt + "\n\n" + source.String(),
		Schema: assets.ParadigmSchemaJSON,
	})
	if err != nil {
		return Material{}, fmt.Errorf("paradigm: generate material: %w", err)
	}
	var material Material
	if err := json.Unmarshal(result, &material); err != nil {
		return Material{}, fmt.Errorf("paradigm: parse material: %w", err)
	}
	return material, nil
}

const paradigmPrompt = `Generate study material only from the supplied transcript, never by inventing missing facts.
Extract atomic concepts. The Cornell cue column may guide concepts, but content without a cue can still be useful.
Do not generate cards from [?], administrative text, meta-notes, covers, blanks or duplicate concepts.
Use Basic for definition/why/how, Cloze for formulas/sequences/exact facts, and Reversed only for terminology that benefits from both directions.
Each card must test one fact, avoid yes/no questions, and have a short answer. Preserve the transcript language and canonical technical terms.
The Feynman script must ask the learner to explain the concepts while the AI acts as a curious layperson. Do not generate exercises.
Return only JSON matching the provided schema.`
