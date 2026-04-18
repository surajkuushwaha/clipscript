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

### Option A — Docker Compose (recommended)

```bash
cp .env.example .env
# Edit .env with your settings

docker compose up --build
```

**Using a local model** (first time only — cached in Docker volume):

```bash
curl -X POST http://localhost:8081/api/whisper/model \
  -H "Content-Type: application/json" \
  -d '{"model": "ggml-base.bin"}'
```

**Using OpenAI Whisper API instead** (no local model download needed):

```bash
# In .env:
OPENAI_API_KEY=sk-...
WHISPER_MODEL=whisper-1
```

Then restart: `docker compose up`

### Option B — Manual

**1. Configure:**

```bash
cp .env.example .env
# Edit .env with your settings
```

**2. Start go-whisper:**

```bash
docker run -d --name whisper-server \
  -v whisper-models:/data -p 8081:8081 \
  ghcr.io/mutablelogic/go-whisper run
```

**3. Download a Whisper model:**

```bash
curl -X POST http://localhost:8081/api/whisper/model \
  -H "Content-Type: application/json" \
  -d '{"model": "ggml-base.bin"}'
```

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

| Field      | Required | Values                               | Default  |
|------------|----------|--------------------------------------|----------|
| `url`      | yes      | Instagram Reel or YouTube Shorts URL | —        |
| `format`   | no       | `"text"` or `"segments"`             | `"text"` |
| `language` | no       | BCP-47 code e.g. `"hi"`, `"ur"`, `"fr"` | —    |

**`language` field behaviour:**
- **Omitted (default)** — uses the translate endpoint → output is always **English**, regardless of audio language. Best for Hinglish / mixed-language content.
- **Provided** (e.g. `"hi"`) — uses the transcribe endpoint with that language as a source hint → output stays in that language with better accuracy.
- `language` is a **source** language hint (what language the audio is in), not an output language selector. Setting `language="en"` on Hindi audio still produces Hindi output.

**Response (format=text):**
```json
{
  "transcript": "Hello world this is my video...",
  "duration_seconds": 47.2,
  "cached": "none",
  "proxy": {
    "used": true,
    "status": "in_use",
    "source": "pool_embedded",
    "endpoint": "31.59.20.176:6754",
    "pool_selection": "round_robin"
  }
}
```

`cached` is `none` (fresh pipeline), `audio` (reused downloaded mp3), or `transcript` (full cache hit). Enable persistence with `CACHE_ENABLED=true` and optional `CACHE_DIR` (default `./cache`); see `.env.example`.

`proxy` is always present. `status` is `not_used` (direct download) or `in_use`. `source` is `none`, `pool_file`, or `pool_inline`. `endpoint` is the proxy host and port only (no credentials).

**Response (format=segments):**
```json
{
  "segments": [
    { "start": 0.0, "end": 3.4, "text": "Hello world" },
    { "start": 3.4, "end": 7.1, "text": "this is my video" }
  ],
  "duration_seconds": 47.2,
  "proxy": {
    "used": false,
    "status": "not_used",
    "source": "none"
  }
}
```

`proxy.status` is always `"in_use"` or `"not_used"` so clients do not need to infer from `used` alone.

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
| `PROXY_POOL_FILE` | —                         | Path to JSON proxy pool (highest precedence) |
| `PROXY_POOL`      | —                         | Inline JSON proxy pool |
| `YTDLP_NO_CHECK_CERTIFICATES` | —               | Set `true` / `1` for local dev if yt-dlp fails with `CERTIFICATE_VERIFY_FAILED` (insecure; fix CA bundle instead when possible) |
| `OPENAI_API_KEY`  | —                         | Optional — routes transcription through OpenAI API; set `WHISPER_MODEL=whisper-1` |

### Whisper model trade-offs

| Model         | Size   | Speed  | Accuracy |
|---------------|--------|--------|----------|
| `ggml-tiny`   | ~75MB  | fast   | low      |
| `ggml-base`   | ~145MB | fast   | good     |
| `ggml-small`  | ~465MB | medium | better   |
| `ggml-medium` | ~1.5GB | slow   | best     |

Download a model:
```bash
curl -X POST http://localhost:8081/api/whisper/model \
  -H "Content-Type: application/json" \
  -d '{"model": "ggml-small"}'
```

### Proxy support

Set `PROXY_POOL_FILE` to a JSON file (or put the same JSON in `PROXY_POOL`). Precedence: `PROXY_POOL_FILE` → `PROXY_POOL` → no proxy. See `config/proxy-pool.example.json` and `internal/transcribe/proxy_pool.go`.

An HTTP proxy does not fix TLS verification errors from yt-dlp’s Python runtime: HTTPS to YouTube is still checked against your machine’s CA store. If you see `CERTIFICATE_VERIFY_FAILED`, install/link certificates for your Python (e.g. macOS “Install Certificates.command”), or use `YTDLP_NO_CHECK_CERTIFICATES` only as a temporary local workaround.

### Troubleshooting: `transcription_failed` / OpenAI TLS

If the error mentions **`tls: protocol version not supported`** on the call to `https://api.openai.com/v1/audio/transcriptions`, that failure happens inside the **go-whisper** container (clipscript only forwards audio to it). Try:

1. **Refresh the image** — `docker compose pull whisper && docker compose up -d --force-recreate whisper`
2. **Use a local model instead of OpenAI** — In `.env`: set `WHISPER_MODEL=ggml-base.bin` (or another GGML id), **remove or empty `OPENAI_API_KEY`** for the whisper service, then download the model onto the `whisper-models` volume (see model download `curl` in this README). Transcription then stays inside the container with no HTTPS to OpenAI.
3. **Network** — Corporate TLS inspection or restrictive firewalls can break TLS to OpenAI; test from another network or VPN.

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
