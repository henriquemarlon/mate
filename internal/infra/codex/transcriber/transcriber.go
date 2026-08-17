package transcriber

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/henriquemarlon/mate/assets"
	"github.com/henriquemarlon/mate/internal/infra/codex"
)

type Uncertainty struct {
	Label string `json:"label"`
	Text  string `json:"text"`
	BBox  []int  `json:"bbox"`
}

type Result struct {
	Kind          string        `json:"kind"`
	Markdown      string        `json:"markdown"`
	NeedsReview   bool          `json:"needs_review"`
	Uncertainties []Uncertainty `json:"uncertainties"`
}

type Transcriber struct {
	client *codex.Client
}

func New(client *codex.Client) *Transcriber {
	return &Transcriber{client: client}
}

func (t *Transcriber) Transcribe(ctx context.Context, imageData []byte) (Result, error) {
	result, err := t.client.Execute(ctx, codex.Request{
		Prompt:    transcriptionPrompt,
		ImageData: imageData,
		MediaType: "image/png",
		Schema:    assets.TranscriptionSchemaJSON,
	})
	if err != nil {
		return Result{}, fmt.Errorf("transcription: transcribe: %w", err)
	}
	var response Result
	if err := json.Unmarshal(result, &response); err != nil {
		return Result{}, fmt.Errorf("transcription: parse response: %w", err)
	}
	if response.Kind == "content" && strings.TrimSpace(response.Markdown) == "" {
		return Result{}, fmt.Errorf("transcription: content page has empty transcription")
	}
	return response, nil
}

const transcriptionPrompt = `Classify and faithfully transcribe this handwritten GoodNotes page.
The layout may use Cornell notes: a narrow cue column on the left, main notes on the right, and a summary footer.
Preserve headings, bullets, equations in LaTeX, diagram labels and meaningful spatial relationships.
Never guess illegible content. Mark it as [?], set needs_review=true, and add an uncertainty with a short label and a normalized bounding box [x1,y1,x2,y2] in the 0..1000 coordinate space.
Use kind=cover for title covers, kind=blank for empty pages, kind=content for transcribable notes, and kind=unknown when classification itself is uncertain.
Return only JSON matching the provided schema.`
