package anki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Note represents an Anki note to be added via AnkiConnect.
type Note struct {
	DeckName  string
	ModelName string
	Front     string
	Back      string
	Tags      []string
}

// Client wraps the AnkiConnect API for managing flashcards.
type Client struct {
	url       string
	deckPrefix string
	modelName string
	http      *http.Client
}

// NewClient creates a new AnkiConnect client.
func NewClient(url, deckPrefix, modelName string) *Client {
	return &Client{
		url:       url,
		deckPrefix: deckPrefix,
		modelName: modelName,
		http:      &http.Client{},
	}
}

// DeckName returns the full deck name for a given cluster topic.
func (c *Client) DeckName(topic string) string {
	return c.deckPrefix + "::" + topic
}

// Ping checks if AnkiConnect is reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.invoke(ctx, "version", nil)
	if err != nil {
		return fmt.Errorf("anki: ping: %w", err)
	}
	return nil
}

// EnsureDeck creates the deck if it doesn't exist.
func (c *Client) EnsureDeck(ctx context.Context, deckName string) error {
	_, err := c.invoke(ctx, "createDeck", map[string]any{
		"deck": deckName,
	})
	if err != nil {
		return fmt.Errorf("anki: create deck %q: %w", deckName, err)
	}
	return nil
}

// AddNotes adds multiple notes to Anki. Returns the created note IDs.
func (c *Client) AddNotes(ctx context.Context, notes []Note) ([]int64, error) {
	ankiNotes := make([]map[string]any, len(notes))
	for i, n := range notes {
		ankiNotes[i] = map[string]any{
			"deckName":  n.DeckName,
			"modelName": n.ModelName,
			"fields": map[string]string{
				"Front": n.Front,
				"Back":  n.Back,
			},
			"tags": n.Tags,
			"options": map[string]any{
				"allowDuplicate": false,
			},
		}
	}

	result, err := c.invoke(ctx, "addNotes", map[string]any{
		"notes": ankiNotes,
	})
	if err != nil {
		return nil, fmt.Errorf("anki: add notes: %w", err)
	}

	var ids []int64
	if arr, ok := result.([]any); ok {
		for _, v := range arr {
			switch id := v.(type) {
			case float64:
				ids = append(ids, int64(id))
			case nil:
				ids = append(ids, 0) // failed to add (duplicate)
			}
		}
	}
	return ids, nil
}

// FindNotes finds note IDs matching the given query string.
func (c *Client) FindNotes(ctx context.Context, query string) ([]int64, error) {
	result, err := c.invoke(ctx, "findNotes", map[string]any{
		"query": query,
	})
	if err != nil {
		return nil, fmt.Errorf("anki: find notes: %w", err)
	}

	var ids []int64
	if arr, ok := result.([]any); ok {
		for _, v := range arr {
			if id, ok := v.(float64); ok {
				ids = append(ids, int64(id))
			}
		}
	}
	return ids, nil
}

// DeleteNotes deletes notes by their IDs.
func (c *Client) DeleteNotes(ctx context.Context, noteIDs []int64) error {
	_, err := c.invoke(ctx, "deleteNotes", map[string]any{
		"notes": noteIDs,
	})
	if err != nil {
		return fmt.Errorf("anki: delete notes: %w", err)
	}
	return nil
}

// DeleteDeck deletes a deck and all its cards.
func (c *Client) DeleteDeck(ctx context.Context, deckName string) error {
	_, err := c.invoke(ctx, "deleteDecks", map[string]any{
		"decks":    []string{deckName},
		"cardsToo": true,
	})
	if err != nil {
		return fmt.Errorf("anki: delete deck %q: %w", deckName, err)
	}
	return nil
}

type ankiRequest struct {
	Action  string `json:"action"`
	Version int    `json:"version"`
	Params  any    `json:"params,omitempty"`
}

type ankiResponse struct {
	Result any    `json:"result"`
	Error  *string `json:"error"`
}

func (c *Client) invoke(ctx context.Context, action string, params any) (any, error) {
	reqBody := ankiRequest{
		Action:  action,
		Version: 6,
		Params:  params,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var ankiResp ankiResponse
	if err := json.Unmarshal(body, &ankiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if ankiResp.Error != nil {
		return nil, fmt.Errorf("AnkiConnect: %s", *ankiResp.Error)
	}

	return ankiResp.Result, nil
}
