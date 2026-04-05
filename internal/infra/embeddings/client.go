package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const baseURL = "https://generativelanguage.googleapis.com/v1beta"

type Client struct {
	apiKey string
	model  string
	http   *http.Client
}

func NewClient(apiKey, model string) *Client {
	return &Client{
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{},
	}
}

type embedRequest struct {
	Model    string       `json:"model"`
	Content  contentParam `json:"content"`
	TaskType string       `json:"taskType"`
}

type contentParam struct {
	Parts []partParam `json:"parts"`
}

type partParam struct {
	Text string `json:"text"`
}

type embedResponse struct {
	Embedding *embeddingValue `json:"embedding"`
	Error     *apiError       `json:"error"`
}

type embeddingValue struct {
	Values []float32 `json:"values"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	body := embedRequest{
		Model: "models/" + c.model,
		Content: contentParam{
			Parts: []partParam{{Text: text}},
		},
		TaskType: "CLUSTERING",
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("embeddings: marshal: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:embedContent?key=%s", baseURL, c.model, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("embeddings: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embeddings: request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embeddings: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embeddings: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result embedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("embeddings: unmarshal: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("embeddings: API error %d: %s", result.Error.Code, result.Error.Message)
	}

	if result.Embedding == nil || len(result.Embedding.Values) == 0 {
		return nil, fmt.Errorf("embeddings: empty embedding returned")
	}

	return result.Embedding.Values, nil
}
