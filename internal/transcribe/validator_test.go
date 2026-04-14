package transcribe_test

import (
	"testing"

	"clipscript/internal/transcribe"
)

func TestValidateURL(t *testing.T) {
	valid := []string{
		// Instagram Reels — standard
		"https://www.instagram.com/reel/ABC123/",
		"https://www.instagram.com/reel/Xy_-Z9/",
		// Instagram Reels — with query string (real share links)
		"https://www.instagram.com/reel/ABC123/?igshid=NTc4MTIwNjQ2YQ==",
		// Instagram Reels — username-prefixed path
		"https://www.instagram.com/thedigital.indian/reel/ABC123/",
		// Instagram Reels — share link format
		"https://www.instagram.com/share/reel/ABC123/",
		// YouTube Shorts — with www
		"https://www.youtube.com/shorts/dQw4w9WgXcQ",
		// YouTube Shorts — without www
		"https://youtube.com/shorts/dQw4w9WgXcQ",
		// YouTube Shorts — with query string (real share links)
		"https://www.youtube.com/shorts/dQw4w9WgXcQ?feature=share",
	}
	invalid := []string{
		"",
		"not-a-url",
		"https://www.instagram.com/p/ABC123/",          // post, not reel
		"https://www.youtube.com/watch?v=abc",           // regular video
		"https://www.tiktok.com/@user/video/1",          // unsupported platform
		"http://evil.com/reel/ABC123/",                  // wrong domain
		"https://www.facebook.com/reel/123456",          // wrong platform
	}

	for _, u := range valid {
		if err := transcribe.ValidateURL(u); err != nil {
			t.Errorf("expected valid, got error for %q: %v", u, err)
		}
	}
	for _, u := range invalid {
		if err := transcribe.ValidateURL(u); err == nil {
			t.Errorf("expected error, got nil for %q", u)
		}
	}
}
