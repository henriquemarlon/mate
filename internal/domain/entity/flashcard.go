package entity

// Flashcard represents a single Q&A flashcard generated from a cluster of pages.
type Flashcard struct {
	Front string   `json:"front"`
	Back  string   `json:"back"`
	Tags  []string `json:"tags"`
}

// ClusterResult holds the LLM output for a cluster: topic label + flashcards.
type ClusterResult struct {
	Topic      string      `json:"topic"`
	Flashcards []Flashcard `json:"flashcards"`
}
