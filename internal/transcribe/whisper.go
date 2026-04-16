package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// apiHTTPClient never uses HTTP_PROXY / HTTPS_PROXY. Download proxies apply only to
// yt-dlp in downloader.go; transcription must talk directly to go-whisper and OpenAI.
var apiHTTPClient = func() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = func(*http.Request) (*url.URL, error) {
		return nil, nil // direct connection; do not use environment proxy
	}
	return &http.Client{Transport: t}
}()

// whisperJSONResponse is the go-whisper JSON transcription response shape.
type whisperJSONResponse struct {
	Segments []struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
		Text  string  `json:"text"`
	} `json:"segments"`
	Duration float64 `json:"duration"`
}

// openAIVerboseJSON is OpenAI's verbose_json response shape (transcriptions + translations).
type openAIVerboseJSON struct {
	Text     string  `json:"text"`
	Duration float64 `json:"duration"`
	Segments []struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
		Text  string  `json:"text"`
	} `json:"segments"`
}

// TranscribeAudio uploads audioPath to the transcription backend and returns
// the transcript in the requested format ("text" or "segments").
//
// Routing logic:
//   - openAIKey != "" && language == "" → OpenAI /v1/audio/translations (always English)
//   - openAIKey != "" && language != "" → go-whisper → OpenAI /v1/audio/transcriptions with language hint
//   - openAIKey == "" && language == "" → go-whisper /api/whisper/translate (local model, English)
//   - openAIKey == "" && language != "" → go-whisper /api/whisper/transcribe with language hint
func TranscribeAudio(ctx context.Context, goWhisperURL, model, audioPath, format, language, openAIKey string) (interface{}, float64, error) {
	if openAIKey != "" && language == "" {
		return transcribeOpenAIDirect(ctx, openAIKey, model, audioPath, format)
	}
	return transcribeViaGoWhisper(ctx, goWhisperURL, model, audioPath, format, language)
}

// transcribeOpenAIDirect calls OpenAI's /v1/audio/translations endpoint directly.
// Always returns English output regardless of input language.
func transcribeOpenAIDirect(ctx context.Context, apiKey, model, audioPath, format string) (interface{}, float64, error) {
	f, err := os.Open(audioPath)
	if err != nil {
		return nil, 0, fmt.Errorf("open audio file: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	part, err := mw.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return nil, 0, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, 0, fmt.Errorf("copy audio: %w", err)
	}
	if err := mw.WriteField("model", model); err != nil {
		return nil, 0, fmt.Errorf("write model field: %w", err)
	}

	// Use verbose_json to get segments + duration from OpenAI.
	if err := mw.WriteField("response_format", "verbose_json"); err != nil {
		return nil, 0, fmt.Errorf("write response_format field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, 0, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/audio/translations", &buf)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := apiHTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("openai returned %d: %s", resp.StatusCode, body)
	}

	var parsed openAIVerboseJSON
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, 0, fmt.Errorf("parse openai JSON: %w", err)
	}

	if format == "segments" {
		segments := make([]Segment, len(parsed.Segments))
		for i, s := range parsed.Segments {
			segments[i] = Segment{Start: s.Start, End: s.End, Text: s.Text}
		}
		duration := parsed.Duration
		if duration == 0 && len(parsed.Segments) > 0 {
			duration = parsed.Segments[len(parsed.Segments)-1].End
		}
		return segments, duration, nil
	}

	return parsed.Text, parsed.Duration, nil
}

// transcribeViaGoWhisper sends audio to the go-whisper sidecar.
func transcribeViaGoWhisper(ctx context.Context, goWhisperURL, model, audioPath, format, language string) (interface{}, float64, error) {
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

	endpoint := "/api/whisper/translate"
	if language != "" {
		endpoint = "/api/whisper/transcribe"
		if err := mw.WriteField("language", language); err != nil {
			return nil, 0, fmt.Errorf("write language field: %w", err)
		}
	}

	if err := mw.Close(); err != nil {
		return nil, 0, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, goWhisperURL+endpoint, &buf)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	req.Header.Set("Accept", "application/json")

	resp, err := apiHTTPClient.Do(req)
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

	var parsed whisperJSONResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, 0, fmt.Errorf("parse whisper JSON: %w", err)
	}
	duration := parsed.Duration
	if duration == 0 && len(parsed.Segments) > 0 {
		duration = parsed.Segments[len(parsed.Segments)-1].End
	}

	if format == "segments" {
		segments := make([]Segment, len(parsed.Segments))
		for i, s := range parsed.Segments {
			segments[i] = Segment{Start: s.Start, End: s.End, Text: s.Text}
		}
		return segments, duration, nil
	}

	var sb strings.Builder
	for _, s := range parsed.Segments {
		sb.WriteString(s.Text)
	}
	return sb.String(), duration, nil
}
