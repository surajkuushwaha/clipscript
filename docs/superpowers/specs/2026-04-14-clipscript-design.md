# Clipscript — Design Spec
**Date:** 2026-04-14
**Status:** Approved

---

## 1. Overview

Clipscript converts short-form video links (Instagram Reels, YouTube Shorts) into text transcripts via a simple HTTP API. Users POST a URL and receive plain text or timestamped segments.

---

## 2. Architecture

Two local processes communicating over HTTP:

```
Client
  │
  ▼
Go Fiber API  (:8080)
  │  — URL validation
  │  — yt-dlp download via go-ytdlp (proxy configured here)
  │  — reads audio file, uploads via multipart to go-whisper
  │  — deletes temp audio file
  │
  │  POST /api/whisper/transcribe  (multipart: audio file + model)
  ▼
go-whisper server  (:8081)     — whisper.cpp, model loaded, no Python
  │
  └─► transcription → text (Accept: text/plain)
                    → segments (Accept: application/json)
```

**Responsibility split:**
- Go API: HTTP routing, validation, download, proxy, temp file lifecycle, multipart upload
- go-whisper: transcription only (whisper.cpp, GPU support, no Python)

**No shared filesystem.** Audio file uploaded directly via multipart — go-whisper never accesses the Go server's disk. No path traversal risk.

The go-whisper server is internal only — never exposed publicly.

---

## 3. Public API Contract

### Endpoint

```
POST /v1/transcribe
Content-Type: application/json
```

### Request

```json
{
  "url": "https://www.instagram.com/reel/ABC123/",
  "format": "text"
}
```

| Field    | Type   | Required | Values                        | Default  |
|----------|--------|----------|-------------------------------|----------|
| `url`    | string | yes      | Instagram Reel, YT Shorts URL | —        |
| `format` | string | no       | `"text"` \| `"segments"`      | `"text"` |

### Success Responses

**format=text:**
```json
{
  "transcript": "Hello world this is my video...",
  "duration_seconds": 47.2
}
```

**format=segments:**
```json
{
  "segments": [
    { "start": 0.0, "end": 3.4, "text": "Hello world" },
    { "start": 3.4, "end": 7.1, "text": "this is my video" }
  ],
  "duration_seconds": 47.2
}
```

### Error Responses

| HTTP Status | `error` field          | Cause                               |
|-------------|------------------------|-------------------------------------|
| 400         | `invalid_url`          | URL not a supported platform format |
| 422         | `download_failed`      | yt-dlp could not download the video |
| 408         | `timeout`              | Processing exceeded `REQUEST_TIMEOUT` |
| 500         | `transcription_failed` | go-whisper error                    |

```json
{ "error": "invalid_url", "message": "URL must be an Instagram Reel or YouTube Shorts link" }
```

---

## 4. Go API — Internal Structure

```
cmd/api/main.go
internal/
  server/
    server.go                 # Fiber app init (existing)
    routes.go                 # add POST /v1/transcribe
  transcribe/
    handler.go                # orchestrates full request lifecycle
    validator.go              # URL regex validation (IG reel + YT shorts)
    downloader.go             # yt-dlp via go-ytdlp, audio-only mp3 to /tmp
    proxy.go                  # proxy provider abstraction
    whisper.go                # HTTP client to go-whisper (multipart upload)
    models.go                 # request/response/error structs
```

### handler.go flow

```
1. Parse + validate request (validator.go)
2. Create overall context with REQUEST_TIMEOUT (covers steps 3–5)
3. Build proxy URL (proxy.go)
4. Download audio → /tmp/<uuid>.mp3 (downloader.go)
5. Upload audio file to go-whisper, get transcript (whisper.go)
6. Delete /tmp/<uuid>.mp3 (deferred — always runs)
7. Shape + return response
```

**Overall timeout:** A single context with `REQUEST_TIMEOUT` (default: 120s) wraps steps 3–5, covering download + transcription end-to-end.

### URL validation patterns

- Instagram Reel: `https://www.instagram.com/reel/<ID>/`
- YouTube Shorts: `https://www.youtube.com/shorts/<ID>`

### downloader.go

Uses `github.com/lrstanley/go-ytdlp` with `--extract-audio --audio-format mp3` flags. yt-dlp invokes the `ffmpeg` binary internally for extraction — no ffmpeg-go wrapper needed.

Output: `/tmp/<uuid>.mp3`. Proxy URL (if configured) passed to go-ytdlp options.

### whisper.go

Sends `multipart/form-data` POST to `GOWHISPER_URL/api/whisper/transcribe`:
- `audio` field: mp3 file bytes
- `model` field: value of `WHISPER_MODEL` env var

Sets `Accept` header based on requested format:
- `format=text` → `Accept: text/plain` → returns raw string as `transcript`
- `format=segments` → `Accept: application/json` → parses segments array

---

## 5. Proxy Abstraction (Go)

Lives in `internal/transcribe/proxy.go`. Decoupled from any specific provider.

```go
type ProxyProvider interface {
    ProxyURL() string // returns "" for no proxy
}

type NoProxy struct{}
func (NoProxy) ProxyURL() string { return "" }

type ScraperAPIProxy struct{ APIKey string }
func (s ScraperAPIProxy) ProxyURL() string {
    return fmt.Sprintf("http://scraperapi:%s@proxy-server.scraperapi.com:8001", s.APIKey)
}

func NewProxyProvider() ProxyProvider {
    switch os.Getenv("PROXY_PROVIDER") {
    case "scraperapi":
        return ScraperAPIProxy{APIKey: os.Getenv("SCRAPER_API_KEY")}
    default:
        return NoProxy{}
    }
}
```

**To add a new provider:** implement `ProxyProvider` interface, add a `case` in `NewProxyProvider()`, set env var.

---

## 6. go-whisper Sidecar

`go-whisper` runs as a standalone binary or Docker container. It is NOT part of the Go codebase — it is an external dependency run as a separate process.

**Run via Docker (recommended):**
```bash
docker volume create whisper
docker run -d --name whisper-server \
  -v whisper:/data -p 8081:8081 \
  ghcr.io/mutablelogic/go-whisper run
```

**Download model before first use:**
```bash
# Using gowhisper CLI, or via API:
curl -X POST http://localhost:8081/api/whisper/model \
  -H "Content-Type: application/json" \
  -d '{"model": "ggml-base"}'
```

**Transcription API (called by `whisper.go`):**
```
POST /api/whisper/transcribe
Content-Type: multipart/form-data

Fields:
  audio   (file)   — mp3 audio file
  model   (string) — e.g. "ggml-base"

Accept: text/plain         → plain transcript string
Accept: application/json   → { segments: [{start, end, text}], ... }
```

---

## 7. Configuration (Environment Variables)

| Variable           | Default                        | Description                                            |
|--------------------|-------------------------------|--------------------------------------------------------|
| `PORT`             | `8080`                         | Go API listen port                                     |
| `GOWHISPER_URL`    | `http://localhost:8081`        | go-whisper server base URL                             |
| `REQUEST_TIMEOUT`  | `120`                          | Max seconds for full pipeline (download + transcription) |
| `WHISPER_MODEL`    | `ggml-base`                    | Whisper model ID (must be pre-downloaded in go-whisper) |
| `PROXY_PROVIDER`   | `none`                         | Proxy: `none` \| `scraperapi`                          |
| `SCRAPER_API_KEY`  | —                              | Required when `PROXY_PROVIDER=scraperapi`              |

**Available models** (download via go-whisper):
`ggml-tiny`, `ggml-base`, `ggml-small`, `ggml-medium`, `ggml-large-v3`

---

## 8. Error Handling

- **Invalid URL:** rejected in `validator.go` before any download — fast 400
- **Download failure:** go-ytdlp error → 422 `download_failed`
- **Transcription failure:** go-whisper non-2xx response → 500 `transcription_failed`
- **Timeout:** `REQUEST_TIMEOUT` context deadline exceeded (covers download + transcription) → 408 `timeout`
- **Temp file cleanup:** deferred in `handler.go`, always runs even on error

---

## 9. Dependencies

**Go:**
- `github.com/gofiber/fiber/v2` — HTTP framework (existing)
- `github.com/lrstanley/go-ytdlp` — yt-dlp Go wrapper

**External services (run separately):**
- `go-whisper` — `ghcr.io/mutablelogic/go-whisper` Docker image

**System binaries:**
- `yt-dlp` binary
- `ffmpeg` binary

**No Python. No Python dependencies.**

---

## 10. Out of Scope (MVP)

- Auth / API keys
- Rate limiting
- Job queue / async processing
- File upload (URL only)
- Multi-language detection
- Speaker diarization
- SRT/VTT subtitle output
- User dashboard or history

---

## 11. Future Considerations

- Add `BrightDataProxy`, `OxylabsProxy` — implement `ProxyProvider` interface, change env var
- Async job queue for concurrency
- GPU-accelerated go-whisper container (CUDA/Metal) for faster transcription
- Streaming transcription via go-whisper SSE support
