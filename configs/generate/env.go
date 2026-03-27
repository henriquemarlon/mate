package main

type Env struct {
	Name string

	Default *string `toml:"default"`

	GoType string `toml:"go-type"`

	Description string `toml:"description"`

	Omit bool `toml:"omit"`

	File bool `toml:"file"`

	UsedBy []string `toml:"used-by"`
}

func (e *Env) validate() {
	if e.GoType == "" {
		panic("missing go-type for " + e.Name)
	}
	if e.Description == "" {
		panic("missing description for " + e.Name)
	}
}
