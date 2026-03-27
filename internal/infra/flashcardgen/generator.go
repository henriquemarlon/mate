package flashcardgen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/henriquemarlon/mate/internal/domain/entity"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const systemPrompt = `You are an expert educator creating Anki flashcards from handwritten study notes.
Rules:
- Create clear, specific Q&A pairs that test understanding, not just recall
- Each card should test ONE concept
- Use precise language — avoid vague questions like "What is X?"
- For math/formulas, include the full derivation steps on the back
- Create cards at varying difficulty levels
- Where relevant, reference connections between the primary and related topics
- Output valid JSON only, no markdown fences, no explanation`

const userPromptTemplate = `Here are transcribed handwritten notes from a study cluster.

PRIMARY PAGES (this cluster):
%s

%sGenerate flashcards covering all key concepts from the PRIMARY pages.

Output JSON:
{"topic": "<2-4 word topic label>", "flashcards": [{"front": "...", "back": "...", "tags": ["..."]}]}`

// Generator uses Claude to generate flashcards from clustered page transcriptions.
type Generator struct {
	client anthropic.Client
	model  string
}

// NewGenerator creates a new flashcard generator.
func NewGenerator(apiKey string) *Generator {
	return &Generator{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  string(anthropic.ModelClaudeSonnet4_6),
	}
}

// PageInput holds the data needed for flashcard generation.
type PageInput struct {
	NotebookName string
	PageNumber   int
	Transcription string
}

// Generate produces a topic label and flashcards from a set of cluster pages,
// optionally with bridge pages from neighboring clusters for cross-topic context.
func (g *Generator) Generate(ctx context.Context, pages []PageInput, bridgePages []PageInput) (*entity.ClusterResult, error) {
	primaryText := formatPages(pages)
	bridgeText := ""
	if len(bridgePages) > 0 {
		bridgeText = fmt.Sprintf("RELATED PAGES (neighboring topics — for context only):\n%s\n\n", formatPages(bridgePages))
	}

	prompt := fmt.Sprintf(userPromptTemplate, primaryText, bridgeText)

	resp, err := g.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(g.model),
		MaxTokens: 8192,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(prompt),
			),
		},
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("flashcardgen: API call: %w", err)
	}

	// Extract text from response
	var sb strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}

	return parseResponse(sb.String())
}

func formatPages(pages []PageInput) string {
	var sb strings.Builder
	for _, p := range pages {
		fmt.Fprintf(&sb, "--- %s, Page %d ---\n%s\n\n", p.NotebookName, p.PageNumber, p.Transcription)
	}
	return sb.String()
}

func parseResponse(raw string) (*entity.ClusterResult, error) {
	// Strip markdown code fences if present
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var result entity.ClusterResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("flashcardgen: parse JSON: %w (raw: %.200s)", err, raw)
	}

	if result.Topic == "" {
		return nil, fmt.Errorf("flashcardgen: empty topic in response")
	}

	return &result, nil
}
