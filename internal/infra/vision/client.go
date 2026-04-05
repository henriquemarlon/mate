package vision

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const defaultTranscriptionPrompt = `Transcribe all handwritten text in this image exactly as written.
Preserve structure, formatting, headings, bullet points, and any diagrams described in text.
If you cannot read a word, use [illegible]. Output only the transcribed text, nothing else.`

type Client struct {
	client anthropic.Client
	model  string
	prompt string
}

func NewClient(apiKey, prompt string) *Client {
	if prompt == "" {
		prompt = defaultTranscriptionPrompt
	}
	return &Client{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  string(anthropic.ModelClaudeSonnet4_6),
		prompt: prompt,
	}
}

func (c *Client) Transcribe(ctx context.Context, imageData []byte, mediaType string) (string, error) {
	encoded := base64.StdEncoding.EncodeToString(imageData)

	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewImageBlockBase64(mediaType, encoded),
				anthropic.NewTextBlock(c.prompt),
			),
		},
	})
	if err != nil {
		return "", fmt.Errorf("vision: transcribe: %w", err)
	}

	var sb strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	return sb.String(), nil
}
