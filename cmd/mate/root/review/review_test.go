package review

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/henriquemarlon/mate/internal/service"
)

func TestReviewOptionsMatchThePageState(t *testing.T) {
	uncertain := reviewOptions(service.ReviewItem{Transcription: "clock [?]"})
	assertChoices(t, uncertain, []menuChoice{
		choiceRetry, choiceEdit, choiceSkip, choiceOpen, choiceQuit,
	})

	changed := reviewOptions(service.ReviewItem{Changed: true})
	assertChoices(t, changed, []menuChoice{
		choiceRetry, choiceEdit, choiceKeep, choiceOpen, choiceQuit,
	})
}

func TestNumberedMenuRetriesInvalidInput(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("unknown\n2\n"))
	var output bytes.Buffer
	choice, err := chooseNumbered(reader, &output, "Choose:", []menuOption{
		{Label: "First", Value: choiceRetry},
		{Label: "Second", Value: choiceEdit},
	})
	if err != nil {
		t.Fatal(err)
	}
	if choice != choiceEdit {
		t.Fatalf("expected the second option, got %q", choice)
	}
	if !strings.Contains(output.String(), "Choose a number between 1 and 2.") {
		t.Fatalf("expected validation feedback, got %q", output.String())
	}
}

func TestNotebookCanBeSelectedWithoutTypingItsName(t *testing.T) {
	previousNoteID := noteID
	noteID = ""
	t.Cleanup(func() { noteID = previousNoteID })

	reader := bufio.NewReader(strings.NewReader("2\n"))
	var output bytes.Buffer
	items, quit, err := chooseNotebook(reader, &output, -1, false, []service.ReviewItem{
		{NoteID: "Distributed Systems.pdf", PageNumber: 3},
		{NoteID: "Operating Systems.pdf", PageNumber: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	if quit || len(items) != 1 || items[0].NoteID != "Operating Systems.pdf" {
		t.Fatalf("unexpected notebook selection: quit=%t items=%+v", quit, items)
	}
}

func assertChoices(t *testing.T, options []menuOption, expected []menuChoice) {
	t.Helper()
	if len(options) != len(expected) {
		t.Fatalf("expected %d options, got %d", len(expected), len(options))
	}
	for index, choice := range expected {
		if options[index].Value != choice {
			t.Fatalf("option %d: expected %q, got %q", index, choice, options[index].Value)
		}
	}
}
