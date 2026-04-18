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
	_ = os.Remove(tmpPath)

	finalPath := strings.TrimSuffix(tmpPath, filepath.Ext(tmpPath)) + ".mp3"
	if err := DownloadAudioTo(ctx, url, proxyURL, finalPath); err != nil {
		return "", err
	}
	return finalPath, nil
}

// DownloadAudioTo downloads audio-only from url to outPath as mp3.
// Writes to a .part file first, then renames to outPath. Caller supplies the final path.
// proxyURL may be empty for no proxy.
func DownloadAudioTo(ctx context.Context, url, proxyURL, outPath string) error {
	stem := strings.TrimSuffix(outPath, filepath.Ext(outPath))
	partPattern := stem + ".part.%(ext)s"
	partFinal := stem + ".part.mp3"

	dl := ytdlp.New().
		ExtractAudio().
		AudioFormat("mp3").
		Output(partPattern)

	if proxyURL != "" {
		dl = dl.Proxy(proxyURL)
	}

	if truthyEnv("YTDLP_NO_CHECK_CERTIFICATES") {
		dl = dl.NoCheckCertificates()
	}

	if _, err := dl.Run(ctx, url); err != nil {
		_ = os.Remove(partFinal)
		return fmt.Errorf("yt-dlp: %w", err)
	}

	if _, err := os.Stat(partFinal); err != nil {
		_ = os.Remove(partFinal)
		return fmt.Errorf("audio file not found after download: %w", err)
	}

	_ = os.Remove(outPath) // replace existing
	if err := os.Rename(partFinal, outPath); err != nil {
		_ = os.Remove(partFinal)
		return fmt.Errorf("rename audio file: %w", err)
	}
	return nil
}

func truthyEnv(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}
