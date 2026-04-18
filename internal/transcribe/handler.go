package transcribe

import (
	"context"
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

	platform, shortcode, err := ParseURL(req.URL)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_url",
			Message: err.Error(),
		})
	}

	cache := h.Cache
	if cache == nil {
		cache = NewCache()
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.RequestTimeout)
	defer cancel()

	usedProxy := h.Proxy.ProxyURL()
	px := describeProxy(h.Proxy, usedProxy)

	if ct, hit, err := cache.ReadTranscript(platform, shortcode, req.Language, req.Format); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
			Proxy:   &px,
		})
	} else if hit {
		if req.Format == "segments" {
			return c.JSON(fiber.Map{
				"segments":         ct.Segments,
				"duration_seconds": ct.Duration,
				"proxy":            px,
				"cached":           "transcript",
			})
		}
		return c.JSON(fiber.Map{
			"transcript":       ct.Text,
			"duration_seconds": ct.Duration,
			"proxy":            px,
			"cached":           "transcript",
		})
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
				return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
					Error:   "internal_error",
					Message: err.Error(),
					Proxy:   &px,
				})
			}
			audioPath = ap
			removeAfter = true
		}
		if err := h.downloadFn(ctx, req.URL, usedProxy, audioPath); err != nil {
			if removeAfter {
				_ = os.Remove(audioPath)
			}
			if isTimeout(err) {
				return c.Status(fiber.StatusRequestTimeout).JSON(ErrorResponse{
					Error:   "timeout",
					Message: "processing exceeded timeout limit",
					Proxy:   &px,
				})
			}
			return c.Status(fiber.StatusUnprocessableEntity).JSON(ErrorResponse{
				Error:   "download_failed",
				Message: err.Error(),
				Proxy:   &px,
			})
		}
	}

	if removeAfter {
		defer func() { _ = os.Remove(audioPath) }()
	}

	result, duration, err := h.transcribeFn(ctx, h.GoWhisperURL, h.WhisperModel, audioPath, req.Format, req.Language, h.OpenAIKey)
	if err != nil {
		if isTimeout(err) {
			return c.Status(fiber.StatusRequestTimeout).JSON(ErrorResponse{
				Error:   "timeout",
				Message: "processing exceeded timeout limit",
				Proxy:   &px,
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "transcription_failed",
			Message: err.Error(),
			Proxy:   &px,
		})
	}

	if req.Format == "segments" {
		segs, ok := result.([]Segment)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "internal_error",
				Message: "unexpected result type from transcription",
				Proxy:   &px,
			})
		}
		if err := cache.WriteTranscript(platform, shortcode, req.Language, req.Format, segs, duration); err != nil {
			log.Printf("clipscript: cache write transcript: %v", err)
		}
		return c.JSON(fiber.Map{
			"segments":         segs,
			"duration_seconds": duration,
			"proxy":            px,
			"cached":           cachedTag,
		})
	}

	text, ok := result.(string)
	if !ok {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "internal_error",
			Message: "unexpected result type from transcription",
			Proxy:   &px,
		})
	}
	if err := cache.WriteTranscript(platform, shortcode, req.Language, req.Format, text, duration); err != nil {
		log.Printf("clipscript: cache write transcript: %v", err)
	}
	return c.JSON(fiber.Map{
		"transcript":        text,
		"duration_seconds": duration,
		"proxy":             px,
		"cached":            cachedTag,
	})
}
