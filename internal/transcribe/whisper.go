// internal/transcribe/whisper.go
package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// whisperJSONResponse is the go-whisper JSON transcription response shape.
type whisperJSONResponse struct {
	Segments []struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
		Text  string  `json:"text"`
	} `json:"segments"`
	Duration float64 `json:"duration"`
}

// TranscribeAudio uploads audioPath to the go-whisper server and returns
// the transcript in the requested format ("text" or "segments").
// goWhisperURL is the base URL, e.g. "http://localhost:8081".
// model is the ggml model ID, e.g. "ggml-base".
func TranscribeAudio(ctx context.Context, goWhisperURL, model, audioPath, format string) (interface{}, float64, error) {
	f, err := os.Open(audioPath)
	if err != nil {
		return nil, 0, fmt.Errorf("open audio file: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	part, err := mw.CreateFormFile("audio", filepath.Base(audioPath))
	if err != nil {
		return nil, 0, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, 0, fmt.Errorf("copy audio: %w", err)
	}
	if err := mw.WriteField("model", model); err != nil {
		return nil, 0, fmt.Errorf("write model field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, 0, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		goWhisperURL+"/api/whisper/transcribe", &buf)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	if format == "segments" {
		req.Header.Set("Accept", "application/json")
	} else {
		req.Header.Set("Accept", "text/plain")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("whisper request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("go-whisper returned %d: %s", resp.StatusCode, body)
	}

	if format == "segments" {
		var parsed whisperJSONResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, 0, fmt.Errorf("parse whisper JSON: %w", err)
		}
		segments := make([]Segment, len(parsed.Segments))
		for i, s := range parsed.Segments {
			segments[i] = Segment{Start: s.Start, End: s.End, Text: s.Text}
		}
		return segments, parsed.Duration, nil
	}

	// text/plain — body is the raw transcript
	return string(body), 0, nil
}
