package entity

type Flashcard struct {
	Front string   `json:"front"`
	Back  string   `json:"back"`
	Tags  []string `json:"tags"`
}

type ClusterResult struct {
	Topic      string      `json:"topic"`
	Flashcards []Flashcard `json:"flashcards"`
}
