package transcribe

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
)

// Selection strategy for ProxyPool (JSON field "selection").
const (
	SelectionRoundRobin = "round_robin"
	SelectionRandom     = "random"
	SelectionFirst      = "first"
)

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// proxyPoolFile is the JSON shape for PROXY_POOL / PROXY_POOL_FILE.
type proxyPoolFile struct {
	Proxies   []proxyPoolEntry `json:"proxies"`
	Selection string           `json:"selection"`
}

type proxyPoolEntry struct {
	ID       string `json:"id"`
	Scheme   string `json:"scheme"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// ProxyPool picks a yt-dlp proxy URL per request from a fixed list.
type ProxyPool struct {
	urls      []string
	selection string
	source    string // pool_file | pool_inline | pool_embedded
	rr        atomic.Uint64
}

// Source returns how this pool was loaded (for API responses).
func (p *ProxyPool) Source() string { return p.source }

// ProxyURL returns the next proxy URL according to the selection strategy.
func (p *ProxyPool) ProxyURL() string {
	if len(p.urls) == 0 {
		return ""
	}
	switch p.selection {
	case SelectionRandom:
		return p.urls[rand.IntN(len(p.urls))]
	case SelectionRoundRobin:
		i := (p.rr.Add(1) - 1) % uint64(len(p.urls))
		return p.urls[i]
	case SelectionFirst, "":
		return p.urls[0]
	default:
		return p.urls[0]
	}
}

// expandEnvSubst replaces ${VAR} with os.Getenv("VAR").
func expandEnvSubst(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		sub := envVarPattern.FindStringSubmatch(match)
		if len(sub) != 2 {
			return match
		}
		return os.Getenv(sub[1])
	})
}

func buildProxyURL(e proxyPoolEntry) (string, error) {
	scheme := strings.TrimSpace(e.Scheme)
	host := strings.TrimSpace(e.Host)
	if scheme == "" {
		return "", errors.New("proxy entry: scheme is required")
	}
	if host == "" {
		return "", errors.New("proxy entry: host is required")
	}
	if e.Port <= 0 || e.Port > 65535 {
		return "", fmt.Errorf("proxy entry: invalid port %d", e.Port)
	}

	user := expandEnvSubst(strings.TrimSpace(e.Username))
	pass := expandEnvSubst(strings.TrimSpace(e.Password))

	u := &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", host, e.Port),
	}
	if user != "" || pass != "" {
		u.User = url.UserPassword(user, pass)
	}
	return u.String(), nil
}

func parseProxyPoolJSON(raw []byte, source string) (*ProxyPool, error) {
	raw = trimBOM(raw)
	var doc proxyPoolFile
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("proxy pool JSON: %w", err)
	}
	if len(doc.Proxies) == 0 {
		return nil, errors.New("proxy pool: \"proxies\" must be non-empty")
	}

	urls := make([]string, 0, len(doc.Proxies))
	for i, e := range doc.Proxies {
		u, err := buildProxyURL(e)
		if err != nil {
			return nil, fmt.Errorf("proxies[%d]: %w", i, err)
		}
		urls = append(urls, u)
	}

	sel := strings.ToLower(strings.TrimSpace(doc.Selection))
	if sel == "" {
		sel = SelectionFirst
	}
	switch sel {
	case SelectionRoundRobin, SelectionRandom, SelectionFirst:
	default:
		return nil, fmt.Errorf("proxy pool: unknown selection %q (use %q, %q, or %q)",
			doc.Selection, SelectionRoundRobin, SelectionRandom, SelectionFirst)
	}

	return &ProxyPool{urls: urls, selection: sel, source: source}, nil
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

// loadProxyPoolFromEnv reads PROXY_POOL_FILE (first) or PROXY_POOL inline JSON.
func loadProxyPoolFromEnv() (*ProxyPool, error) {
	path := strings.TrimSpace(os.Getenv("PROXY_POOL_FILE"))
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("PROXY_POOL_FILE: %w", err)
		}
		return parseProxyPoolJSON(raw, "pool_file")
	}

	inline := strings.TrimSpace(os.Getenv("PROXY_POOL"))
	if inline != "" {
		return parseProxyPoolJSON([]byte(inline), "pool_inline")
	}

	return nil, nil
}
