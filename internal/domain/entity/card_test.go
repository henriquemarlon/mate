package entity

import (
	"errors"
	"testing"
)

func TestCardNormalizeDowngradesClozeWithoutDeletion(t *testing.T) {
	card := Card{Type: " Cloze ", Front: " How is bandwidth defined? ", Back: " Information over time. "}

	if !card.Normalize() {
		t.Fatal("expected a cloze without a deletion to be reported as changed")
	}
	if card.Type != CardTypeBasic {
		t.Fatalf("expected the card to become basic, got %q", card.Type)
	}
	if card.Front != "How is bandwidth defined?" || card.Back != "Information over time." {
		t.Fatalf("expected front and back to be trimmed, got %q / %q", card.Front, card.Back)
	}
	if err := card.Validate(); err != nil {
		t.Fatalf("expected the downgraded card to be valid: %v", err)
	}
}

func TestCardNormalizeKeepsValidCloze(t *testing.T) {
	card := Card{
		Type:  CardTypeCloze,
		Front: "Bandwidth is {{c1::the information transmitted per unit of time}}.",
		Back:  "Expressed in bits per second.",
	}

	if card.Normalize() {
		t.Fatal("expected a valid cloze to be left alone")
	}
	if card.Type != CardTypeCloze {
		t.Fatalf("expected the card to stay cloze, got %q", card.Type)
	}
	if err := card.Validate(); err != nil {
		t.Fatalf("expected a valid cloze to pass validation: %v", err)
	}
}

func TestCardValidateRejectsClozeWithoutDeletion(t *testing.T) {
	cases := map[string]string{
		"no deletion":     "Bandwidth is the information per unit of time.",
		"unclosed":        "Bandwidth is {{c1::information per unit of time.",
		"empty deletion":  "Bandwidth is {{c1::}}.",
		"missing counter": "Bandwidth is {{c::information}}.",
	}
	for name, front := range cases {
		t.Run(name, func(t *testing.T) {
			card := Card{Type: CardTypeCloze, Front: front, Back: "Extra context."}
			if err := card.Validate(); !errors.Is(err, ErrInvalidCard) {
				t.Fatalf("expected ErrInvalidCard, got %v", err)
			}
		})
	}
}

func TestCardValidateAcceptsMultilineAndSecondDeletion(t *testing.T) {
	card := Card{
		Type:  CardTypeCloze,
		Front: "Photosynthesis:\n{{c1::6CO2 + 6H2O}} -> {{c2::C6H12O6 + 6O2}}",
		Back:  "Occurs in the chloroplasts.",
	}

	if err := card.Validate(); err != nil {
		t.Fatalf("expected a multiline cloze to be valid: %v", err)
	}
}

func TestCardValidateAcceptsClozeWithoutExtraContext(t *testing.T) {
	card := Card{
		Type:  CardTypeCloze,
		Front: "Bandwidth is {{c1::information transmitted per unit of time}}.",
		Back:  "",
	}

	if err := card.Validate(); err != nil {
		t.Fatalf("expected a cloze without extra context to be valid: %v", err)
	}
}

func TestCardValidateRequiresBackForBasicAndReversed(t *testing.T) {
	for _, cardType := range []CardType{CardTypeBasic, CardTypeReversed} {
		t.Run(string(cardType), func(t *testing.T) {
			card := Card{Type: cardType, Front: "What is bandwidth?", Back: ""}
			if err := card.Validate(); !errors.Is(err, ErrInvalidCard) {
				t.Fatalf("expected ErrInvalidCard, got %v", err)
			}
		})
	}
}

func TestNewCardRejectsInvalidCloze(t *testing.T) {
	if _, err := NewCard(CardTypeCloze, "No deletion here.", "Back", nil); !errors.Is(err, ErrInvalidCard) {
		t.Fatalf("expected ErrInvalidCard, got %v", err)
	}
}
