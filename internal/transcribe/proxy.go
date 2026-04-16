package transcribe

import (
	"log"
)

// ProxyProvider returns a proxy URL for use with yt-dlp.
// An empty string means no proxy.
type ProxyProvider interface {
	ProxyURL() string
	// Source identifies how the proxy was configured (for API responses, not secrets).
	Source() string
}

// NoProxy disables proxy routing.
type NoProxy struct{}

func (NoProxy) ProxyURL() string { return "" }

func (NoProxy) Source() string { return "none" }

// NewProxyProvider loads proxy config from the environment.
// Precedence: PROXY_POOL_FILE → PROXY_POOL → no proxy.
func NewProxyProvider() ProxyProvider {
	if pool, err := loadProxyPoolFromEnv(); err != nil {
		log.Printf("clipscript: proxy pool: %v", err)
		return NoProxy{}
	} else if pool != nil {
		return pool
	}
	return NoProxy{}
}
