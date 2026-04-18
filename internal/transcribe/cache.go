package transcribe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Cache stores downloaded audio and transcripts on disk when CACHE_ENABLED is true.
type Cache struct {
	Dir     string
	Enabled bool
}

// CachedTranscript is a transcript entry read from the cache file.
type CachedTranscript struct {
	Format   string
	Text     string
	Segments []Segment
	Duration float64
}

type cachedTranscriptFile struct {
	Format          string    `json:"format"`
	Transcript      string    `json:"transcript,omitempty"`
	Segments        []Segment `json:"segments,omitempty"`
	DurationSeconds float64   `json:"duration_seconds"`
}

// NewCache reads CACHE_ENABLED (default false) and CACHE_DIR (default ./cache).
func NewCache() *Cache {
	dir := strings.TrimSpace(os.Getenv("CACHE_DIR"))
	if dir == "" {
		dir = "./cache"
	}
	return &Cache{Dir: dir, Enabled: truthyEnv("CACHE_ENABLED")}
}

// EnsureDirs creates audio/ and transcripts/ under Dir when caching is enabled.
func (c *Cache) EnsureDirs() error {
	if !c.Enabled {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(c.Dir, "audio"), 0o755); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(c.Dir, "transcripts"), 0o755)
}

// AudioPath returns the canonical path for cached mp3 audio.
func (c *Cache) AudioPath(platform, shortcode string) string {
	return filepath.Join(c.Dir, "audio", fmt.Sprintf("%s_%s.mp3", platform, shortcode))
}

// TranscriptPath returns the path for a cached transcript JSON file.
func (c *Cache) TranscriptPath(platform, shortcode, lang, format string) string {
	name := fmt.Sprintf("%s_%s__%s__%s.json", platform, shortcode, sanitizeLang(lang), format)
	return filepath.Join(c.Dir, "transcripts", name)
}

// HasAudio reports whether a non-empty cached audio file exists.
func (c *Cache) HasAudio(platform, shortcode string) (path string, ok bool) {
	if !c.Enabled {
		return "", false
	}
	p := c.AudioPath(platform, shortcode)
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() || fi.Size() == 0 {
		return "", false
	}
	return p, true
}

// ReadTranscript loads a cached transcript when present and valid for the requested format.
func (c *Cache) ReadTranscript(platform, shortcode, lang, format string) (*CachedTranscript, bool, error) {
	if !c.Enabled {
		return nil, false, nil
	}
	path := c.TranscriptPath(platform, shortcode, lang, format)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var f cachedTranscriptFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, false, nil
	}
	if f.Format != format {
		return nil, false, nil
	}
	ct := &CachedTranscript{Format: f.Format, Duration: f.DurationSeconds}
	switch format {
	case "text":
		ct.Text = f.Transcript
	case "segments":
		ct.Segments = f.Segments
	default:
		return nil, false, nil
	}
	return ct, true, nil
}

// WriteTranscript writes transcript JSON atomically (tmp + rename). No-op when caching is disabled.
func (c *Cache) WriteTranscript(platform, shortcode, lang, format string, result interface{}, duration float64) error {
	if !c.Enabled {
		return nil
	}
	path := c.TranscriptPath(platform, shortcode, lang, format)
	f := cachedTranscriptFile{Format: format, DurationSeconds: duration}
	switch format {
	case "text":
		s, ok := result.(string)
		if !ok {
			return fmt.Errorf("cache: expected string for text format")
		}
		f.Transcript = s
	case "segments":
		segs, ok := result.([]Segment)
		if !ok {
			return fmt.Errorf("cache: expected []Segment for segments format")
		}
		f.Segments = segs
	default:
		return fmt.Errorf("cache: unknown format %q", format)
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func sanitizeLang(lang string) string {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return "auto"
	}
	repl := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	return repl.Replace(lang)
}
