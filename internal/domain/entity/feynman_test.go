package entity

import (
	"errors"
	"testing"
)

func batch() map[int]string {
	return map[int]string{3: "modelos", 4: "mais modelos", 8: "relogios"}
}

func TestFeynmanPromptIdentifyDerivesReadableID(t *testing.T) {
	prompt := FeynmanPrompt{
		Title:   " Relógios lógicos & vetoriais ",
		Pages:   []int{4, 3, 4},
		Content: " Explique os modelos. ",
	}

	if err := prompt.Identify(batch()); err != nil {
		t.Fatalf("expected the prompt to be accepted: %v", err)
	}
	if prompt.ID != "003-004-relogios-logicos-vetoriais" {
		t.Fatalf("unexpected ID %q", prompt.ID)
	}
	if got := prompt.Pages; len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("expected pages to be sorted and deduplicated, got %v", got)
	}
	if prompt.Title != "Relógios lógicos & vetoriais" || prompt.Content != "Explique os modelos." {
		t.Fatalf("expected title and content to be trimmed, got %q / %q", prompt.Title, prompt.Content)
	}
	if prompt.SourceHash == "" {
		t.Fatal("expected a source hash covering the cited pages")
	}
}

func TestFeynmanPromptSourceHashTracksCitedPagesOnly(t *testing.T) {
	build := func(source map[int]string) FeynmanPrompt {
		prompt := FeynmanPrompt{Title: "Modelos", Pages: []int{3}, Content: "Explique."}
		if err := prompt.Identify(source); err != nil {
			t.Fatalf("expected the prompt to be accepted: %v", err)
		}
		return prompt
	}

	original := build(batch())
	untouched := build(map[int]string{3: "modelos", 4: "outra coisa", 8: "relogios"})
	edited := build(map[int]string{3: "modelos revisados", 4: "mais modelos", 8: "relogios"})

	if original.SourceHash != untouched.SourceHash {
		t.Fatal("expected editing an uncited page to leave the hash alone")
	}
	if original.SourceHash == edited.SourceHash {
		t.Fatal("expected editing a cited page to change the hash")
	}
}

func TestFeynmanPromptIdentifyRejectsInvalidPrompts(t *testing.T) {
	cases := map[string]FeynmanPrompt{
		"page outside the batch": {Title: "Modelos", Pages: []int{99}, Content: "Explique."},
		"no pages":               {Title: "Modelos", Content: "Explique."},
		"no title":               {Title: "  ", Pages: []int{3}, Content: "Explique."},
		"no content":             {Title: "Modelos", Pages: []int{3}, Content: "  "},
	}
	for name, prompt := range cases {
		t.Run(name, func(t *testing.T) {
			if err := prompt.Identify(batch()); !errors.Is(err, ErrInvalidFeynmanPrompt) {
				t.Fatalf("expected ErrInvalidFeynmanPrompt, got %v", err)
			}
		})
	}
}

func TestNewLegacyFeynmanPromptKeepsPreTopicScript(t *testing.T) {
	prompt, err := NewLegacyFeynmanPrompt("# Roteiro\n\nExplique tudo.")
	if err != nil {
		t.Fatalf("expected the legacy script to be wrapped: %v", err)
	}
	if prompt.ID != "revisao-geral" {
		t.Fatalf("unexpected legacy ID %q", prompt.ID)
	}
	if len(prompt.Pages) != 0 || prompt.SourceHash != "" {
		t.Fatal("expected a legacy prompt to claim no pages and no source hash")
	}
	if err := prompt.Validate(); err != nil {
		t.Fatalf("expected the legacy prompt to be valid: %v", err)
	}
	if _, err := NewLegacyFeynmanPrompt("   "); !errors.Is(err, ErrInvalidFeynmanPrompt) {
		t.Fatalf("expected ErrInvalidFeynmanPrompt for an empty script, got %v", err)
	}
}

func TestSlugProducesSafePathSegments(t *testing.T) {
	cases := map[string]string{
		"Relógios lógicos":     "relogios-logicos",
		"Coordenação/Exclusão": "coordenacao-exclusao",
		"../../etc/passwd":     "etc-passwd",
		"  Multi   espaços  ":  "multi-espacos",
		"日本語":                  "session",
		"":                     "session",
	}
	for title, expected := range cases {
		t.Run(title, func(t *testing.T) {
			if got := slug(title); got != expected {
				t.Fatalf("expected %q, got %q", expected, got)
			}
		})
	}
}

func TestSlugStaysShortEnoughForAFileName(t *testing.T) {
	long := "Consistência eventual e replicação em sistemas distribuídos de larga escala com tolerância a falhas"

	got := slug(long)
	if len(got) > slugMaxLength {
		t.Fatalf("expected at most %d characters, got %d: %q", slugMaxLength, len(got), got)
	}
	if got[len(got)-1] == '-' {
		t.Fatalf("expected no trailing separator, got %q", got)
	}
}
