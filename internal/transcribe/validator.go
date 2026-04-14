// internal/transcribe/validator.go
package transcribe

import (
	"fmt"
	"regexp"
)

var (
	reInstagramReel  = regexp.MustCompile(`^https://www\.instagram\.com/reel/[\w\-]+/?$`)
	reYouTubeShorts  = regexp.MustCompile(`^https://(www\.)?youtube\.com/shorts/[\w\-]+$`)
)

// ValidateURL returns nil if url is a supported Instagram Reel or YouTube Shorts link.
func ValidateURL(url string) error {
	if reInstagramReel.MatchString(url) || reYouTubeShorts.MatchString(url) {
		return nil
	}
	return fmt.Errorf("URL must be an Instagram Reel or YouTube Shorts link")
}
