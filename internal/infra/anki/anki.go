package anki

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/pkg/ankiconnect"
)

const (
	basicNoteType    = "Mate Basic"
	reversedNoteType = "Mate Reversed"
	clozeNoteType    = "Mate Cloze"
)

// clozePattern matches the minimal cloze deletion syntax Anki requires,
// {{c<number>::, so malformed cloze cards fail here with a clear error
// instead of failing opaquely at addNote.
var clozePattern = regexp.MustCompile(`\{\{c\d+::`)

type SyncInputDTO struct {
	NoteID string
	Cards  []entity.Card
}

type SyncOutputDTO struct {
	Created int
	Updated int
}

type Client struct {
	conn *ankiconnect.Client
	deck string
}

func New(endpoint, deck string) (*Client, error) {
	if strings.TrimSpace(deck) == "" {
		return nil, errors.New("anki: deck cannot be empty")
	}
	conn, err := ankiconnect.New(endpoint)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn: conn,
		deck: strings.TrimSpace(deck),
	}, nil
}

func (c *Client) Sync(ctx context.Context, input SyncInputDTO) (SyncOutputDTO, error) {
	var output SyncOutputDTO

	// Each PDF maps to a child deck below the configured root, mirroring its
	// relative path. "::" is the Anki deck separator, so path segments that
	// contain it are defused.
	relative := strings.TrimSuffix(filepath.ToSlash(input.NoteID), filepath.Ext(input.NoteID))
	parts := strings.FieldsFunc(relative, func(char rune) bool { return char == '/' || char == '\\' })
	for index := range parts {
		parts[index] = strings.TrimSpace(strings.ReplaceAll(parts[index], "::", "-"))
	}
	deck := c.deck
	if len(parts) > 0 {
		deck = c.deck + "::" + strings.Join(parts, "::")
	}

	prepared := make([]ankiconnect.Note, 0, len(input.Cards))
	for _, card := range input.Cards {
		card.Type = entity.CardType(strings.ToLower(strings.TrimSpace(string(card.Type))))
		card.Front = strings.TrimSpace(card.Front)
		card.Back = strings.TrimSpace(card.Back)
		if card.Front == "" || card.Back == "" {
			return output, errors.New("anki: card front and back cannot be empty")
		}

		// The MateID field is the stable identity that lets Sync update an
		// existing note instead of duplicating it. \x00 separates the parts
		// so distinct inputs can never concatenate into the same digest.
		identity := sha256.Sum256([]byte(input.NoteID + "\x00" + string(card.Type) + "\x00" + card.Front))
		mateID := "mate-" + hex.EncodeToString(identity[:])

		// Anki splits tags on whitespace, so inner spaces become underscores.
		// The short note hash tags every card with its source PDF.
		noteHash := sha256.Sum256([]byte(input.NoteID))
		tags := append(card.Tags, "mate", "mate_note_"+hex.EncodeToString(noteHash[:6]))
		seen := make(map[string]struct{}, len(tags))
		normalized := make([]string, 0, len(tags))
		for _, tag := range tags {
			tag = strings.Join(strings.Fields(strings.TrimSpace(tag)), "_")
			if tag == "" {
				continue
			}
			if _, exists := seen[tag]; exists {
				continue
			}
			seen[tag] = struct{}{}
			normalized = append(normalized, tag)
		}
		slices.Sort(normalized)

		item := ankiconnect.Note{
			DeckName: deck,
			Tags:     normalized,
			Options:  ankiconnect.NoteOptions{AllowDuplicate: true},
		}
		switch card.Type {
		case entity.CardTypeBasic:
			item.ModelName = basicNoteType
			item.Fields = map[string]string{"MateID": mateID, "Front": card.Front, "Back": card.Back}
		case entity.CardTypeReversed:
			item.ModelName = reversedNoteType
			item.Fields = map[string]string{"MateID": mateID, "Front": card.Front, "Back": card.Back}
		case entity.CardTypeCloze:
			if !clozePattern.MatchString(card.Front) {
				return output, fmt.Errorf("anki: cloze card does not contain a cloze deletion: %q", card.Front)
			}
			item.ModelName = clozeNoteType
			item.Fields = map[string]string{"MateID": mateID, "Text": card.Front, "Extra": card.Back}
		default:
			return output, fmt.Errorf("anki: unsupported card type %q", card.Type)
		}
		prepared = append(prepared, item)
	}

	version, err := c.conn.Version(ctx)
	if err != nil {
		return output, err
	}
	if version < ankiconnect.APIVersion {
		return output, fmt.Errorf("anki: AnkiConnect API version %d is unsupported; version %d or newer is required", version, ankiconnect.APIVersion)
	}

	if _, err := c.conn.CreateDeck(ctx, deck); err != nil {
		return output, err
	}

	// Mate owns its note types: they carry the MateID field, keep their
	// names stable across localized Anki installs, and are validated so a
	// hand-edited note type fails loudly instead of corrupting future syncs.
	names, err := c.conn.NoteTypeNames(ctx)
	if err != nil {
		return output, err
	}
	noteTypes := []ankiconnect.NoteType{
		{
			Name:   basicNoteType,
			Fields: []string{"Front", "Back", "MateID"},
			Templates: []ankiconnect.CardTemplate{{
				Name: "Forward", Front: "{{Front}}", Back: "{{FrontSide}}<hr id=answer>{{Back}}",
			}},
		},
		{
			Name:   reversedNoteType,
			Fields: []string{"Front", "Back", "MateID"},
			Templates: []ankiconnect.CardTemplate{
				{Name: "Forward", Front: "{{Front}}", Back: "{{FrontSide}}<hr id=answer>{{Back}}"},
				{Name: "Reverse", Front: "{{Back}}", Back: "{{FrontSide}}<hr id=answer>{{Front}}"},
			},
		},
		{
			Name:   clozeNoteType,
			Fields: []string{"Text", "Extra", "MateID"},
			Templates: []ankiconnect.CardTemplate{{
				Name: "Cloze", Front: "{{cloze:Text}}", Back: "{{cloze:Text}}<hr id=answer>{{Extra}}",
			}},
			Cloze: true,
		},
	}
	for _, noteType := range noteTypes {
		if !slices.Contains(names, noteType.Name) {
			if err := c.conn.CreateNoteType(ctx, noteType); err != nil {
				return output, err
			}
			continue
		}
		fields, err := c.conn.NoteTypeFieldNames(ctx, noteType.Name)
		if err != nil {
			return output, err
		}
		if !slices.Equal(fields, noteType.Fields) {
			return output, fmt.Errorf("anki: note type %q has fields %v; expected %v", noteType.Name, fields, noteType.Fields)
		}
	}

	for _, item := range prepared {
		mateID := item.Fields["MateID"]
		noteIDs, err := c.conn.FindNotes(ctx, "MateID:"+mateID)
		if err != nil {
			return output, err
		}
		switch len(noteIDs) {
		case 0:
			if _, err := c.conn.AddNote(ctx, item); err != nil {
				return output, err
			}
			output.Created++
		case 1:
			if err := c.conn.UpdateNoteFields(ctx, noteIDs[0], item.Fields); err != nil {
				return output, err
			}
			if err := c.conn.UpdateNoteTags(ctx, noteIDs[0], item.Tags); err != nil {
				return output, err
			}
			output.Updated++
		default:
			return output, fmt.Errorf("anki: MateID %q matched %d notes", mateID, len(noteIDs))
		}
	}
	return output, nil
}
