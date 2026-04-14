package transcribe

import (
	"fmt"
	"os"
)

// ProxyProvider returns a proxy URL for use with yt-dlp.
// An empty string means no proxy.
type ProxyProvider interface {
	ProxyURL() string
}

// NoProxy disables proxy routing.
type NoProxy struct{}

func (NoProxy) ProxyURL() string { return "" }

// ScraperAPIProxy routes requests through ScraperAPI.
type ScraperAPIProxy struct {
	APIKey string
}

func (s ScraperAPIProxy) ProxyURL() string {
	return fmt.Sprintf("http://scraperapi:%s@proxy-server.scraperapi.com:8001", s.APIKey)
}

// NewProxyProvider reads PROXY_PROVIDER env var and returns the matching provider.
// Add new providers here by implementing ProxyProvider and adding a case.
func NewProxyProvider() ProxyProvider {
	switch os.Getenv("PROXY_PROVIDER") {
	case "scraperapi":
		return ScraperAPIProxy{APIKey: os.Getenv("SCRAPER_API_KEY")}
	default:
		return NoProxy{}
	}
}
