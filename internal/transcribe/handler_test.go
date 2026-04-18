package transcribe_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
func newHandler(t *testing.T) *transcribe.Handler {
	t.Helper()
	t.Setenv("PROXY_POOL", "")
	t.Setenv("PROXY_POOL_FILE", "")
	t.Setenv("USE_EMBEDDED_PROXY_POOL", "")
	t.Setenv("CACHE_ENABLED", "false")
	h := transcribe.NewHandler()
	// Default stubs — each test overrides what it needs.
	h.SetDownloadFn(func(_ context.Context, _, _, _ string) error {
		return nil
	})
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _, _, _ string) (interface{}, float64, error) {
		return "hello world", 12.5, nil
	})
	return h
}

func doRequest(app *fiber.App, body string) (*http.Response, error) {
	reqBody := strings.NewReader(body)
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

func decodeBatch(resp *http.Response) (transcribe.TranscribeBatchResponse, error) {
	var out transcribe.TranscribeBatchResponse
	err := json.NewDecoder(resp.Body).Decode(&out)
	return out, err
}

// ── 1. Invalid JSON ────────────────────────────────────────────────────────────

func TestTranscribe_InvalidJSON(t *testing.T) {
	h := newHandler(t)
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

// ── 2. Deprecated `url` field ─────────────────────────────────────────────────

func TestTranscribe_DeprecatedURLField(t *testing.T) {
	h := newHandler(t)
	app := newTestApp(h)

	body := `{"url":"https://www.youtube.com/shorts/abc123","format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	result := decodeError(resp)
	if result.Error != "deprecated_field" {
		t.Errorf("expected error=deprecated_field, got %q", result.Error)
	}
}

// ── 3. Empty urls ─────────────────────────────────────────────────────────────

func TestTranscribe_EmptyURLs(t *testing.T) {
	h := newHandler(t)
	app := newTestApp(h)

	resp, err := doRequest(app, `{"urls":[],"format":"text"}`)
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

// ── 4. Invalid URL (per-item) ────────────────────────────────────────────────

func TestTranscribe_InvalidURL(t *testing.T) {
	h := newHandler(t)
	app := newTestApp(h)

	body := `{"urls":["https://www.tiktok.com/@user/video/123"],"format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(batch.Results))
	}
	r := batch.Results[0]
	if r.Ok || r.Error != "invalid_url" {
		t.Fatalf("expected invalid_url item, got %+v", r)
	}
}

// ── 5. Invalid Format ──────────────────────────────────────────────────────────

func TestTranscribe_InvalidFormat(t *testing.T) {
	h := newHandler(t)
	app := newTestApp(h)

	body := `{"urls":["https://www.youtube.com/shorts/abc123"],"format":"srt"}`
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

// ── 6. Download Failed ───────────────────────────────────────────────────────

func TestTranscribe_DownloadFailed(t *testing.T) {
	h := newHandler(t)
	h.SetDownloadFn(func(_ context.Context, _, _, _ string) error {
		return errors.New("network error")
	})
	app := newTestApp(h)

	body := `{"urls":["https://www.youtube.com/shorts/abc123"],"format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	r := batch.Results[0]
	if r.Ok || r.Error != "download_failed" {
		t.Fatalf("expected download_failed, got %+v", r)
	}
}

// ── 7. Download Timeout ────────────────────────────────────────────────────────

func TestTranscribe_DownloadTimeout(t *testing.T) {
	h := newHandler(t)
	h.SetDownloadFn(func(_ context.Context, _, _, _ string) error {
		return context.DeadlineExceeded
	})
	app := newTestApp(h)

	body := `{"urls":["https://www.youtube.com/shorts/abc123"],"format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	r := batch.Results[0]
	if r.Ok || r.Error != "timeout" {
		t.Fatalf("expected timeout, got %+v", r)
	}
}

// ── 8. Transcription Failed ───────────────────────────────────────────────────

func TestTranscribe_TranscriptionFailed(t *testing.T) {
	h := newHandler(t)
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _, _, _ string) (interface{}, float64, error) {
		return nil, 0, errors.New("whisper error")
	})
	app := newTestApp(h)

	body := `{"urls":["https://www.youtube.com/shorts/abc123"],"format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	r := batch.Results[0]
	if r.Ok || r.Error != "transcription_failed" {
		t.Fatalf("expected transcription_failed, got %+v", r)
	}
}

// ── 9. Text Success ───────────────────────────────────────────────────────────

func TestTranscribe_TextSuccess(t *testing.T) {
	h := newHandler(t)
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _, _, _ string) (interface{}, float64, error) {
		return "hello world", 12.5, nil
	})
	app := newTestApp(h)

	body := `{"urls":["https://www.youtube.com/shorts/abc123"],"format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(batch.Results))
	}
	r := batch.Results[0]
	if !r.Ok || r.Transcript != "hello world" {
		t.Fatalf("unexpected result: %+v", r)
	}
	if r.DurationSeconds != 12.5 {
		t.Errorf("expected duration_seconds=12.5, got %f", r.DurationSeconds)
	}
	if batch.Proxy.Source != "none" {
		t.Errorf("expected proxy.source=none, got %q", batch.Proxy.Source)
	}
	if batch.Proxy.Status != transcribe.ProxyStatusNotUsed {
		t.Errorf("expected proxy.status=not_used, got %q", batch.Proxy.Status)
	}
	if r.Cached != "none" {
		t.Errorf("expected cached=none, got %q", r.Cached)
	}
}

func TestTranscribe_SuccessBodyIncludesProxyKey(t *testing.T) {
	h := newHandler(t)
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _, _, _ string) (interface{}, float64, error) {
		return "x", 1.0, nil
	})
	app := newTestApp(h)
	resp, err := doRequest(app, `{"urls":["https://www.youtube.com/shorts/abc123"],"format":"text"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"results"`)) || !bytes.Contains(raw, []byte(`"proxy"`)) || !bytes.Contains(raw, []byte(`"not_used"`)) {
		t.Fatalf("expected JSON to include results + proxy + not_used, got: %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"cached"`)) {
		t.Fatalf("expected JSON to include cached, got: %s", raw)
	}
}

// ── 10. Segments Success ───────────────────────────────────────────────────────

func TestTranscribe_SegmentsSuccess(t *testing.T) {
	h := newHandler(t)
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _, _, _ string) (interface{}, float64, error) {
		return []transcribe.Segment{{Start: 0.0, End: 3.4, Text: "hello"}}, 3.4, nil
	})
	app := newTestApp(h)

	body := `{"urls":["https://www.youtube.com/shorts/abc123"],"format":"segments"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	r := batch.Results[0]
	if !r.Ok || len(r.Segments) != 1 {
		t.Fatalf("unexpected result: %+v", r)
	}
	seg := r.Segments[0]
	if seg.Start != 0.0 || seg.End != 3.4 || seg.Text != "hello" {
		t.Errorf("segment mismatch: got %+v", seg)
	}
	if r.DurationSeconds != 3.4 {
		t.Errorf("expected duration_seconds=3.4, got %f", r.DurationSeconds)
	}
	if r.Cached != "none" {
		t.Errorf("expected cached=none, got %q", r.Cached)
	}
}

// ── 11. Default Format ──────────────────────────────────────────────────────────

func TestTranscribe_DefaultFormat(t *testing.T) {
	h := newHandler(t)
	h.SetTranscribeFn(func(_ context.Context, _, _, _, format, _, _ string) (interface{}, float64, error) {
		if format != "text" {
			return nil, 0, errors.New("unexpected format: " + format)
		}
		return "default text", 5.0, nil
	})
	app := newTestApp(h)

	body := `{"urls":["https://www.youtube.com/shorts/abc123"]}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("expected 200, got %d — body: %s", resp.StatusCode, buf.String())
	}

	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	r := batch.Results[0]
	if !r.Ok || r.Transcript != "default text" {
		t.Fatalf("unexpected: %+v", r)
	}
	if r.Cached != "none" {
		t.Errorf("expected cached=none, got %q", r.Cached)
	}
}

// ── 12. Wrong Type From TranscribeFn ───────────────────────────────────────────

func TestTranscribe_WrongTypeFromTranscribeFn(t *testing.T) {
	h := newHandler(t)
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _, _, _ string) (interface{}, float64, error) {
		return 42, 0, nil
	})
	app := newTestApp(h)

	body := `{"urls":["https://www.youtube.com/shorts/abc123"],"format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	r := batch.Results[0]
	if r.Ok || r.Error != "internal_error" {
		t.Fatalf("expected internal_error, got %+v", r)
	}
}

// ── 13. Default language → translate endpoint ─────────────────────────────────

func TestTranscribe_DefaultLanguageTranslates(t *testing.T) {
	h := newHandler(t)
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _, language, _ string) (interface{}, float64, error) {
		if language != "" {
			return nil, 0, errors.New("expected empty language for translate mode, got: " + language)
		}
		return "translated english text", 10.0, nil
	})
	app := newTestApp(h)

	body := `{"urls":["https://www.youtube.com/shorts/abc123"],"format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("expected 200, got %d — body: %s", resp.StatusCode, buf.String())
	}
	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	r := batch.Results[0]
	if !r.Ok || r.Transcript != "translated english text" {
		t.Fatalf("unexpected: %+v", r)
	}
	if r.Cached != "none" {
		t.Errorf("expected cached=none, got %q", r.Cached)
	}
}

// ── 14. With language → transcribe endpoint ───────────────────────────────────

func TestTranscribe_WithLanguageTranscribes(t *testing.T) {
	h := newHandler(t)
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _, language, _ string) (interface{}, float64, error) {
		if language != "hi" {
			return nil, 0, errors.New("expected language=hi, got: " + language)
		}
		return "हिन्दी text", 8.0, nil
	})
	app := newTestApp(h)

	body := `{"urls":["https://www.youtube.com/shorts/abc123"],"format":"text","language":"hi"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("expected 200, got %d — body: %s", resp.StatusCode, buf.String())
	}
	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	r := batch.Results[0]
	if !r.Ok || r.Transcript != "हिन्दी text" {
		t.Fatalf("unexpected: %+v", r)
	}
	if r.Cached != "none" {
		t.Errorf("expected cached=none, got %q", r.Cached)
	}
}

// ── 15. Multiple URLs (mixed outcomes) ────────────────────────────────────────

func TestTranscribe_MultipleURLs(t *testing.T) {
	h := newHandler(t)
	var seen []string
	h.SetTranscribeFn(func(_ context.Context, _, _, audioPath, _, _, _ string) (interface{}, float64, error) {
		seen = append(seen, audioPath)
		return "ok", 1.0, nil
	})
	app := newTestApp(h)

	body := `{"urls":["https://www.youtube.com/shorts/one1","https://www.tiktok.com/bad"],"format":"text"}`
	resp, err := doRequest(app, body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(batch.Results))
	}
	if !batch.Results[0].Ok || batch.Results[0].Transcript != "ok" {
		t.Fatalf("first item: %+v", batch.Results[0])
	}
	if batch.Results[1].Ok || batch.Results[1].Error != "invalid_url" {
		t.Fatalf("second item: %+v", batch.Results[1])
	}
	if len(seen) != 1 {
		t.Fatalf("expected 1 transcribe call, got %d paths %v", len(seen), seen)
	}
}

// ── 16. Cache: cold (enabled) ─────────────────────────────────────────────────

func TestTranscribe_CacheColdWritesFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("CACHE_DIR", dir)
	t.Setenv("PROXY_POOL", "")
	t.Setenv("PROXY_POOL_FILE", "")
	t.Setenv("USE_EMBEDDED_PROXY_POOL", "")

	var dlCalls int
	h := transcribe.NewHandler()
	h.SetDownloadFn(func(_ context.Context, _, _, dest string) error {
		dlCalls++
		return os.WriteFile(dest, []byte("fake"), 0o644)
	})
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _, _, _ string) (interface{}, float64, error) {
		return "hello", 2.0, nil
	})
	app := newTestApp(h)

	resp, err := doRequest(app, `{"urls":["https://www.youtube.com/shorts/abc123xyz"],"format":"text"}`)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if dlCalls != 1 {
		t.Fatalf("expected 1 download, got %d", dlCalls)
	}
	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	r := batch.Results[0]
	if !r.Ok || r.Cached != "none" {
		t.Fatalf("unexpected: %+v", r)
	}
	audioPath := filepath.Join(dir, "audio", "yt_abc123xyz.mp3")
	if _, err := os.Stat(audioPath); err != nil {
		t.Fatalf("expected audio file at %s: %v", audioPath, err)
	}
	trPath := filepath.Join(dir, "transcripts", "yt_abc123xyz__auto__text.json")
	if _, err := os.Stat(trPath); err != nil {
		t.Fatalf("expected transcript file at %s: %v", trPath, err)
	}
}

// ── 17. Cache: audio hit skips download ───────────────────────────────────────

func TestTranscribe_CacheAudioHitSkipsDownload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("CACHE_DIR", dir)
	t.Setenv("PROXY_POOL", "")
	t.Setenv("PROXY_POOL_FILE", "")
	t.Setenv("USE_EMBEDDED_PROXY_POOL", "")

	h := transcribe.NewHandler()
	_ = os.MkdirAll(filepath.Join(dir, "audio"), 0o755)
	audioPath := filepath.Join(dir, "audio", "yt_vid123.mp3")
	if err := os.WriteFile(audioPath, []byte("cached-audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	var dlCalls int
	h.SetDownloadFn(func(_ context.Context, _, _, _ string) error {
		dlCalls++
		return nil
	})
	var trCalls int
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _, _, _ string) (interface{}, float64, error) {
		trCalls++
		return "second lang", 3.0, nil
	})
	app := newTestApp(h)

	resp, err := doRequest(app, `{"urls":["https://www.youtube.com/shorts/vid123"],"format":"text","language":"hi"}`)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if dlCalls != 0 {
		t.Fatalf("expected download skipped, got %d calls", dlCalls)
	}
	if trCalls != 1 {
		t.Fatalf("expected transcribe once, got %d", trCalls)
	}
	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	r := batch.Results[0]
	if !r.Ok || r.Cached != "audio" {
		t.Fatalf("unexpected: %+v", r)
	}
}

// ── 18. Cache: transcript hit skips pipeline ─────────────────────────────────

func TestTranscribe_CacheTranscriptHit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("CACHE_DIR", dir)
	t.Setenv("PROXY_POOL", "")
	t.Setenv("PROXY_POOL_FILE", "")
	t.Setenv("USE_EMBEDDED_PROXY_POOL", "")

	h := transcribe.NewHandler()
	_ = os.MkdirAll(filepath.Join(dir, "transcripts"), 0o755)
	trPath := filepath.Join(dir, "transcripts", "yt_pre123__auto__text.json")
	payload := `{"format":"text","transcript":"from cache","duration_seconds":9.5}`
	if err := os.WriteFile(trPath, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	var dlCalls, trCalls int
	h.SetDownloadFn(func(_ context.Context, _, _, _ string) error {
		dlCalls++
		return nil
	})
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _, _, _ string) (interface{}, float64, error) {
		trCalls++
		return "no", 0, nil
	})
	app := newTestApp(h)

	resp, err := doRequest(app, `{"urls":["https://www.youtube.com/shorts/pre123"],"format":"text"}`)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if dlCalls != 0 || trCalls != 0 {
		t.Fatalf("expected no download/transcribe, got dl=%d tr=%d", dlCalls, trCalls)
	}
	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	r := batch.Results[0]
	if !r.Ok || r.Transcript != "from cache" || r.DurationSeconds != 9.5 {
		t.Fatalf("unexpected body: %+v", r)
	}
	if r.Cached != "transcript" {
		t.Errorf("expected cached=transcript, got %q", r.Cached)
	}
}

// ── 19. Cache: disabled leaves cache dir empty ───────────────────────────────

func TestTranscribe_CacheDisabledNoFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CACHE_ENABLED", "false")
	t.Setenv("CACHE_DIR", dir)
	t.Setenv("PROXY_POOL", "")
	t.Setenv("PROXY_POOL_FILE", "")
	t.Setenv("USE_EMBEDDED_PROXY_POOL", "")

	var dlCalls int
	h := transcribe.NewHandler()
	h.SetDownloadFn(func(_ context.Context, _, _, dest string) error {
		dlCalls++
		return os.WriteFile(dest, []byte("x"), 0o644)
	})
	h.SetTranscribeFn(func(_ context.Context, _, _, _, _, _, _ string) (interface{}, float64, error) {
		return "ok", 1.0, nil
	})
	app := newTestApp(h)

	resp, err := doRequest(app, `{"urls":["https://www.youtube.com/shorts/nocache99"],"format":"text"}`)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if dlCalls != 1 {
		t.Fatalf("expected 1 download, got %d", dlCalls)
	}
	batch, err := decodeBatch(resp)
	if err != nil {
		t.Fatal(err)
	}
	r := batch.Results[0]
	if !r.Ok || r.Cached != "none" {
		t.Fatalf("unexpected: %+v", r)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty cache dir, got %d entries", len(entries))
	}
}
