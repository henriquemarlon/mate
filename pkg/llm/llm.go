// Package llm executes isolated model requests through the eino framework
// against an OpenAI-compatible chat completion endpoint. Every Execute call
// is one self-contained request: prompt, optional image, and JSON schema in,
// validated JSON out. There is no retained conversation, so a failed call is
// scoped to that call and never poisons subsequent ones.
package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

// DefaultRequestTimeout bounds one full model request, including upload,
// generation, and download of the final message.
const DefaultRequestTimeout = 10 * time.Minute

// DefaultBaseURL is the OpenAI API endpoint used when Config.BaseURL is empty.
const DefaultBaseURL = "https://api.openai.com/v1"

// Request is one isolated unit of model work: a prompt, an optional local
// image, and the JSON schema the final answer must match.
type Request struct {
	Prompt    string
	ImageData []byte
	MediaType string
	// SchemaName labels the schema on the wire; OpenAI requires
	// ^[a-zA-Z0-9_-]+$. Empty means "output".
	SchemaName string
	Schema     []byte
}

// Model is the public contract for executing isolated requests against the
// configured chat model.
type Model interface {
	// Execute runs one chat completion and returns the validated JSON
	// produced by the model.
	Execute(ctx context.Context, request Request) ([]byte, error)
}

// Config describes how to construct a Client.
type Config struct {
	// APIKey authenticates against the endpoint. Required.
	APIKey string
	// Model is the chat model identifier. Required.
	Model string
	// BaseURL is the OpenAI-compatible endpoint. Empty means DefaultBaseURL.
	BaseURL string
	// RequestTimeout is the total cap on one request. Zero means
	// DefaultRequestTimeout.
	RequestTimeout time.Duration
	// Logger receives request lifecycle records. Nil means discard.
	Logger *slog.Logger
}

type Client struct {
	apiKey         string
	model          string
	baseURL        string
	requestTimeout time.Duration
	logger         *slog.Logger
}

var _ Model = (*Client)(nil)

// New validates the configuration and returns a ready Client. No network
// traffic happens until Execute is called.
func New(config Config) (*Client, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, errors.New("llm: API key is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("llm: model is required")
	}
	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	timeout := config.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Client{
		apiKey:         config.APIKey,
		model:          config.Model,
		baseURL:        baseURL,
		requestTimeout: timeout,
		logger:         logger,
	}, nil
}

func (c *Client) Execute(ctx context.Context, request Request) ([]byte, error) {
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, errors.New("llm: prompt is required")
	}
	var outputSchema jsonschema.Schema
	if err := json.Unmarshal(request.Schema, &outputSchema); err != nil {
		return nil, fmt.Errorf("llm: output schema is not a valid JSON schema: %w", err)
	}
	schemaName := strings.TrimSpace(request.SchemaName)
	if schemaName == "" {
		schemaName = "output"
	}

	execCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	// The response format carries the request's own schema, so the chat
	// model is built per call. Construction is local wiring, not I/O.
	chatModel, err := einoopenai.NewChatModel(execCtx, &einoopenai.ChatModelConfig{
		APIKey:  c.apiKey,
		Model:   c.model,
		BaseURL: c.baseURL,
		Timeout: c.requestTimeout,
		ResponseFormat: &einoopenai.ChatCompletionResponseFormat{
			Type: "json_schema",
			JSONSchema: &einoopenai.ChatCompletionResponseFormatJSONSchema{
				Name:       schemaName,
				JSONSchema: &outputSchema,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("llm: build chat model: %w", err)
	}

	message := &schema.Message{Role: schema.User, Content: request.Prompt}
	if len(request.ImageData) > 0 {
		mediaType := strings.ToLower(strings.TrimSpace(request.MediaType))
		if mediaType == "" {
			mediaType = "image/png"
		}
		encoded := base64.StdEncoding.EncodeToString(request.ImageData)
		message = &schema.Message{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{
				{Type: schema.ChatMessagePartTypeText, Text: request.Prompt},
				{
					Type: schema.ChatMessagePartTypeImageURL,
					Image: &schema.MessageInputImage{
						MessagePartCommon: schema.MessagePartCommon{
							Base64Data: &encoded,
							MIMEType:   mediaType,
						},
						// Handwriting transcription needs full resolution.
						Detail: schema.ImageURLDetailHigh,
					},
				},
			},
		}
	}

	started := time.Now()
	c.logger.Debug("llm request started", "model", c.model)
	response, err := chatModel.Generate(execCtx, []*schema.Message{message})
	if err != nil {
		if execCtx.Err() != nil && ctx.Err() == nil {
			return nil, fmt.Errorf("llm: request exceeded %s: %w", c.requestTimeout, execCtx.Err())
		}
		return nil, fmt.Errorf("llm: generate: %w", err)
	}

	output := bytes.TrimSpace([]byte(response.Content))
	if !json.Valid(output) {
		return nil, fmt.Errorf("llm: final response is not valid JSON: %s", truncate(string(output), 4096))
	}
	c.logger.Debug("llm request completed", "model", c.model, "duration", time.Since(started))
	return output, nil
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
