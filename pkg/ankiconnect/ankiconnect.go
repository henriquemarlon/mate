// Package ankiconnect is a minimal client for the AnkiConnect add-on
// protocol: one JSON envelope over HTTP POST, correlated by a result/error
// pair, plus typed helpers for the actions and payload shapes the protocol
// defines. It carries no application policy; callers own note type names, note
// content, and identity semantics.
package ankiconnect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIVersion is the AnkiConnect protocol version sent with every request.
const APIVersion = 6

type invokeInputDTO struct {
	Action  string `json:"action"`
	Version int    `json:"version"`
	Params  any    `json:"params,omitempty"`
}

type invokeOutputDTO struct {
	Result json.RawMessage `json:"result"`
	Error  *string         `json:"error"`
}

// Note is the payload accepted by the addNote action.
type Note struct {
	DeckName  string            `json:"deckName"`
	ModelName string            `json:"modelName"`
	Fields    map[string]string `json:"fields"`
	Tags      []string          `json:"tags"`
	Options   NoteOptions       `json:"options"`
}

type NoteOptions struct {
	AllowDuplicate bool `json:"allowDuplicate"`
}

// CardTemplate is one template of a note type. The capitalized JSON keys are
// mandated by the createModel action.
type CardTemplate struct {
	Name  string `json:"Name"`
	Front string `json:"Front"`
	Back  string `json:"Back"`
}

// NoteType describes a note type ("model" in the wire actions).
type NoteType struct {
	Name      string
	Fields    []string
	Templates []CardTemplate
	Cloze     bool
}

type Client struct {
	endpoint string
	http     *http.Client
}

func New(endpoint string) (*Client, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(endpoint))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("ankiconnect: invalid endpoint %q", endpoint)
	}
	return &Client{
		endpoint: parsed.String(),
		http: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		},
	}, nil
}

func (c *Client) Version(ctx context.Context) (int, error) {
	var version int
	if err := c.Invoke(ctx, "version", nil, &version); err != nil {
		return 0, err
	}
	return version, nil
}

func (c *Client) CreateDeck(ctx context.Context, deck string) (int64, error) {
	var deckID int64
	err := c.Invoke(ctx, "createDeck", struct {
		Deck string `json:"deck"`
	}{Deck: deck}, &deckID)
	return deckID, err
}

func (c *Client) NoteTypeNames(ctx context.Context) ([]string, error) {
	var names []string
	if err := c.Invoke(ctx, "modelNames", nil, &names); err != nil {
		return nil, err
	}
	return names, nil
}

func (c *Client) NoteTypeFieldNames(ctx context.Context, name string) ([]string, error) {
	var fields []string
	err := c.Invoke(ctx, "modelFieldNames", struct {
		ModelName string `json:"modelName"`
	}{ModelName: name}, &fields)
	return fields, err
}

func (c *Client) CreateNoteType(ctx context.Context, noteType NoteType) error {
	return c.Invoke(ctx, "createModel", struct {
		ModelName     string         `json:"modelName"`
		InOrderFields []string       `json:"inOrderFields"`
		CardTemplates []CardTemplate `json:"cardTemplates"`
		IsCloze       bool           `json:"isCloze"`
	}{
		ModelName:     noteType.Name,
		InOrderFields: noteType.Fields,
		CardTemplates: noteType.Templates,
		IsCloze:       noteType.Cloze,
	}, nil)
}

// FindNotes returns the IDs of notes matching an Anki browser search query.
func (c *Client) FindNotes(ctx context.Context, query string) ([]int64, error) {
	var noteIDs []int64
	err := c.Invoke(ctx, "findNotes", struct {
		Query string `json:"query"`
	}{Query: query}, &noteIDs)
	return noteIDs, err
}

func (c *Client) AddNote(ctx context.Context, item Note) (int64, error) {
	var createdID int64
	err := c.Invoke(ctx, "addNote", struct {
		Note Note `json:"note"`
	}{Note: item}, &createdID)
	return createdID, err
}

func (c *Client) UpdateNoteFields(ctx context.Context, noteID int64, fields map[string]string) error {
	type noteUpdate struct {
		ID     int64             `json:"id"`
		Fields map[string]string `json:"fields"`
	}
	return c.Invoke(ctx, "updateNoteFields", struct {
		Note noteUpdate `json:"note"`
	}{Note: noteUpdate{ID: noteID, Fields: fields}}, nil)
}

// UpdateNoteTags replaces every tag on the note with the given list.
func (c *Client) UpdateNoteTags(ctx context.Context, noteID int64, tags []string) error {
	return c.Invoke(ctx, "updateNoteTags", struct {
		Note int64    `json:"note"`
		Tags []string `json:"tags"`
	}{Note: noteID, Tags: tags}, nil)
}

// Invoke sends one AnkiConnect action and decodes its result into result.
// A nil result discards the payload. It is the escape hatch for actions
// without a typed helper.
func (c *Client) Invoke(ctx context.Context, action string, params, result any) error {
	payload, err := json.Marshal(invokeInputDTO{Action: action, Version: APIVersion, Params: params})
	if err != nil {
		return fmt.Errorf("ankiconnect: encode %s request: %w", action, err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("ankiconnect: create %s request: %w", action, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpResponse, err := c.http.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("ankiconnect: call %s at %s: %w", action, c.endpoint, err)
	}
	defer httpResponse.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("ankiconnect: read %s response: %w", action, err)
	}
	if httpResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("ankiconnect: %s returned HTTP %d: %s", action, httpResponse.StatusCode, strings.TrimSpace(string(body)))
	}
	var reply invokeOutputDTO
	if err := json.Unmarshal(body, &reply); err != nil {
		return fmt.Errorf("ankiconnect: decode %s response: %w", action, err)
	}
	if reply.Error != nil {
		return fmt.Errorf("ankiconnect: %s: %s", action, *reply.Error)
	}
	if result == nil || string(reply.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(reply.Result, result); err != nil {
		return fmt.Errorf("ankiconnect: decode %s result: %w", action, err)
	}
	return nil
}
