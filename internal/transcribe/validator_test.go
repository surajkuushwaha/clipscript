// internal/transcribe/validator_test.go
package transcribe_test

import (
	"testing"
	"clipscript/internal/transcribe"
)

func TestValidateURL(t *testing.T) {
	valid := []string{
		"https://www.instagram.com/reel/ABC123/",
		"https://www.instagram.com/reel/Xy_-Z9/",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ",
		"https://youtube.com/shorts/dQw4w9WgXcQ",
	}
	invalid := []string{
		"",
		"not-a-url",
		"https://www.instagram.com/p/ABC123/",   // post, not reel
		"https://www.youtube.com/watch?v=abc",   // regular video
		"https://www.tiktok.com/@user/video/1",  // unsupported platform
		"http://evil.com/reel/ABC123/",          // wrong domain
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
