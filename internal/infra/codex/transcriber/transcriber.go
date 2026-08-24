package transcriber

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/henriquemarlon/mate/assets"
	"github.com/henriquemarlon/mate/pkg/codex"
)

type TranscribeInputDTO struct {
	ImageData []byte
}

type Uncertainty struct {
	Label string `json:"label"`
	Text  string `json:"text"`
	BBox  []int  `json:"bbox"`
}

type TranscribeOutputDTO struct {
	Kind          string        `json:"kind"`
	Markdown      string        `json:"markdown"`
	NeedsReview   bool          `json:"needs_review"`
	Uncertainties []Uncertainty `json:"uncertainties"`
}

type Transcriber struct {
	client codex.Codex
}

func New(client codex.Codex) *Transcriber {
	return &Transcriber{client: client}
}

func (t *Transcriber) Transcribe(ctx context.Context, input TranscribeInputDTO) (TranscribeOutputDTO, error) {
	result, err := t.client.Execute(ctx, codex.Request{
		Prompt:    transcriptionPrompt,
		ImageData: input.ImageData,
		MediaType: "image/png",
		Schema:    assets.TranscriptionSchemaJSON,
	})
	if err != nil {
		return TranscribeOutputDTO{}, fmt.Errorf("transcription: transcribe: %w", err)
	}
	var output TranscribeOutputDTO
	if err := json.Unmarshal(result, &output); err != nil {
		return TranscribeOutputDTO{}, fmt.Errorf("transcription: parse response: %w", err)
	}
	if output.Kind == "content" && strings.TrimSpace(output.Markdown) == "" {
		return TranscribeOutputDTO{}, fmt.Errorf("transcription: content page has empty transcription")
	}
	return output, nil
}

const transcriptionPrompt = `Classify and faithfully transcribe this handwritten GoodNotes page.
The layout may use Cornell notes: a narrow cue column on the left, main notes on the right, and a summary footer.
Preserve headings, bullets, equations in LaTeX, diagram labels and meaningful spatial relationships.
Never guess illegible content. Mark it as [?], set needs_review=true, and add an uncertainty with a short label and a normalized bounding box [x1,y1,x2,y2] in the 0..1000 coordinate space.
Use kind=cover for title covers, kind=blank for empty pages, kind=content for transcribable notes, and kind=unknown when classification itself is uncertain.
Return only JSON matching the provided schema.`
