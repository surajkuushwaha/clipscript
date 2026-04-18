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

// ParseURL returns ("ig"|"yt", shortcode, nil) for a supported Instagram Reel or YouTube Shorts URL.
// Query strings and fragments are stripped before matching.
func ParseURL(rawURL string) (platform, shortcode string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("URL must be an Instagram Reel or YouTube Shorts link")
	}

	pathURL := u.Scheme + "://" + u.Host + u.Path

	if m := reInstagramReel.FindStringSubmatch(pathURL); len(m) >= 2 {
		return "ig", m[1], nil
	}
	if m := reInstagramShare.FindStringSubmatch(pathURL); len(m) >= 2 {
		return "ig", m[1], nil
	}
	if m := reYouTubeShorts.FindStringSubmatch(pathURL); len(m) >= 2 {
		return "yt", m[1], nil
	}
	return "", "", fmt.Errorf("URL must be an Instagram Reel or YouTube Shorts link")
}

// ValidateURL returns nil if url is a supported Instagram Reel or YouTube Shorts link.
// Query strings and fragments are stripped before matching.
func ValidateURL(rawURL string) error {
	_, _, err := ParseURL(rawURL)
	return err
}
