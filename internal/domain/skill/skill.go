package skill

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"
)

type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Model       string `yaml:"model"`
	MaxTokens   int    `yaml:"max_tokens"`
	Prompt      string `yaml:"-"`
}

var separator = []byte("---")

func Parse(data []byte) (*Skill, error) {
	data = bytes.TrimSpace(data)

	if !bytes.HasPrefix(data, separator) {
		return nil, fmt.Errorf("skill: missing opening frontmatter delimiter")
	}

	data = data[len(separator):]

	idx := bytes.Index(data, separator)
	if idx < 0 {
		return nil, fmt.Errorf("skill: missing closing frontmatter delimiter")
	}

	frontmatter := data[:idx]
	body := bytes.TrimSpace(data[idx+len(separator):])

	var s Skill
	if err := yaml.Unmarshal(frontmatter, &s); err != nil {
		return nil, fmt.Errorf("skill: parse frontmatter: %w", err)
	}

	if s.Name == "" {
		return nil, fmt.Errorf("skill: name is required")
	}
	if s.Description == "" {
		return nil, fmt.Errorf("skill: description is required")
	}

	s.Prompt = string(body)

	return &s, nil
}
