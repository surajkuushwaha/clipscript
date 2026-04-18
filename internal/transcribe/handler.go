package transcribe

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Handler holds config needed to serve transcription requests.
type Handler struct {
	GoWhisperURL   string
	WhisperModel   string
	OpenAIKey      string
	RequestTimeout time.Duration
	Proxy          ProxyProvider
	Cache          *Cache
	// Injectable for testing
	downloadFn   func(ctx context.Context, url, proxyURL, destPath string) error
	transcribeFn func(ctx context.Context, goWhisperURL, model, audioPath, format, language, openAIKey string) (interface{}, float64, error)
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
		model = "ggml-base.bin"
	}
	cache := NewCache()
	if err := cache.EnsureDirs(); err != nil {
		log.Printf("clipscript: cache EnsureDirs: %v", err)
	}
	return &Handler{
		GoWhisperURL:   goWhisperURL,
		WhisperModel:   model,
		OpenAIKey:      os.Getenv("OPENAI_API_KEY"),
		RequestTimeout: time.Duration(timeout) * time.Second,
		Proxy:          NewProxyProvider(),
		Cache:          cache,
		downloadFn: func(ctx context.Context, url, proxyURL, destPath string) error {
			return DownloadAudioTo(ctx, url, proxyURL, destPath)
		},
		transcribeFn: TranscribeAudio,
	}
}

// SetDownloadFn overrides the download function (for testing).
func (h *Handler) SetDownloadFn(fn func(ctx context.Context, url, proxyURL, destPath string) error) {
	h.downloadFn = fn
}

// SetTranscribeFn overrides the transcribe function (for testing).
func (h *Handler) SetTranscribeFn(fn func(ctx context.Context, goWhisperURL, model, audioPath, format, language, openAIKey string) (interface{}, float64, error)) {
	h.transcribeFn = fn
}

// isTimeout reports whether err is a context deadline or cancellation error.
func isTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func tempMP3Path() (string, error) {
	tmpFile, err := os.CreateTemp("", "clipscript-*.mp3")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	_ = os.Remove(tmpPath)
	return strings.TrimSuffix(tmpPath, filepath.Ext(tmpPath)) + ".mp3", nil
}

func (h *Handler) transcribeOne(ctx context.Context, reqURL, format, language string) TranscribeItemResult {
	item := TranscribeItemResult{URL: reqURL, Ok: false}

	if err := ValidateURL(reqURL); err != nil {
		item.Error = "invalid_url"
		item.Message = err.Error()
		return item
	}

	platform, shortcode, err := ParseURL(reqURL)
	if err != nil {
		item.Error = "invalid_url"
		item.Message = err.Error()
		return item
	}

	cache := h.Cache
	if cache == nil {
		cache = NewCache()
	}

	usedProxy := h.Proxy.ProxyURL()

	if ct, hit, err := cache.ReadTranscript(platform, shortcode, language, format); err != nil {
		item.Error = "internal_error"
		item.Message = err.Error()
		return item
	} else if hit {
		item.Ok = true
		item.Cached = "transcript"
		item.DurationSeconds = ct.Duration
		if format == "segments" {
			item.Segments = ct.Segments
		} else {
			item.Transcript = ct.Text
		}
		return item
	}

	var audioPath string
	var removeAfter bool
	cachedTag := "none"

	if p, ok := cache.HasAudio(platform, shortcode); ok {
		audioPath = p
		cachedTag = "audio"
	} else {
		if cache.Enabled {
			audioPath = cache.AudioPath(platform, shortcode)
			removeAfter = false
		} else {
			ap, err := tempMP3Path()
			if err != nil {
				item.Error = "internal_error"
				item.Message = err.Error()
				return item
			}
			audioPath = ap
			removeAfter = true
		}
		if err := h.downloadFn(ctx, reqURL, usedProxy, audioPath); err != nil {
			if removeAfter {
				_ = os.Remove(audioPath)
			}
			if isTimeout(err) {
				item.Error = "timeout"
				item.Message = "processing exceeded timeout limit"
				return item
			}
			item.Error = "download_failed"
			item.Message = err.Error()
			return item
		}
	}

	if removeAfter {
		defer func() { _ = os.Remove(audioPath) }()
	}

	result, duration, err := h.transcribeFn(ctx, h.GoWhisperURL, h.WhisperModel, audioPath, format, language, h.OpenAIKey)
	if err != nil {
		if isTimeout(err) {
			item.Error = "timeout"
			item.Message = "processing exceeded timeout limit"
			return item
		}
		item.Error = "transcription_failed"
		item.Message = err.Error()
		return item
	}

	if format == "segments" {
		segs, ok := result.([]Segment)
		if !ok {
			item.Error = "internal_error"
			item.Message = "unexpected result type from transcription"
			return item
		}
		if err := cache.WriteTranscript(platform, shortcode, language, format, segs, duration); err != nil {
			log.Printf("clipscript: cache write transcript: %v", err)
		}
		item.Ok = true
		item.Segments = segs
		item.DurationSeconds = duration
		item.Cached = cachedTag
		return item
	}

	text, ok := result.(string)
	if !ok {
		item.Error = "internal_error"
		item.Message = "unexpected result type from transcription"
		return item
	}
	if err := cache.WriteTranscript(platform, shortcode, language, format, text, duration); err != nil {
		log.Printf("clipscript: cache write transcript: %v", err)
	}
	item.Ok = true
	item.Transcript = text
	item.DurationSeconds = duration
	item.Cached = cachedTag
	return item
}

// Transcribe handles POST /v1/transcribe (batch: `urls` array only).
func (h *Handler) Transcribe(c *fiber.Ctx) error {
	body := c.Body()
	if len(strings.TrimSpace(string(body))) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "request body must be non-empty JSON",
		})
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "request body must be valid JSON",
		})
	}
	if _, hasURL := raw["url"]; hasURL {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "deprecated_field",
			Message: "use 'urls' array instead of 'url'",
		})
	}

	var req TranscribeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "request body must be valid JSON",
		})
	}

	if len(req.URLs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "urls must be a non-empty array",
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

	ctx, cancel := context.WithTimeout(context.Background(), h.RequestTimeout)
	defer cancel()

	usedProxy := h.Proxy.ProxyURL()
	px := describeProxy(h.Proxy, usedProxy)

	results := make([]TranscribeItemResult, 0, len(req.URLs))
	for _, u := range req.URLs {
		results = append(results, h.transcribeOne(ctx, u, req.Format, req.Language))
	}

	return c.JSON(TranscribeBatchResponse{
		Results: results,
		Proxy:   px,
	})
}
