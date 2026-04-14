# Clipscript

Convert Instagram Reels and YouTube Shorts links into text transcripts via a simple HTTP API.

## How It Works

Two services run locally:

1. **Go API** — validates URLs, downloads audio via yt-dlp (`go-ytdlp`), uploads to go-whisper, returns transcript
2. **go-whisper** — `whisper.cpp`-based transcription server (Docker), no Python required

```
Client → POST /v1/transcribe → Go API → yt-dlp (audio) → go-whisper → transcript
```

No Python. No shared filesystem. Audio uploaded directly via multipart to go-whisper.

## Prerequisites

- Go 1.21+
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) binary — `pip install yt-dlp`
- [ffmpeg](https://ffmpeg.org/) binary — required by yt-dlp for audio extraction
- Docker (for go-whisper)

## Setup

**1. Clone and configure:**

```bash
cp .env.example .env
# Edit .env with your settings
```

**2. Start go-whisper (transcription server):**

```bash
docker volume create whisper
docker run -d --name whisper-server \
  -v whisper:/data -p 8081:8081 \
  ghcr.io/mutablelogic/go-whisper run
```

**3. Download a Whisper model:**

```bash
curl -X POST http://localhost:8081/api/whisper/model \
  -H "Content-Type: application/json" \
  -d '{"model": "ggml-base"}'
```

> First model download may take a minute. Model stays cached in the Docker volume.

**4. Start the Go API:**

```bash
make run
```

## API

### Transcribe a video

```
POST /v1/transcribe
Content-Type: application/json
```

**Request:**
```json
{
  "url": "https://www.instagram.com/reel/ABC123/",
  "format": "text"
}
```

| Field    | Required | Values                           | Default  |
|----------|----------|----------------------------------|----------|
| `url`    | yes      | Instagram Reel or YouTube Shorts URL | —    |
| `format` | no       | `"text"` or `"segments"`         | `"text"` |

**Response (format=text):**
```json
{
  "transcript": "Hello world this is my video...",
  "duration_seconds": 47.2
}
```

**Response (format=segments):**
```json
{
  "segments": [
    { "start": 0.0, "end": 3.4, "text": "Hello world" },
    { "start": 3.4, "end": 7.1, "text": "this is my video" }
  ],
  "duration_seconds": 47.2
}
```

**Error response:**
```json
{ "error": "invalid_url", "message": "URL must be an Instagram Reel or YouTube Shorts link" }
```

| Status | Error                  | Cause                          |
|--------|------------------------|--------------------------------|
| 400    | `invalid_url`          | Unsupported or malformed URL   |
| 422    | `download_failed`      | yt-dlp could not download      |
| 408    | `timeout`              | Processing exceeded timeout    |
| 500    | `transcription_failed` | go-whisper error               |

## Configuration

All config via environment variables (copy `.env.example` to `.env`):

| Variable          | Default                   | Description                                          |
|-------------------|---------------------------|------------------------------------------------------|
| `PORT`            | `8080`                    | Go API port                                          |
| `GOWHISPER_URL`   | `http://localhost:8081`   | go-whisper server address                            |
| `REQUEST_TIMEOUT` | `120`                     | Max seconds for full pipeline (download + transcription) |
| `WHISPER_MODEL`   | `ggml-base`               | Model ID (must be pre-downloaded in go-whisper)      |
| `PROXY_PROVIDER`  | `none`                    | Proxy: `none` or `scraperapi`                        |
| `SCRAPER_API_KEY` | —                         | Required when `PROXY_PROVIDER=scraperapi`            |

### Whisper model trade-offs

| Model          | Size   | Speed  | Accuracy |
|----------------|--------|--------|----------|
| `ggml-tiny`    | ~75MB  | fast   | low      |
| `ggml-base`    | ~145MB | fast   | good     |
| `ggml-small`   | ~465MB | medium | better   |
| `ggml-medium`  | ~1.5GB | slow   | best     |

Download a model:
```bash
curl -X POST http://localhost:8081/api/whisper/model \
  -H "Content-Type: application/json" \
  -d '{"model": "ggml-small"}'
```

### Proxy support

Set `PROXY_PROVIDER=scraperapi` and add your `SCRAPER_API_KEY` to route yt-dlp through [ScraperAPI](https://www.scraperapi.com/). The proxy layer is designed to be swappable — see `internal/transcribe/proxy.go`.

## Development

```bash
# Build
make build

# Run with live reload
make watch

# Run tests
make test

# Clean build artifacts
make clean
```

## Supported Platforms

- Instagram Reels (`instagram.com/reel/*`)
- YouTube Shorts (`youtube.com/shorts/*`)
