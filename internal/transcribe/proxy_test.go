package transcribe_test

import (
	"os"
	"testing"
	"clipscript/internal/transcribe"
)

func TestNoProxy(t *testing.T) {
	p := transcribe.NoProxy{}
	if got := p.ProxyURL(); got != "" {
		t.Errorf("NoProxy.ProxyURL() = %q, want empty string", got)
	}
}

func TestScraperAPIProxy(t *testing.T) {
	p := transcribe.ScraperAPIProxy{APIKey: "testkey123"}
	got := p.ProxyURL()
	want := "http://scraperapi:testkey123@proxy-server.scraperapi.com:8001"
	if got != want {
		t.Errorf("ScraperAPIProxy.ProxyURL() = %q, want %q", got, want)
	}
}

func TestNewProxyProvider_None(t *testing.T) {
	os.Setenv("PROXY_PROVIDER", "none")
	p := transcribe.NewProxyProvider()
	if p.ProxyURL() != "" {
		t.Errorf("expected empty proxy URL for provider=none")
	}
}

func TestNewProxyProvider_ScraperAPI(t *testing.T) {
	os.Setenv("PROXY_PROVIDER", "scraperapi")
	os.Setenv("SCRAPER_API_KEY", "mykey")
	p := transcribe.NewProxyProvider()
	if p.ProxyURL() == "" {
		t.Errorf("expected non-empty proxy URL for provider=scraperapi")
	}
}
