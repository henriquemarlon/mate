package main

import "fmt"

type Env struct {
	Name        string
	Default     *string  `toml:"default"`
	GoType      string   `toml:"go-type"`
	Description string   `toml:"description"`
	UsedBy      []string `toml:"used-by"`
}

func (e Env) validate() error {
	if e.Default == nil {
		return fmt.Errorf("%s: missing default", e.Name)
	}
	if e.Description == "" {
		return fmt.Errorf("%s: missing description", e.Name)
	}
	switch e.GoType {
	case "string", "Bool", "LogLevel", "Path", "RenderDPI":
	default:
		return fmt.Errorf("%s: unsupported go-type %q", e.Name, e.GoType)
	}
	return nil
}
