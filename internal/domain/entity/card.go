package entity

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidCard = errors.New("invalid card")

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
	if err := card.validate(); err != nil {
		return nil, err
	}
	return card, nil
}

func (c *Card) validate() error {
	switch c.Type {
	case CardTypeBasic, CardTypeReversed, CardTypeCloze:
	default:
		return fmt.Errorf("%w: unknown type %q", ErrInvalidCard, c.Type)
	}
	if strings.TrimSpace(c.Front) == "" {
		return fmt.Errorf("%w: front cannot be empty", ErrInvalidCard)
	}
	if strings.TrimSpace(c.Back) == "" {
		return fmt.Errorf("%w: back cannot be empty", ErrInvalidCard)
	}
	return nil
}
