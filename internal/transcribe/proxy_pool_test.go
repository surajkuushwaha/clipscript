package transcribe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProxyPoolJSON_First(t *testing.T) {
	raw := `{
		"proxies": [
			{"scheme": "http", "host": "a.example.com", "port": 8001, "username": "u", "password": "p"},
			{"scheme": "socks5", "host": "b.example.com", "port": 1080}
		],
		"selection": "first"
	}`
	pool, err := parseProxyPoolJSON([]byte(raw), "test")
	if err != nil {
		t.Fatal(err)
	}
	want := "http://u:p@a.example.com:8001"
	if got := pool.ProxyURL(); got != want {
		t.Errorf("ProxyURL() = %q, want %q", got, want)
	}
	if got := pool.ProxyURL(); got != want {
		t.Errorf("second call = %q, want %q", got, want)
	}
}

func TestParseProxyPoolJSON_RoundRobin(t *testing.T) {
	raw := `{
		"proxies": [
			{"scheme": "http", "host": "a.example.com", "port": 80},
			{"scheme": "http", "host": "b.example.com", "port": 80}
		],
		"selection": "round_robin"
	}`
	pool, err := parseProxyPoolJSON([]byte(raw), "test")
	if err != nil {
		t.Fatal(err)
	}
	u1 := pool.ProxyURL()
	u2 := pool.ProxyURL()
	u3 := pool.ProxyURL()
	if u1 == u2 {
		t.Errorf("round_robin: expected alternating URLs, got %q twice", u1)
	}
	if u1 != u3 {
		t.Errorf("third call should match first in rr of 2: %q vs %q", u1, u3)
	}
}

func TestParseProxyPoolJSON_EnvSubst(t *testing.T) {
	t.Setenv("MY_SECRET", "secret123")
	raw := `{
		"proxies": [
			{"scheme": "http", "host": "p.example.com", "port": 8001, "username": "u", "password": "${MY_SECRET}"}
		]
	}`
	pool, err := parseProxyPoolJSON([]byte(raw), "test")
	if err != nil {
		t.Fatal(err)
	}
	if got := pool.ProxyURL(); !strings.Contains(got, "secret123") {
		t.Errorf("expected expanded password, got %q", got)
	}
}

func TestNewProxyProvider_ProxyPoolEnvPrecedence(t *testing.T) {
	t.Setenv("PROXY_POOL_FILE", "")
	t.Setenv("USE_EMBEDDED_PROXY_POOL", "")
	t.Setenv("PROXY_POOL", `{"proxies":[{"scheme":"http","host":"x.test","port":80}],"selection":"first"}`)
	t.Setenv("PROXY_PROVIDER", "scraperapi")
	t.Setenv("SCRAPER_API_KEY", "ignored")
	p := NewProxyProvider()
	if got := p.ProxyURL(); got != "http://x.test:80" {
		t.Errorf("ProxyURL() = %q, want pool entry", got)
	}
}

func TestNewProxyProvider_ProxyPoolFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pool.json")
	if err := os.WriteFile(path, []byte(`{"proxies":[{"scheme":"https","host":"h.test","port":443}],"selection":"first"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROXY_POOL", "")
	t.Setenv("USE_EMBEDDED_PROXY_POOL", "")
	t.Setenv("PROXY_POOL_FILE", path)
	p := NewProxyProvider()
	if got := p.ProxyURL(); got != "https://h.test:443" {
		t.Errorf("ProxyURL() = %q", got)
	}
}

func TestParseProxyPoolJSON_InvalidSelection(t *testing.T) {
	raw := `{"proxies":[{"scheme":"http","host":"a","port":1}],"selection":"nope"}`
	_, err := parseProxyPoolJSON([]byte(raw), "test")
	if err == nil {
		t.Fatal("expected error")
	}
}

