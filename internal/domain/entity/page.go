package entity

import "strconv"

// Page represents a single transcribed page from a Goodnotes notebook.
type Page struct {
	// NotebookID is the Google Drive file ID of the notebook PDF.
	NotebookID string
	// NotebookName is the human-readable name of the notebook.
	NotebookName string
	// PageNumber is the 1-based page index within the notebook.
	PageNumber int
	// Transcription is the vision model's transcription of the handwritten content.
	Transcription string
	// ContentHash is the SHA-256 hex digest of the transcription text.
	ContentHash string
	// Vector is the embedding vector for this page's transcription.
	Vector []float32
}

// PageID returns a unique identifier for this page: "notebookID:pageNumber".
func (p Page) PageID() string {
	return p.NotebookID + ":" + strconv.Itoa(p.PageNumber)
}
