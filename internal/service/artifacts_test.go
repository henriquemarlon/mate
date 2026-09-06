package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/llm/paradigm"
)

func promptFixture(id, title string, pages []int) entity.FeynmanPrompt {
	return entity.FeynmanPrompt{ID: id, Title: title, Pages: pages, Content: "Explique " + title + "."}
}

func TestWriteMaterialWritesOneFilePerPromptAndAnIndex(t *testing.T) {
	root := t.TempDir()
	material := paradigm.GenerateOutputDTO{
		Feynman: []entity.FeynmanPrompt{
			promptFixture("003-004-modelos-fundamentais", "Modelos fundamentais", []int{3, 4}),
			promptFixture("008-010-relogios-logicos", "Relógios lógicos", []int{8, 9, 10}),
		},
	}

	if err := writeMaterial(root, "Distributed Systems.pdf", material); err != nil {
		t.Fatalf("expected the material to be written: %v", err)
	}

	dir := filepath.Join(root, "Distributed Systems", "feynman")
	for _, prompt := range material.Feynman {
		content, err := os.ReadFile(filepath.Join(dir, prompt.ID+".md"))
		if err != nil {
			t.Fatalf("expected a file for %q: %v", prompt.ID, err)
		}
		if !strings.Contains(string(content), prompt.Content) {
			t.Fatalf("expected %q to carry its prompt, got %q", prompt.ID, content)
		}
	}

	index, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatalf("expected an index: %v", err)
	}
	expected := "# Distributed Systems\n\n" +
		"- [Modelos fundamentais](003-004-modelos-fundamentais.md) — páginas 3–4\n" +
		"- [Relógios lógicos](008-010-relogios-logicos.md) — páginas 8–10\n"
	if string(index) != expected {
		t.Fatalf("unexpected index:\n%s", index)
	}
}

func TestWriteMaterialReplacesTheLegacyScriptOnlyAfterWriting(t *testing.T) {
	root := t.TempDir()
	noteDir := filepath.Join(root, "Distributed Systems")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(noteDir, "feynman.md")
	if err := os.WriteFile(legacy, []byte("roteiro antigo"), 0o644); err != nil {
		t.Fatal(err)
	}

	broken := paradigm.GenerateOutputDTO{Feynman: []entity.FeynmanPrompt{{Title: "Sem ID", Content: "Explique."}}}
	if err := writeMaterial(root, "Distributed Systems.pdf", broken); err == nil {
		t.Fatal("expected an unidentified prompt to be rejected")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("expected the legacy script to survive a failed write: %v", err)
	}

	material := paradigm.GenerateOutputDTO{
		Feynman: []entity.FeynmanPrompt{promptFixture("003-003-modelos", "Modelos", []int{3})},
	}
	if err := writeMaterial(root, "Distributed Systems.pdf", material); err != nil {
		t.Fatalf("expected the material to be written: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("expected the legacy script to be removed, got %v", err)
	}
}

func TestWriteMaterialKeepsLegacyPromptWithoutPages(t *testing.T) {
	root := t.TempDir()
	material := paradigm.GenerateOutputDTO{
		Feynman: []entity.FeynmanPrompt{{ID: "revisao-geral", Title: "Revisão geral", Content: "Explique tudo."}},
	}

	if err := writeMaterial(root, "note.pdf", material); err != nil {
		t.Fatalf("expected the material to be written: %v", err)
	}

	index, err := os.ReadFile(filepath.Join(root, "note", "feynman", "index.md"))
	if err != nil {
		t.Fatalf("expected an index: %v", err)
	}
	if string(index) != "# note\n\n- [Revisão geral](revisao-geral.md)\n" {
		t.Fatalf("expected no page range for a legacy prompt, got:\n%s", index)
	}
}

func TestFormatPagesCollapsesRuns(t *testing.T) {
	cases := []struct {
		pages    []int
		expected string
	}{
		{nil, ""},
		{[]int{5}, "página 5"},
		{[]int{3, 4}, "páginas 3–4"},
		{[]int{8, 9, 10}, "páginas 8–10"},
		{[]int{3, 4, 7}, "páginas 3–4, 7"},
		{[]int{1, 3, 5}, "páginas 1, 3, 5"},
	}
	for _, test := range cases {
		if got := formatPages(test.pages); got != test.expected {
			t.Fatalf("for %v expected %q, got %q", test.pages, test.expected, got)
		}
	}
}
