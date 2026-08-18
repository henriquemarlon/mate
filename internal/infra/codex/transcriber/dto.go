package transcriber

type Uncertainty struct {
	Label string `json:"label"`
	Text  string `json:"text"`
	BBox  []int  `json:"bbox"`
}

type TranscribeOutput struct {
	Kind          string        `json:"kind"`
	Markdown      string        `json:"markdown"`
	NeedsReview   bool          `json:"needs_review"`
	Uncertainties []Uncertainty `json:"uncertainties"`
}
