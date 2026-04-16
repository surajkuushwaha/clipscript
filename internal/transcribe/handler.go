package transcribe

import (
	"context"
	"errors"
	"os"
	"strconv"
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
	// Injectable for testing
	downloadFn   func(ctx context.Context, url, proxyURL string) (string, error)
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
	return &Handler{
		GoWhisperURL:   goWhisperURL,
		WhisperModel:   model,
		OpenAIKey:      os.Getenv("OPENAI_API_KEY"),
		RequestTimeout: time.Duration(timeout) * time.Second,
		Proxy:          NewProxyProvider(),
		downloadFn:     DownloadAudio,
		transcribeFn:   TranscribeAudio,
	}
}

// SetDownloadFn overrides the download function (for testing).
func (h *Handler) SetDownloadFn(fn func(ctx context.Context, url, proxyURL string) (string, error)) {
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

	usedProxy := h.Proxy.ProxyURL()
	px := describeProxy(h.Proxy, usedProxy)

	audioPath, err := h.downloadFn(ctx, req.URL, usedProxy)
	if err != nil {
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
	defer func() { _ = os.Remove(audioPath) }()

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
		return c.JSON(fiber.Map{
			"segments":         segs,
			"duration_seconds": duration,
			"proxy":            px,
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
	// fiber.Map keeps "proxy" in the JSON payload reliably across encoders / clients.
	return c.JSON(fiber.Map{
		"transcript":        text,
		"duration_seconds": duration,
		"proxy":            px,
	})
}
