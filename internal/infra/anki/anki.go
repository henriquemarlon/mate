package anki

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const apiVersion = 6

const (
	basicModel    = "Mate Basic"
	reversedModel = "Mate Reversed"
	clozeModel    = "Mate Cloze"
)

type Card struct {
	Type  string
	Front string
	Back  string
	Tags  []string
}

type Summary struct {
	Created int
	Updated int
}

type Client struct {
	endpoint string
	deck     string
	http     *http.Client
}

type request struct {
	Action  string `json:"action"`
	Version int    `json:"version"`
	Params  any    `json:"params,omitempty"`
}

type response struct {
	Result json.RawMessage `json:"result"`
	Error  *string         `json:"error"`
}

type modelDefinition struct {
	Name      string
	Fields    []string
	Templates []cardTemplate
	Cloze     bool
}

type cardTemplate struct {
	Name  string `json:"Name"`
	Front string `json:"Front"`
	Back  string `json:"Back"`
}

type note struct {
	DeckName  string            `json:"deckName"`
	ModelName string            `json:"modelName"`
	Fields    map[string]string `json:"fields"`
	Tags      []string          `json:"tags"`
	Options   noteOptions       `json:"options"`
}

type noteOptions struct {
	AllowDuplicate bool `json:"allowDuplicate"`
}

type noteUpdate struct {
	ID     int64             `json:"id"`
	Fields map[string]string `json:"fields"`
}

func New(endpoint, deck string) (*Client, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(endpoint))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("anki: invalid endpoint %q", endpoint)
	}
	if strings.TrimSpace(deck) == "" {
		return nil, errors.New("anki: deck cannot be empty")
	}
	return &Client{
		endpoint: parsed.String(),
		deck:     strings.TrimSpace(deck),
		http:     &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (c *Client) Sync(ctx context.Context, noteID string, cards []Card) (Summary, error) {
	var summary Summary
	prepared := make([]note, 0, len(cards))
	for _, card := range cards {
		item, err := prepareNote(c.deckName(noteID), noteID, card)
		if err != nil {
			return summary, err
		}
		prepared = append(prepared, item)
	}

	var version int
	if err := c.invoke(ctx, "version", nil, &version); err != nil {
		return summary, err
	}
	if version < apiVersion {
		return summary, fmt.Errorf("anki: AnkiConnect API version %d is unsupported; version %d or newer is required", version, apiVersion)
	}
	if err := c.ensureDeck(ctx, c.deckName(noteID)); err != nil {
		return summary, err
	}
	if err := c.ensureModels(ctx); err != nil {
		return summary, err
	}

	for _, item := range prepared {
		mateID := item.Fields["MateID"]
		var noteIDs []int64
		if err := c.invoke(ctx, "findNotes", struct {
			Query string `json:"query"`
		}{Query: "MateID:" + mateID}, &noteIDs); err != nil {
			return summary, err
		}
		switch len(noteIDs) {
		case 0:
			var createdID int64
			if err := c.invoke(ctx, "addNote", struct {
				Note note `json:"note"`
			}{Note: item}, &createdID); err != nil {
				return summary, err
			}
			summary.Created++
		case 1:
			if err := c.invoke(ctx, "updateNoteFields", struct {
				Note noteUpdate `json:"note"`
			}{Note: noteUpdate{ID: noteIDs[0], Fields: item.Fields}}, nil); err != nil {
				return summary, err
			}
			if err := c.invoke(ctx, "updateNoteTags", struct {
				Note int64    `json:"note"`
				Tags []string `json:"tags"`
			}{Note: noteIDs[0], Tags: item.Tags}, nil); err != nil {
				return summary, err
			}
			summary.Updated++
		default:
			return summary, fmt.Errorf("anki: MateID %q matched %d notes", mateID, len(noteIDs))
		}
	}
	return summary, nil
}

func (c *Client) ensureDeck(ctx context.Context, deck string) error {
	var deckID int64
	if err := c.invoke(ctx, "createDeck", struct {
		Deck string `json:"deck"`
	}{Deck: deck}, &deckID); err != nil {
		return err
	}
	return nil
}

func (c *Client) ensureModels(ctx context.Context) error {
	var names []string
	if err := c.invoke(ctx, "modelNames", nil, &names); err != nil {
		return err
	}
	for _, definition := range modelDefinitions() {
		if !slices.Contains(names, definition.Name) {
			if err := c.invoke(ctx, "createModel", struct {
				ModelName     string         `json:"modelName"`
				InOrderFields []string       `json:"inOrderFields"`
				CardTemplates []cardTemplate `json:"cardTemplates"`
				IsCloze       bool           `json:"isCloze"`
			}{
				ModelName:     definition.Name,
				InOrderFields: definition.Fields,
				CardTemplates: definition.Templates,
				IsCloze:       definition.Cloze,
			}, nil); err != nil {
				return err
			}
			continue
		}
		var fields []string
		if err := c.invoke(ctx, "modelFieldNames", struct {
			ModelName string `json:"modelName"`
		}{ModelName: definition.Name}, &fields); err != nil {
			return err
		}
		if !slices.Equal(fields, definition.Fields) {
			return fmt.Errorf("anki: model %q has fields %v; expected %v", definition.Name, fields, definition.Fields)
		}
	}
	return nil
}

func (c *Client) invoke(ctx context.Context, action string, params, result any) error {
	payload, err := json.Marshal(request{Action: action, Version: apiVersion, Params: params})
	if err != nil {
		return fmt.Errorf("anki: encode %s request: %w", action, err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("anki: create %s request: %w", action, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpResponse, err := c.http.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("anki: call %s at %s: %w", action, c.endpoint, err)
	}
	defer httpResponse.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("anki: read %s response: %w", action, err)
	}
	if httpResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("anki: %s returned HTTP %d: %s", action, httpResponse.StatusCode, strings.TrimSpace(string(body)))
	}
	var reply response
	if err := json.Unmarshal(body, &reply); err != nil {
		return fmt.Errorf("anki: decode %s response: %w", action, err)
	}
	if reply.Error != nil {
		return fmt.Errorf("anki: %s: %s", action, *reply.Error)
	}
	if result == nil || string(reply.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(reply.Result, result); err != nil {
		return fmt.Errorf("anki: decode %s result: %w", action, err)
	}
	return nil
}

func prepareNote(deck, noteID string, card Card) (note, error) {
	card.Type = strings.ToLower(strings.TrimSpace(card.Type))
	card.Front = strings.TrimSpace(card.Front)
	card.Back = strings.TrimSpace(card.Back)
	if card.Front == "" || card.Back == "" {
		return note{}, errors.New("anki: card front and back cannot be empty")
	}
	mateID := cardID(noteID, card.Type, card.Front)
	item := note{
		DeckName: deck,
		Tags:     normalizeTags(append(card.Tags, "mate", "mate_note_"+shortHash(noteID))),
		Options:  noteOptions{AllowDuplicate: true},
	}
	switch card.Type {
	case "basic":
		item.ModelName = basicModel
		item.Fields = map[string]string{"MateID": mateID, "Front": card.Front, "Back": card.Back}
	case "reversed":
		item.ModelName = reversedModel
		item.Fields = map[string]string{"MateID": mateID, "Front": card.Front, "Back": card.Back}
	case "cloze":
		if !strings.Contains(card.Front, "{{c") {
			return note{}, fmt.Errorf("anki: cloze card does not contain a cloze deletion: %q", card.Front)
		}
		item.ModelName = clozeModel
		item.Fields = map[string]string{"MateID": mateID, "Text": card.Front, "Extra": card.Back}
	default:
		return note{}, fmt.Errorf("anki: unsupported card type %q", card.Type)
	}
	return item, nil
}

func (c *Client) deckName(noteID string) string {
	relative := strings.TrimSuffix(filepath.ToSlash(noteID), filepath.Ext(noteID))
	parts := strings.FieldsFunc(relative, func(char rune) bool { return char == '/' || char == '\\' })
	for index := range parts {
		parts[index] = strings.TrimSpace(strings.ReplaceAll(parts[index], "::", "-"))
	}
	if len(parts) == 0 {
		return c.deck
	}
	return c.deck + "::" + strings.Join(parts, "::")
}

func cardID(noteID, cardType, front string) string {
	hash := sha256.Sum256([]byte(noteID + "\x00" + cardType + "\x00" + front))
	return "mate-" + hex.EncodeToString(hash[:])
}

func shortHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:6])
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.Join(strings.Fields(strings.TrimSpace(tag)), "_")
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	slices.Sort(result)
	return result
}

func modelDefinitions() []modelDefinition {
	return []modelDefinition{
		{
			Name:   basicModel,
			Fields: []string{"Front", "Back", "MateID"},
			Templates: []cardTemplate{{
				Name: "Forward", Front: "{{Front}}", Back: "{{FrontSide}}<hr id=answer>{{Back}}",
			}},
		},
		{
			Name:   reversedModel,
			Fields: []string{"Front", "Back", "MateID"},
			Templates: []cardTemplate{
				{Name: "Forward", Front: "{{Front}}", Back: "{{FrontSide}}<hr id=answer>{{Back}}"},
				{Name: "Reverse", Front: "{{Back}}", Back: "{{FrontSide}}<hr id=answer>{{Front}}"},
			},
		},
		{
			Name:   clozeModel,
			Fields: []string{"Text", "Extra", "MateID"},
			Templates: []cardTemplate{{
				Name: "Cloze", Front: "{{cloze:Text}}", Back: "{{cloze:Text}}<hr id=answer>{{Extra}}",
			}},
			Cloze: true,
		},
	}
}
