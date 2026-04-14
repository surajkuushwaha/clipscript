package transcribe_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"clipscript/internal/transcribe"

	"github.com/gofiber/fiber/v2"
)

// newTestApp wires h into a minimal Fiber app.
func newTestApp(h *transcribe.Handler) *fiber.App {
	app := fiber.New()
	app.Post("/v1/transcribe", h.Transcribe)
	return app
}

// newHandler returns a Handler with no-op fns to avoid real network calls.
func newHandler() *transcribe.Handler {
	h := transcribe.NewHandler()
	// Default stubs — each test overrides what it needs.
	h.SetDownloadFn(func(_ context.Context, _, _ string) (string, error) {
		return "/tmp/fake-audio.mp3", nil
	})
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _ string) (interface{}, float64, error) {
		return "hello world", 12.5, nil
	})
	return h
}

func doRequest(app *fiber.App, body string) (*http.Response, error) {
	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	} else {
		reqBody = strings.NewReader("")
	}
	req, err := http.NewRequest(http.MethodPost, "/v1/transcribe", reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Host = "localhost"
	return app.Test(req)
}

func decodeError(resp *http.Response) transcribe.ErrorResponse {
	var result transcribe.ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return result
}

// ── 1. Invalid JSON ────────────────────────────────────────────────────────────

func TestTranscribe_InvalidJSON(t *testing.T) {
	h := newHandler()
	app := newTestApp(h)

	resp, err := doRequest(app, "not-json")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	result := decodeError(resp)
	if result.Error != "invalid_request" {
		t.Errorf("expected error=invalid_request, got %q", result.Error)
	}
}

// ── 2. Invalid URL ─────────────────────────────────────────────────────────────

func TestTranscribe_InvalidURL(t *testing.T) {
	h := newHandler()
	app := newTestApp(h)

	body := `{"url":"https://www.tiktok.com/@user/video/123","format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	result := decodeError(resp)
	if result.Error != "invalid_url" {
		t.Errorf("expected error=invalid_url, got %q", result.Error)
	}
}

// ── 3. Invalid Format ──────────────────────────────────────────────────────────

func TestTranscribe_InvalidFormat(t *testing.T) {
	h := newHandler()
	app := newTestApp(h)

	body := `{"url":"https://www.youtube.com/shorts/abc123","format":"srt"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	result := decodeError(resp)
	if result.Error != "invalid_format" {
		t.Errorf("expected error=invalid_format, got %q", result.Error)
	}
}

// ── 4. Download Failed ─────────────────────────────────────────────────────────

func TestTranscribe_DownloadFailed(t *testing.T) {
	h := newHandler()
	h.SetDownloadFn(func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("network error")
	})
	app := newTestApp(h)

	body := `{"url":"https://www.youtube.com/shorts/abc123","format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
	result := decodeError(resp)
	if result.Error != "download_failed" {
		t.Errorf("expected error=download_failed, got %q", result.Error)
	}
}

// ── 5. Download Timeout ────────────────────────────────────────────────────────

func TestTranscribe_DownloadTimeout(t *testing.T) {
	h := newHandler()
	h.SetDownloadFn(func(_ context.Context, _, _ string) (string, error) {
		return "", context.DeadlineExceeded
	})
	app := newTestApp(h)

	body := `{"url":"https://www.youtube.com/shorts/abc123","format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusRequestTimeout {
		t.Fatalf("expected 408, got %d", resp.StatusCode)
	}
	result := decodeError(resp)
	if result.Error != "timeout" {
		t.Errorf("expected error=timeout, got %q", result.Error)
	}
}

// ── 6. Transcription Failed ────────────────────────────────────────────────────

func TestTranscribe_TranscriptionFailed(t *testing.T) {
	h := newHandler()
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _ string) (interface{}, float64, error) {
		return nil, 0, errors.New("whisper error")
	})
	app := newTestApp(h)

	body := `{"url":"https://www.youtube.com/shorts/abc123","format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
	result := decodeError(resp)
	if result.Error != "transcription_failed" {
		t.Errorf("expected error=transcription_failed, got %q", result.Error)
	}
}

// ── 7. Text Success ────────────────────────────────────────────────────────────

func TestTranscribe_TextSuccess(t *testing.T) {
	h := newHandler()
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _ string) (interface{}, float64, error) {
		return "hello world", 12.5, nil
	})
	app := newTestApp(h)

	body := `{"url":"https://www.youtube.com/shorts/abc123","format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result transcribe.TranscribeTextResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Transcript != "hello world" {
		t.Errorf("expected transcript=hello world, got %q", result.Transcript)
	}
	if result.DurationSeconds != 12.5 {
		t.Errorf("expected duration_seconds=12.5, got %f", result.DurationSeconds)
	}
}

// ── 8. Segments Success ────────────────────────────────────────────────────────

func TestTranscribe_SegmentsSuccess(t *testing.T) {
	h := newHandler()
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _ string) (interface{}, float64, error) {
		return []transcribe.Segment{{Start: 0.0, End: 3.4, Text: "hello"}}, 3.4, nil
	})
	app := newTestApp(h)

	body := `{"url":"https://www.youtube.com/shorts/abc123","format":"segments"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result transcribe.TranscribeSegmentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(result.Segments))
	}
	seg := result.Segments[0]
	if seg.Start != 0.0 || seg.End != 3.4 || seg.Text != "hello" {
		t.Errorf("segment mismatch: got %+v", seg)
	}
	if result.DurationSeconds != 3.4 {
		t.Errorf("expected duration_seconds=3.4, got %f", result.DurationSeconds)
	}
}

// ── 9. Default Format ──────────────────────────────────────────────────────────

func TestTranscribe_DefaultFormat(t *testing.T) {
	h := newHandler()
	h.SetTranscribeFn(func(_ context.Context, _, _, _, format string) (interface{}, float64, error) {
		if format != "text" {
			return nil, 0, errors.New("unexpected format: " + format)
		}
		return "default text", 5.0, nil
	})
	app := newTestApp(h)

	// No "format" field — handler should default to "text".
	body := `{"url":"https://www.youtube.com/shorts/abc123"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		// Drain body for diagnostics.
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("expected 200, got %d — body: %s", resp.StatusCode, buf.String())
	}

	var result transcribe.TranscribeTextResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Transcript != "default text" {
		t.Errorf("expected transcript=default text, got %q", result.Transcript)
	}
}

// ── 10. Wrong Type From TranscribeFn ──────────────────────────────────────────

func TestTranscribe_WrongTypeFromTranscribeFn(t *testing.T) {
	h := newHandler()
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _ string) (interface{}, float64, error) {
		return 42, 0, nil // wrong type — neither string nor []Segment
	})
	app := newTestApp(h)

	body := `{"url":"https://www.youtube.com/shorts/abc123","format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
	result := decodeError(resp)
	if result.Error != "internal_error" {
		t.Errorf("expected error=internal_error, got %q", result.Error)
	}
}
