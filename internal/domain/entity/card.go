package entity

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var ErrInvalidCard = errors.New("invalid card")

// clozeDeletion matches Anki's cloze deletion syntax, {{c<number>::text}}.
// A cloze note without one hides nothing, so Anki rejects it and the whole
// batch fails; this is the check that keeps that from reaching the sync.
var clozeDeletion = regexp.MustCompile(`(?s)\{\{c\d+::.+?\}\}`)

type CardType string

const (
	CardTypeBasic    CardType = "basic"
	CardTypeReversed CardType = "reversed"
	CardTypeCloze    CardType = "cloze"
)

type Card struct {
	Type  CardType `json:"type"`
	Front string   `json:"front"`
	Back  string   `json:"back"`
	Tags  []string `json:"tags"`
}

func NewCard(cardType CardType, front, back string, tags []string) (*Card, error) {
	card := &Card{
		Type:  cardType,
		Front: front,
		Back:  back,
		Tags:  tags,
	}
	if err := card.Validate(); err != nil {
		return nil, err
	}
	return card, nil
}

// Normalize canonicalizes a card and repairs the one mistake models make
// often enough to matter: a cloze whose front carries no deletion is a basic
// card wearing the wrong label. Converting it keeps the rest of the batch
// usable. It reports whether the card's meaning changed, so callers can log
// the downgrade and persist the repair. Trimming alone is not a change.
func (c *Card) Normalize() bool {
	c.Type = CardType(strings.ToLower(strings.TrimSpace(string(c.Type))))
	c.Front = strings.TrimSpace(c.Front)
	c.Back = strings.TrimSpace(c.Back)
	if c.Type == CardTypeCloze && !clozeDeletion.MatchString(c.Front) {
		c.Type = CardTypeBasic
		return true
	}
	return false
}

func (c Card) Validate() error {
	switch c.Type {
	case CardTypeBasic, CardTypeReversed, CardTypeCloze:
	default:
		return fmt.Errorf("%w: unknown type %q", ErrInvalidCard, c.Type)
	}
	if strings.TrimSpace(c.Front) == "" {
		return fmt.Errorf("%w: front cannot be empty", ErrInvalidCard)
	}
	if c.Type != CardTypeCloze && strings.TrimSpace(c.Back) == "" {
		return fmt.Errorf("%w: back cannot be empty", ErrInvalidCard)
	}
	if c.Type == CardTypeCloze && !clozeDeletion.MatchString(c.Front) {
		return fmt.Errorf("%w: cloze front has no {{c1::...}} deletion", ErrInvalidCard)
	}
	return nil
}
