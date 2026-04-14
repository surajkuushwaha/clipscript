# Clipscript MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go HTTP API that accepts Instagram Reel / YouTube Shorts URLs and returns text transcripts by downloading audio via yt-dlp and transcribing via go-whisper.

**Architecture:** Go Fiber API handles validation, download (go-ytdlp), proxy, and temp file lifecycle. Audio is uploaded via multipart to a go-whisper Docker container which runs whisper.cpp transcription. No Python, no shared filesystem.

**Tech Stack:** Go 1.21+, Fiber v2, go-ytdlp, go-whisper (Docker), yt-dlp binary, ffmpeg binary

---

## File Map

### Created
- `internal/transcribe/models.go` — request/response/error structs
- `internal/transcribe/validator.go` — URL regex validation
- `internal/transcribe/proxy.go` — proxy provider interface + implementations
- `internal/transcribe/downloader.go` — yt-dlp audio download via go-ytdlp
- `internal/transcribe/whisper.go` — multipart upload to go-whisper, response parsing
- `internal/transcribe/handler.go` — orchestrates full pipeline with timeout
- `internal/transcribe/validator_test.go` — validator unit tests
- `internal/transcribe/proxy_test.go` — proxy unit tests
- `internal/transcribe/handler_test.go` — handler integration tests (mocked downloader + whisper)

### Modified
- `internal/server/routes.go` — register `POST /v1/transcribe`
- `go.mod` / `go.sum` — add go-ytdlp dependency
- `.env` / `.env.example` — already updated

---

## Task 1: Add go-ytdlp dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add dependency**

```bash
go get github.com/lrstanley/go-ytdlp
```

- [ ] **Step 2: Verify it resolves**

```bash
go mod tidy
```

Expected: no errors, `go.mod` now lists `github.com/lrstanley/go-ytdlp`.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add go-ytdlp dependency"
```

---

## Task 2: Define request/response structs

**Files:**
- Create: `internal/transcribe/models.go`

- [ ] **Step 1: Create models file**

```go
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
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/transcribe/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/transcribe/models.go
git commit -m "feat: add transcribe request/response models"
```

---

## Task 3: URL validator

**Files:**
- Create: `internal/transcribe/validator.go`
- Create: `internal/transcribe/validator_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/transcribe/validator_test.go
package transcribe_test

import (
	"testing"
	"clipscript/internal/transcribe"
)

func TestValidateURL(t *testing.T) {
	valid := []string{
		"https://www.instagram.com/reel/ABC123/",
		"https://www.instagram.com/reel/Xy_-Z9/",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ",
		"https://youtube.com/shorts/dQw4w9WgXcQ",
	}
	invalid := []string{
		"",
		"not-a-url",
		"https://www.instagram.com/p/ABC123/",   // post, not reel
		"https://www.youtube.com/watch?v=abc",   // regular video
		"https://www.tiktok.com/@user/video/1",  // unsupported platform
		"http://evil.com/reel/ABC123/",          // wrong domain
	}

	for _, u := range valid {
		if err := transcribe.ValidateURL(u); err != nil {
			t.Errorf("expected valid, got error for %q: %v", u, err)
		}
	}
	for _, u := range invalid {
		if err := transcribe.ValidateURL(u); err == nil {
			t.Errorf("expected error, got nil for %q", u)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/transcribe/... -run TestValidateURL -v
```

Expected: FAIL — `transcribe.ValidateURL undefined`

- [ ] **Step 3: Implement validator**

```go
// internal/transcribe/validator.go
package transcribe

import (
	"fmt"
	"regexp"
)

var (
	reInstagramReel  = regexp.MustCompile(`^https://www\.instagram\.com/reel/[\w\-]+/?$`)
	reYouTubeShorts  = regexp.MustCompile(`^https://(www\.)?youtube\.com/shorts/[\w\-]+$`)
)

// ValidateURL returns nil if url is a supported Instagram Reel or YouTube Shorts link.
func ValidateURL(url string) error {
	if reInstagramReel.MatchString(url) || reYouTubeShorts.MatchString(url) {
		return nil
	}
	return fmt.Errorf("URL must be an Instagram Reel or YouTube Shorts link")
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/transcribe/... -run TestValidateURL -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transcribe/validator.go internal/transcribe/validator_test.go
git commit -m "feat: add URL validator for Instagram Reels and YouTube Shorts"
```

---

## Task 4: Proxy abstraction

**Files:**
- Create: `internal/transcribe/proxy.go`
- Create: `internal/transcribe/proxy_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/transcribe/proxy_test.go
package transcribe_test

import (
	"os"
	"testing"
	"clipscript/internal/transcribe"
)

func TestNoProxy(t *testing.T) {
	p := transcribe.NoProxy{}
	if got := p.ProxyURL(); got != "" {
		t.Errorf("NoProxy.ProxyURL() = %q, want empty string", got)
	}
}

func TestScraperAPIProxy(t *testing.T) {
	p := transcribe.ScraperAPIProxy{APIKey: "testkey123"}
	got := p.ProxyURL()
	want := "http://scraperapi:testkey123@proxy-server.scraperapi.com:8001"
	if got != want {
		t.Errorf("ScraperAPIProxy.ProxyURL() = %q, want %q", got, want)
	}
}

func TestNewProxyProvider_None(t *testing.T) {
	os.Setenv("PROXY_PROVIDER", "none")
	p := transcribe.NewProxyProvider()
	if p.ProxyURL() != "" {
		t.Errorf("expected empty proxy URL for provider=none")
	}
}

func TestNewProxyProvider_ScraperAPI(t *testing.T) {
	os.Setenv("PROXY_PROVIDER", "scraperapi")
	os.Setenv("SCRAPER_API_KEY", "mykey")
	p := transcribe.NewProxyProvider()
	if p.ProxyURL() == "" {
		t.Errorf("expected non-empty proxy URL for provider=scraperapi")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/transcribe/... -run TestNoProxy -v
```

Expected: FAIL — `transcribe.NoProxy undefined`

- [ ] **Step 3: Implement proxy abstraction**

```go
// internal/transcribe/proxy.go
package transcribe

import (
	"fmt"
	"os"
)

// ProxyProvider returns a proxy URL for use with yt-dlp.
// An empty string means no proxy.
type ProxyProvider interface {
	ProxyURL() string
}

// NoProxy disables proxy routing.
type NoProxy struct{}

func (NoProxy) ProxyURL() string { return "" }

// ScraperAPIProxy routes requests through ScraperAPI.
type ScraperAPIProxy struct {
	APIKey string
}

func (s ScraperAPIProxy) ProxyURL() string {
	return fmt.Sprintf("http://scraperapi:%s@proxy-server.scraperapi.com:8001", s.APIKey)
}

// NewProxyProvider reads PROXY_PROVIDER env var and returns the matching provider.
// Add new providers here by implementing ProxyProvider and adding a case.
func NewProxyProvider() ProxyProvider {
	switch os.Getenv("PROXY_PROVIDER") {
	case "scraperapi":
		return ScraperAPIProxy{APIKey: os.Getenv("SCRAPER_API_KEY")}
	default:
		return NoProxy{}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/transcribe/... -run "TestNoProxy|TestScraperAPIProxy|TestNewProxyProvider" -v
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transcribe/proxy.go internal/transcribe/proxy_test.go
git commit -m "feat: add pluggable proxy provider abstraction"
```

---

## Task 5: Audio downloader

**Files:**
- Create: `internal/transcribe/downloader.go`

- [ ] **Step 1: Implement downloader**

This function shells out to yt-dlp via go-ytdlp. It cannot be unit tested without network access — tested end-to-end in Task 8.

```go
// internal/transcribe/downloader.go
package transcribe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	ytdlp "github.com/lrstanley/go-ytdlp"
)

// DownloadAudio downloads audio-only from url to a temp mp3 file.
// Returns the file path. Caller is responsible for deleting the file.
// proxyURL may be empty string for no proxy.
func DownloadAudio(ctx context.Context, url, proxyURL string) (string, error) {
	tmpFile, err := os.CreateTemp("", "clipscript-*.mp3")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	// Remove the empty placeholder so yt-dlp can write to the path freely.
	os.Remove(tmpPath)

	dl := ytdlp.New().
		ExtractAudio().
		AudioFormat("mp3").
		Output(strings.TrimSuffix(tmpPath, filepath.Ext(tmpPath)) + ".%(ext)s")

	if proxyURL != "" {
		dl = dl.Proxy(proxyURL)
	}

	if _, err := dl.Run(ctx, url); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("yt-dlp: %w", err)
	}

	// yt-dlp appends the extension; our path is already .mp3
	if _, err := os.Stat(tmpPath); err != nil {
		return "", fmt.Errorf("audio file not found after download: %w", err)
	}

	return tmpPath, nil
}
```

- [ ] **Step 2: Add missing import**

The file above uses `strings` — add it to the import block:

```go
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ytdlp "github.com/lrstanley/go-ytdlp"
)
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./internal/transcribe/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/transcribe/downloader.go
git commit -m "feat: add yt-dlp audio downloader"
```

---

## Task 6: go-whisper client

**Files:**
- Create: `internal/transcribe/whisper.go`

- [ ] **Step 1: Implement whisper client**

```go
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
	mw.Close()

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
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/transcribe/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/transcribe/whisper.go
git commit -m "feat: add go-whisper multipart upload client"
```

---

## Task 7: Handler + route registration

**Files:**
- Create: `internal/transcribe/handler.go`
- Modify: `internal/server/routes.go`

- [ ] **Step 1: Implement handler**

```go
// internal/transcribe/handler.go
package transcribe

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Handler holds config needed to serve transcription requests.
type Handler struct {
	GoWhisperURL   string
	WhisperModel   string
	RequestTimeout time.Duration
	Proxy          ProxyProvider
}

// NewHandler builds a Handler from environment variables.
func NewHandler() *Handler {
	timeout := 120
	if v, err := strconv.Atoi(os.Getenv("REQUEST_TIMEOUT")); err == nil && v > 0 {
		timeout = v
	}
	goWhisperURL := os.Getenv("GOWHISPER_URL")
	if goWhisperURL == "" {
		goWhisperURL = "http://localhost:8081"
	}
	model := os.Getenv("WHISPER_MODEL")
	if model == "" {
		model = "ggml-base"
	}
	return &Handler{
		GoWhisperURL:   goWhisperURL,
		WhisperModel:   model,
		RequestTimeout: time.Duration(timeout) * time.Second,
		Proxy:          NewProxyProvider(),
	}
}

// Transcribe handles POST /v1/transcribe.
func (h *Handler) Transcribe(c *fiber.Ctx) error {
	var req TranscribeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "request body must be valid JSON with a 'url' field",
		})
	}

	if req.Format == "" {
		req.Format = "text"
	}

	if req.Format != "text" && req.Format != "segments" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_format",
			Message: "format must be 'text' or 'segments'",
		})
	}

	if err := ValidateURL(req.URL); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_url",
			Message: err.Error(),
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.RequestTimeout)
	defer cancel()

	audioPath, err := DownloadAudio(ctx, req.URL, h.Proxy.ProxyURL())
	if err != nil {
		if ctx.Err() != nil {
			return c.Status(fiber.StatusRequestTimeout).JSON(ErrorResponse{
				Error:   "timeout",
				Message: "processing exceeded timeout limit",
			})
		}
		return c.Status(fiber.StatusUnprocessableEntity).JSON(ErrorResponse{
			Error:   "download_failed",
			Message: err.Error(),
		})
	}
	defer os.Remove(audioPath)

	result, duration, err := TranscribeAudio(ctx, h.GoWhisperURL, h.WhisperModel, audioPath, req.Format)
	if err != nil {
		if ctx.Err() != nil {
			return c.Status(fiber.StatusRequestTimeout).JSON(ErrorResponse{
				Error:   "timeout",
				Message: "processing exceeded timeout limit",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "transcription_failed",
			Message: err.Error(),
		})
	}

	if req.Format == "segments" {
		return c.JSON(TranscribeSegmentsResponse{
			Segments:        result.([]Segment),
			DurationSeconds: duration,
		})
	}
	return c.JSON(TranscribeTextResponse{
		Transcript:      result.(string),
		DurationSeconds: duration,
	})
}
```

- [ ] **Step 2: Register route**

Edit `internal/server/routes.go`:

```go
package server

import (
	"clipscript/internal/transcribe"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func (s *FiberServer) RegisterFiberRoutes() {
	s.App.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS,PATCH",
		AllowHeaders:     "Accept,Authorization,Content-Type",
		AllowCredentials: false,
		MaxAge:           300,
	}))

	s.App.Get("/", s.HelloWorldHandler)

	h := transcribe.NewHandler()
	s.App.Post("/v1/transcribe", h.Transcribe)
}

func (s *FiberServer) HelloWorldHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"message": "Hello World"})
}
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/transcribe/handler.go internal/server/routes.go
git commit -m "feat: wire up transcribe handler to POST /v1/transcribe"
```

---

## Task 8: Handler unit tests (mocked dependencies)

**Files:**
- Create: `internal/transcribe/handler_test.go`

These tests mock DownloadAudio and TranscribeAudio by testing the handler's error-routing logic via HTTP test cases. We swap the handler's internals by making the handler accept functional options for the download and transcribe steps.

- [ ] **Step 1: Refactor handler to accept injectable functions (testability)**

Update `internal/transcribe/handler.go` — add function fields:

```go
// Handler holds config needed to serve transcription requests.
type Handler struct {
	GoWhisperURL   string
	WhisperModel   string
	RequestTimeout time.Duration
	Proxy          ProxyProvider
	// Injectable for testing
	downloadFn   func(ctx context.Context, url, proxyURL string) (string, error)
	transcribeFn func(ctx context.Context, goWhisperURL, model, audioPath, format string) (interface{}, float64, error)
}

// NewHandler builds a Handler from environment variables.
func NewHandler() *Handler {
	timeout := 120
	if v, err := strconv.Atoi(os.Getenv("REQUEST_TIMEOUT")); err == nil && v > 0 {
		timeout = v
	}
	goWhisperURL := os.Getenv("GOWHISPER_URL")
	if goWhisperURL == "" {
		goWhisperURL = "http://localhost:8081"
	}
	model := os.Getenv("WHISPER_MODEL")
	if model == "" {
		model = "ggml-base"
	}
	return &Handler{
		GoWhisperURL:   goWhisperURL,
		WhisperModel:   model,
		RequestTimeout: time.Duration(timeout) * time.Second,
		Proxy:          NewProxyProvider(),
		downloadFn:     DownloadAudio,
		transcribeFn:   TranscribeAudio,
	}
}
```

Replace direct calls in `Transcribe()`:
- `DownloadAudio(ctx, ...)` → `h.downloadFn(ctx, ...)`
- `TranscribeAudio(ctx, ...)` → `h.transcribeFn(ctx, ...)`

- [ ] **Step 2: Write tests**

```go
// internal/transcribe/handler_test.go
package transcribe_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clipscript/internal/transcribe"
	"github.com/gofiber/fiber/v2"
)

func newTestApp(h *transcribe.Handler) *fiber.App {
	app := fiber.New()
	app.Post("/v1/transcribe", h.Transcribe)
	return app
}

func TestTranscribe_InvalidJSON(t *testing.T) {
	h := transcribe.NewHandler()
	app := newTestApp(h)

	req := httptest.NewRequest(http.MethodPost, "/v1/transcribe", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTranscribe_InvalidURL(t *testing.T) {
	h := transcribe.NewHandler()
	app := newTestApp(h)

	body, _ := json.Marshal(map[string]string{"url": "https://tiktok.com/video/1"})
	req := httptest.NewRequest(http.MethodPost, "/v1/transcribe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTranscribe_DownloadFailed(t *testing.T) {
	h := transcribe.NewHandler()
	h.SetDownloadFn(func(ctx context.Context, url, proxy string) (string, error) {
		return "", fmt.Errorf("yt-dlp: HTTP Error 429")
	})
	app := newTestApp(h)

	body, _ := json.Marshal(map[string]string{"url": "https://www.youtube.com/shorts/abc123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/transcribe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", resp.StatusCode)
	}
}

func TestTranscribe_TextSuccess(t *testing.T) {
	h := transcribe.NewHandler()
	h.SetDownloadFn(func(ctx context.Context, url, proxy string) (string, error) {
		return "/tmp/fake-audio.mp3", nil
	})
	h.SetTranscribeFn(func(ctx context.Context, goWhisperURL, model, audioPath, format string) (interface{}, float64, error) {
		return "hello world", 12.5, nil
	})
	app := newTestApp(h)

	body, _ := json.Marshal(map[string]string{"url": "https://www.youtube.com/shorts/abc123", "format": "text"})
	req := httptest.NewRequest(http.MethodPost, "/v1/transcribe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var result transcribe.TranscribeTextResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Transcript != "hello world" {
		t.Errorf("expected 'hello world', got %q", result.Transcript)
	}
}

func TestTranscribe_SegmentsSuccess(t *testing.T) {
	h := transcribe.NewHandler()
	h.SetDownloadFn(func(ctx context.Context, url, proxy string) (string, error) {
		return "/tmp/fake-audio.mp3", nil
	})
	h.SetTranscribeFn(func(ctx context.Context, goWhisperURL, model, audioPath, format string) (interface{}, float64, error) {
		return []transcribe.Segment{{Start: 0.0, End: 3.4, Text: "hello"}}, 3.4, nil
	})
	app := newTestApp(h)

	body, _ := json.Marshal(map[string]string{"url": "https://www.youtube.com/shorts/abc123", "format": "segments"})
	req := httptest.NewRequest(http.MethodPost, "/v1/transcribe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var result transcribe.TranscribeSegmentsResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Segments) != 1 || result.Segments[0].Text != "hello" {
		t.Errorf("unexpected segments: %+v", result.Segments)
	}
}
```

- [ ] **Step 3: Add exported setter methods to Handler**

Add to `internal/transcribe/handler.go`:

```go
// SetDownloadFn overrides the download function (for testing).
func (h *Handler) SetDownloadFn(fn func(ctx context.Context, url, proxyURL string) (string, error)) {
	h.downloadFn = fn
}

// SetTranscribeFn overrides the transcribe function (for testing).
func (h *Handler) SetTranscribeFn(fn func(ctx context.Context, goWhisperURL, model, audioPath, format string) (interface{}, float64, error)) {
	h.transcribeFn = fn
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/transcribe/... -v
```

Expected: all tests PASS (validator, proxy, handler tests)

- [ ] **Step 5: Commit**

```bash
git add internal/transcribe/handler.go internal/transcribe/handler_test.go
git commit -m "test: add handler unit tests with injectable dependencies"
```

---

## Task 9: go-whisper setup verification

This task verifies the full pipeline works end-to-end with a real go-whisper instance.

- [ ] **Step 1: Start go-whisper**

```bash
docker volume create whisper
docker run -d --name whisper-server \
  -v whisper:/data -p 8081:8081 \
  ghcr.io/mutablelogic/go-whisper run
```

- [ ] **Step 2: Wait for server to be ready**

```bash
curl -s http://localhost:8081/api/whisper/model
```

Expected: `{"object":"list","models":[]}` (empty list is fine — server is up)

- [ ] **Step 3: Download ggml-base model**

```bash
curl -X POST http://localhost:8081/api/whisper/model \
  -H "Content-Type: application/json" \
  -d '{"model": "ggml-base"}'
```

Expected: JSON response with model info once download completes (may take ~30 seconds).

- [ ] **Step 4: Configure .env**

```
GOWHISPER_URL=http://localhost:8081
WHISPER_MODEL=ggml-base
PROXY_PROVIDER=none
PORT=8080
REQUEST_TIMEOUT=120
```

- [ ] **Step 5: Start Go API and test end-to-end**

```bash
make run
```

In another terminal:

```bash
curl -X POST http://localhost:8080/v1/transcribe \
  -H "Content-Type: application/json" \
  -d '{"url": "https://www.youtube.com/shorts/<a-real-shorts-id>", "format": "text"}'
```

Expected: JSON response with `transcript` and `duration_seconds`.

- [ ] **Step 6: Test segments format**

```bash
curl -X POST http://localhost:8080/v1/transcribe \
  -H "Content-Type: application/json" \
  -d '{"url": "https://www.youtube.com/shorts/<a-real-shorts-id>", "format": "segments"}'
```

Expected: JSON response with `segments` array and `duration_seconds`.

---

## Task 10: Run full test suite + final commit

- [ ] **Step 1: Run all tests**

```bash
go test ./... -v
```

Expected: all PASS

- [ ] **Step 2: Build binary**

```bash
make build
```

Expected: no errors, binary at `./bin/clipscript` (or per Makefile target).

- [ ] **Step 3: Final commit**

```bash
git add -A
git commit -m "feat: clipscript MVP — video URL to transcript via go-ytdlp + go-whisper"
```
