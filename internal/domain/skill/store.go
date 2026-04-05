package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Store struct {
	skills map[string]*Skill
}

func NewStore(dir string) (*Store, error) {
	s := &Store{skills: make(map[string]*Skill)}

	if dir == "" {
		return nil, fmt.Errorf("skill: skills directory is required")
	}

	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("skill: open skills directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skill: %s is not a directory", dir)
	}

	if err := s.loadDir(dir); err != nil {
		return nil, fmt.Errorf("skill: load from %s: %w", dir, err)
	}

	return s, nil
}

func (s *Store) Get(name string) (*Skill, error) {
	sk, ok := s.skills[name]
	if !ok {
		return nil, fmt.Errorf("skill: %q not found", name)
	}
	return sk, nil
}

func (s *Store) All() []*Skill {
	result := make([]*Skill, 0, len(s.skills))
	for _, sk := range s.skills {
		result = append(result, sk)
	}
	return result
}

func (s *Store) loadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		sk, err := Parse(data)
		if err != nil {
			return fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		s.skills[sk.Name] = sk
	}
	return nil
}
