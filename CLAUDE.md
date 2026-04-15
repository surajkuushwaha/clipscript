# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build        # compile binary to ./main
make run          # run without building (go run)
make test         # run all tests with -v
make watch        # live reload via air (installs if missing)
make clean        # remove ./main binary

# Single test
go test ./internal/transcribe/... -run TestValidateURL -v

# All tests
go test ./...
```

## Architecture

Two-process system: a **Go Fiber API** (`:8080`) and a **go-whisper Docker container** (`:8081`).

```
Client
  → POST /v1/transcribe
  → Go API: validate URL, download audio (go-ytdlp → /tmp/<uuid>.mp3)
  → Transcription backend (see routing below)
  → JSON response
```

**Transcription routing** (in `internal/transcribe/whisper.go`):

| `OPENAI_API_KEY` | `language` field | Backend |
|---|---|---|
| set | empty | OpenAI `/v1/audio/translations` directly (English output) |
| set | provided | go-whisper → OpenAI `/v1/audio/transcriptions` with language hint |
| not set | empty | go-whisper `/api/whisper/translate` (local model, English) |
| not set | provided | go-whisper `/api/whisper/transcribe` with language hint |

The `language` field is a **source** language hint (what language the audio is in), not an output language selector. Omitting it triggers the translate endpoint → always English output.

## Key packages

**`internal/transcribe/`** — all transcription logic:
- `models.go` — request/response structs (`TranscribeRequest`, `Segment`, etc.)
- `validator.go` — URL regex for Instagram Reels + YouTube Shorts; strips query strings before matching
- `proxy.go` — `ProxyProvider` interface; add new providers by implementing the interface + a `case` in `NewProxyProvider()`
- `downloader.go` — `DownloadAudio()` via go-ytdlp; writes to `/tmp/<uuid>.mp3`; caller defers `os.Remove`
- `whisper.go` — `TranscribeAudio()` routes to OpenAI or go-whisper; `transcribeOpenAIDirect()` / `transcribeViaGoWhisper()` are the two implementations
- `handler.go` — `Handler` struct wires everything; `downloadFn`/`transcribeFn` fields are injectable for tests via `SetDownloadFn`/`SetTranscribeFn`

**`internal/server/`** — Fiber app init and route registration.

## Testing patterns

Handler tests (`handler_test.go`) use injectable function fields — stub both by default in `newHandler()`, override only what each test needs:

```go
h := newHandler() // stubs downloadFn + transcribeFn
h.SetTranscribeFn(func(_ context.Context, _, _, _, _, _, _ string) (interface{}, float64, error) {
    return "hello", 12.5, nil
})
```

`TranscribeAudio` signature: `(ctx, goWhisperURL, model, audioPath, format, language, openAIKey string)` — 7 string params after ctx.

Fiber test requests require `req.Host = "localhost"` or the test will fail with "missing required Host header".

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | Go API port |
| `GOWHISPER_URL` | `http://localhost:8081` | go-whisper sidecar |
| `REQUEST_TIMEOUT` | `120` | Full pipeline timeout (seconds) |
| `WHISPER_MODEL` | `ggml-base.bin` | Model ID; use `whisper-1` with OpenAI |
| `PROXY_PROVIDER` | `none` | `none` or `scraperapi` |
| `SCRAPER_API_KEY` | — | Required when `PROXY_PROVIDER=scraperapi` |
| `OPENAI_API_KEY` | — | When set, translate calls OpenAI directly |

Copy `.env.example` → `.env` to get started.

## Infrastructure

`docker-compose.yml` runs both services. go-whisper models persist in a named Docker volume. Download a model before first use:

```bash
curl -X POST http://localhost:8081/api/whisper/model \
  -H "Content-Type: application/json" \
  -d '{"model": "ggml-base.bin"}'
```

System binaries required: `yt-dlp` and `ffmpeg` (used by go-ytdlp for audio extraction).
