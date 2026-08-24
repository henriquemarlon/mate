package assets

import _ "embed"

var (
	//go:embed artifacts/transcription.json
	TranscriptionSchemaJSON []byte

	//go:embed artifacts/paradigm.json
	ParadigmSchemaJSON []byte
)
