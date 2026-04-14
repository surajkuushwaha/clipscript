package transcribe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
