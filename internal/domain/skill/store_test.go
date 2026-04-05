package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	data := []byte(`---
name: test-skill
description: A test skill
model: claude-sonnet-4-6
max_tokens: 1024
---

Do something useful.`)

	sk, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sk.Name != "test-skill" {
		t.Errorf("Name = %q, want %q", sk.Name, "test-skill")
	}
	if sk.Description != "A test skill" {
		t.Errorf("Description = %q, want %q", sk.Description, "A test skill")
	}
	if sk.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want %q", sk.Model, "claude-sonnet-4-6")
	}
	if sk.MaxTokens != 1024 {
		t.Errorf("MaxTokens = %d, want %d", sk.MaxTokens, 1024)
	}
	if sk.Prompt != "Do something useful." {
		t.Errorf("Prompt = %q, want %q", sk.Prompt, "Do something useful.")
	}
}

func TestParse_MissingName(t *testing.T) {
	data := []byte(`---
description: no name
---

prompt`)

	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParse_MissingDelimiter(t *testing.T) {
	_, err := Parse([]byte("no frontmatter here"))
	if err == nil {
		t.Fatal("expected error for missing delimiter")
	}
}

func TestNewStore_LoadsFromDir(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "my-skill.md", `---
name: my-skill
description: A skill
model: claude-sonnet-4-6
---

Do the thing.`)

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	sk, err := s.Get("my-skill")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sk.Prompt != "Do the thing." {
		t.Errorf("Prompt = %q, want %q", sk.Prompt, "Do the thing.")
	}
}

func TestNewStore_EmptyDirRequired(t *testing.T) {
	_, err := NewStore("")
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
}

func TestNewStore_NonexistentDir(t *testing.T) {
	_, err := NewStore("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent dir")
	}
}

func TestNewStore_MultipleSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "a.md", `---
name: alpha
description: First
---

Prompt A.`)
	writeSkill(t, dir, "b.md", `---
name: beta
description: Second
---

Prompt B.`)

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	all := s.All()
	if len(all) != 2 {
		t.Errorf("All() returned %d skills, want 2", len(all))
	}

	if _, err := s.Get("alpha"); err != nil {
		t.Errorf("Get alpha: %v", err)
	}
	if _, err := s.Get("beta"); err != nil {
		t.Errorf("Get beta: %v", err)
	}
}

func TestGet_NotFound(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "x.md", `---
name: exists
description: Yes
---

Prompt.`)

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	_, err = s.Get("nope")
	if err == nil {
		t.Fatal("expected error for missing skill")
	}
}

func writeSkill(t *testing.T, dir, filename, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}
}
