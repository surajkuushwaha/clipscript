// internal/transcribe/models.go
package transcribe

// TranscribeRequest is the public API request body.
type TranscribeRequest struct {
	URLs     []string `json:"urls"`
	Format   string   `json:"format"`   // "text" or "segments"; default "text"
	Language string   `json:"language"` // optional BCP-47 code (e.g. "hi", "ur"); empty = translate to English
}

// TranscribeBatchResponse is the successful JSON body for POST /v1/transcribe.
type TranscribeBatchResponse struct {
	Results []TranscribeItemResult `json:"results"`
	Proxy   ProxyInfo              `json:"proxy"`
}

// TranscribeItemResult is one URL's outcome (success or failure).
type TranscribeItemResult struct {
	URL             string    `json:"url"`
	Ok              bool      `json:"ok"`
	Transcript      string    `json:"transcript,omitempty"`
	Segments        []Segment `json:"segments,omitempty"`
	DurationSeconds float64   `json:"duration_seconds,omitempty"`
	Cached          string    `json:"cached,omitempty"` // "none" | "transcript" | "audio"
	Error           string    `json:"error,omitempty"`
	Message         string    `json:"message,omitempty"`
}

// Segment is a timestamped transcript chunk.
type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// TranscribeTextResponse documents a single text result item (subset of batch).
type TranscribeTextResponse struct {
	Transcript      string    `json:"transcript"`
	DurationSeconds float64   `json:"duration_seconds"`
	Proxy           ProxyInfo `json:"proxy"`
	Cached          string    `json:"cached"`
}

// TranscribeSegmentsResponse documents a single segments result item (subset of batch).
type TranscribeSegmentsResponse struct {
	Segments        []Segment `json:"segments"`
	DurationSeconds float64   `json:"duration_seconds"`
	Proxy           ProxyInfo `json:"proxy"`
	Cached          string    `json:"cached"`
}

// ErrorResponse is returned on request-level error paths (not per-URL failures).
type ErrorResponse struct {
	Error   string     `json:"error"`
	Message string     `json:"message"`
	Proxy   *ProxyInfo `json:"proxy,omitempty"`
}
