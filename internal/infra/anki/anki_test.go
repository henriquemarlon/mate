package anki

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSyncCreatesThenUpdatesCards(t *testing.T) {
	models := make(map[string][]string)
	notes := make(map[string]int64)
	nextNoteID := int64(100)
	created := 0
	updated := 0

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var call struct {
			Action string          `json:"action"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		var result any
		switch call.Action {
		case "version":
			result = apiVersion
		case "createDeck":
			result = int64(1)
		case "modelNames":
			names := make([]string, 0, len(models))
			for name := range models {
				names = append(names, name)
			}
			result = names
		case "createModel":
			var params struct {
				ModelName     string   `json:"modelName"`
				InOrderFields []string `json:"inOrderFields"`
			}
			decodeParams(t, call.Params, &params)
			models[params.ModelName] = params.InOrderFields
			result = nil
		case "modelFieldNames":
			var params struct {
				ModelName string `json:"modelName"`
			}
			decodeParams(t, call.Params, &params)
			result = models[params.ModelName]
		case "findNotes":
			var params struct {
				Query string `json:"query"`
			}
			decodeParams(t, call.Params, &params)
			if id, exists := notes[params.Query[len("MateID:"):]]; exists {
				result = []int64{id}
			} else {
				result = []int64{}
			}
		case "addNote":
			var params struct {
				Note note `json:"note"`
			}
			decodeParams(t, call.Params, &params)
			nextNoteID++
			notes[params.Note.Fields["MateID"]] = nextNoteID
			created++
			result = nextNoteID
		case "updateNoteFields", "updateNoteTags":
			updated++
			result = nil
		default:
			t.Errorf("unexpected action %q", call.Action)
		}
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(struct {
			Result any     `json:"result"`
			Error  *string `json:"error"`
		}{Result: result}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, "Mate")
	if err != nil {
		t.Fatal(err)
	}
	cards := []Card{
		{Type: "basic", Front: "What is a process?", Back: "A running program."},
		{Type: "reversed", Front: "What is latency?", Back: "Transmission delay."},
		{Type: "cloze", Front: "A process is a {{c1::running program}}.", Back: "Process"},
	}

	first, err := client.Sync(context.Background(), "Distributed Systems.pdf", cards)
	if err != nil {
		t.Fatal(err)
	}
	if first.Created != 3 || first.Updated != 0 {
		t.Fatalf("first sync = %+v, want 3 created and 0 updated", first)
	}

	second, err := client.Sync(context.Background(), "Distributed Systems.pdf", cards)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created != 0 || second.Updated != 3 {
		t.Fatalf("second sync = %+v, want 0 created and 3 updated", second)
	}
	if created != 3 || updated != 6 {
		t.Fatalf("calls: created=%d updated=%d, want 3 addNote and 6 update calls", created, updated)
	}
}

func decodeParams(t *testing.T, input json.RawMessage, output any) {
	t.Helper()
	if err := json.Unmarshal(input, output); err != nil {
		t.Fatalf("decode params: %v", err)
	}
}
