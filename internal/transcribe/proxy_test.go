package transcribe_test

import (
	"testing"

	"clipscript/internal/transcribe"
)

func TestNoProxy(t *testing.T) {
	p := transcribe.NoProxy{}
	if got := p.ProxyURL(); got != "" {
		t.Errorf("NoProxy.ProxyURL() = %q, want empty string", got)
	}
}

func TestNewProxyProvider_NoPool(t *testing.T) {
	t.Setenv("PROXY_POOL", "")
	t.Setenv("PROXY_POOL_FILE", "")
	p := transcribe.NewProxyProvider()
	if p.ProxyURL() != "" {
		t.Errorf("expected empty proxy URL when no pool configured")
	}
}
