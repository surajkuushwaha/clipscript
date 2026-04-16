package transcribe

import (
	"net/url"
)

// Proxy status values for JSON (proxy.status).
const (
	ProxyStatusNotUsed = "not_used"
	ProxyStatusInUse   = "in_use"
)

// ProxyInfo is safe for JSON responses (no credentials).
type ProxyInfo struct {
	Used          bool   `json:"used"`
	Status        string `json:"status"` // "in_use" | "not_used"
	Source        string `json:"source"` // none | scraperapi | pool_file | pool_inline | pool_embedded
	Endpoint      string `json:"endpoint,omitempty"`
	PoolSelection string `json:"pool_selection,omitempty"`
}

func describeProxy(p ProxyProvider, usedProxyURL string) ProxyInfo {
	used := usedProxyURL != ""
	st := ProxyStatusNotUsed
	if used {
		st = ProxyStatusInUse
	}
	info := ProxyInfo{
		Used:     used,
		Status:   st,
		Source:   p.Source(),
		Endpoint: redactProxyHostPort(usedProxyURL),
	}
	if pool, ok := p.(*ProxyPool); ok {
		info.PoolSelection = pool.selection
	}
	return info
}

func redactProxyHostPort(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}
