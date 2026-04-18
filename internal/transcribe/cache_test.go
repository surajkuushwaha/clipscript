package transcribe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCache_TranscriptRoundTrip_Text(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("CACHE_DIR", dir)
	c := NewCache()
	if err := c.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteTranscript("yt", "abc", "", "text", "hello world", 1.25); err != nil {
		t.Fatal(err)
	}
	path := c.TranscriptPath("yt", "abc", "", "text")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	ct, hit, err := c.ReadTranscript("yt", "abc", "", "text")
	if err != nil || !hit {
		t.Fatalf("ReadTranscript: hit=%v err=%v", hit, err)
	}
	if ct.Text != "hello world" || ct.Duration != 1.25 || ct.Format != "text" {
		t.Fatalf("unexpected: %+v", ct)
	}
}

func TestCache_TranscriptRoundTrip_Segments(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("CACHE_DIR", dir)
	c := NewCache()
	if err := c.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	segs := []Segment{{Start: 0, End: 1, Text: "a"}}
	if err := c.WriteTranscript("ig", "x1", "hi", "segments", segs, 2.0); err != nil {
		t.Fatal(err)
	}
	wantName := filepath.Join(dir, "transcripts", "ig_x1__hi__segments.json")
	if _, err := os.Stat(wantName); err != nil {
		t.Fatal(err)
	}
	ct, hit, err := c.ReadTranscript("ig", "x1", "hi", "segments")
	if err != nil || !hit {
		t.Fatalf("ReadTranscript: hit=%v err=%v", hit, err)
	}
	if len(ct.Segments) != 1 || ct.Segments[0].Text != "a" || ct.Duration != 2.0 {
		t.Fatalf("unexpected: %+v", ct)
	}
}

func TestCache_TranscriptPath_EmptyLanguageUsesAuto(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("CACHE_DIR", dir)
	c := NewCache()
	p := c.TranscriptPath("yt", "z", "", "text")
	if want := filepath.Join(dir, "transcripts", "yt_z__auto__text.json"); p != want {
		t.Fatalf("got %q want %q", p, want)
	}
}

func TestCache_DisabledNoReadWrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CACHE_ENABLED", "false")
	t.Setenv("CACHE_DIR", dir)
	c := NewCache()
	if _, ok := c.HasAudio("yt", "x"); ok {
		t.Fatal("expected no audio")
	}
	ct, hit, err := c.ReadTranscript("yt", "x", "", "text")
	if err != nil || hit || ct != nil {
		t.Fatalf("ReadTranscript disabled: ct=%v hit=%v err=%v", ct, hit, err)
	}
	if err := c.WriteTranscript("yt", "x", "", "text", "nope", 1); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no cache files when disabled, got %d entries", len(entries))
	}
}
