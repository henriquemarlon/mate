package paradigm

type SourcePage struct {
	Number   int
	Markdown string
}

type GenerateInput struct {
	NoteID string
	Pages  []SourcePage
}

type Card struct {
	Type  string   `json:"type"`
	Front string   `json:"front"`
	Back  string   `json:"back"`
	Tags  []string `json:"tags"`
}

type GenerateOutput struct {
	Feynman string `json:"feynman"`
	Cards   []Card `json:"cards"`
}
