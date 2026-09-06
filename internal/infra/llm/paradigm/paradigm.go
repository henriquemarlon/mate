package paradigm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/henriquemarlon/mate/assets"
	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/pkg/llm"
)

type SourcePage struct {
	Number   int
	Markdown string
}

type GenerateInputDTO struct {
	NoteID string
	Pages  []SourcePage
}

type GenerateOutputDTO struct {
	Feynman string        `json:"feynman"`
	Cards   []entity.Card `json:"cards"`
}

type Generator struct {
	client llm.Model
}

func New(client llm.Model) *Generator {
	return &Generator{client: client}
}

func (g *Generator) Generate(ctx context.Context, input GenerateInputDTO) (GenerateOutputDTO, error) {
	var source strings.Builder
	fmt.Fprintf(&source, "NOTE: %s\n\n", input.NoteID)
	for _, page := range input.Pages {
		fmt.Fprintf(&source, "--- PAGE %d ---\n%s\n\n", page.Number, page.Markdown)
	}
	result, err := g.client.Execute(ctx, llm.Request{
		Prompt:     paradigmPrompt + "\n\n" + source.String(),
		SchemaName: "study_material",
		Schema:     assets.ParadigmSchemaJSON,
	})
	if err != nil {
		return GenerateOutputDTO{}, fmt.Errorf("paradigm: generate material: %w", err)
	}
	var material GenerateOutputDTO
	if err := json.Unmarshal(result, &material); err != nil {
		return GenerateOutputDTO{}, fmt.Errorf("paradigm: parse material: %w", err)
	}
	return material, nil
}

const paradigmPrompt = `Generate study material only from the supplied transcript, never by inventing missing facts.
Extract atomic concepts. The Cornell cue column may guide concepts, but content without a cue can still be useful.
Do not generate cards from [?], administrative text, meta-notes, covers, blanks or duplicate concepts.
Use Basic for definition/why/how, Cloze for formulas/sequences/exact facts, and Reversed only for terminology that benefits from both directions.
A Cloze front must hide the tested span inline with Anki syntax {{c1::hidden text}}, using {{c2::...}} for a second independent deletion. Its back holds only optional extra context, never the answer, and may be empty.
Correct Cloze: front "Bandwidth is {{c1::the amount of information transmitted per unit of time}}." with back "Usually expressed in bits per second."
Wrong Cloze: front "How is bandwidth defined?" with back "Amount of information divided by time." That is a Basic card; label it basic instead of writing a Cloze with no deletion.
Each card must test one fact, avoid yes/no questions, and have a short answer. Preserve the transcript language and canonical technical terms.
The Feynman script must ask the learner to explain the concepts while the AI acts as a curious layperson. Do not generate exercises.
Return only JSON matching the provided schema.`
