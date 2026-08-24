package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type configTOML map[string]map[string]*Env

func loadConfig(path string) ([]Env, error) {
	var input configTOML
	if _, err := toml.DecodeFile(path, &input); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}

	var topics []string
	for topic := range input {
		topics = append(topics, topic)
	}
	sort.Strings(topics)

	var result []Env
	for _, topic := range topics {
		var names []string
		for name := range input[topic] {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			env := *input[topic][name]
			env.Name = name
			if err := env.validate(); err != nil {
				return nil, err
			}
			result = append(result, env)
		}
	}
	return result, nil
}

func fieldName(name string) string {
	words := strings.Split(strings.TrimPrefix(name, "MATE_"), "_")
	for index, word := range words {
		switch word {
		case "DB", "DPI":
			words[index] = word
		default:
			word = strings.ToLower(word)
			words[index] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, "")
}

func constName(name string) string {
	return strings.TrimPrefix(name, "MATE_")
}
