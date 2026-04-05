package entity

import "strconv"

type Page struct {
	NotebookID    string
	NotebookName  string
	PageNumber    int
	Transcription string
	ContentHash   string
	Vector        []float32
}

func (p Page) PageID() string {
	return p.NotebookID + ":" + strconv.Itoa(p.PageNumber)
}
