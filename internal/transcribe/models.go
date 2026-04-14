// internal/transcribe/models.go
package transcribe

// TranscribeRequest is the public API request body.
type TranscribeRequest struct {
	URL    string `json:"url"`
	Format string `json:"format"` // "text" or "segments"; default "text"
}

// Segment is a timestamped transcript chunk.
type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// TranscribeTextResponse is returned when format="text".
type TranscribeTextResponse struct {
	Transcript      string  `json:"transcript"`
	DurationSeconds float64 `json:"duration_seconds"`
}

// TranscribeSegmentsResponse is returned when format="segments".
type TranscribeSegmentsResponse struct {
	Segments        []Segment `json:"segments"`
	DurationSeconds float64   `json:"duration_seconds"`
}

// ErrorResponse is returned on all error paths.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
