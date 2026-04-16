package transcribe

import (
	"fmt"
	"net/url"
	"regexp"
)

var (
	// Instagram Reel: /reel/ID, /reels/ID, /username/reel/ID, /share/reel/ID
	reInstagramReel = regexp.MustCompile(`(?i)instagram\.com/(?:[a-zA-Z0-9_.\-]+/)?(?:reel|reels)/([a-zA-Z0-9_\-]+)`)
	// Instagram share link format
	reInstagramShare = regexp.MustCompile(`(?i)instagram\.com/share/reel/([a-zA-Z0-9_\-]+)`)
	// YouTube Shorts: youtube.com/shorts/ID (with or without www)
	reYouTubeShorts = regexp.MustCompile(`(?i)(?:www\.)?youtube\.com/shorts/([a-zA-Z0-9_\-]+)`)
)

// ValidateURL returns nil if url is a supported Instagram Reel or YouTube Shorts link.
// Query strings and fragments are stripped before matching.
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("URL must be an Instagram Reel or YouTube Shorts link")
	}

	// Match against path only (query string stripped)
	pathURL := u.Scheme + "://" + u.Host + u.Path

	if reInstagramReel.MatchString(pathURL) ||
		reInstagramShare.MatchString(pathURL) ||
		reYouTubeShorts.MatchString(pathURL) {
		return nil
	}
	return fmt.Errorf("URL must be an Instagram Reel or YouTube Shorts link")
}
