package transcribe

import (
	"testing"
)

func TestDescribeProxy_NotUsed(t *testing.T) {
	p := NoProxy{}
	info := describeProxy(p, "")
	if info.Status != ProxyStatusNotUsed || info.Used {
		t.Fatalf("describeProxy none: %+v", info)
	}
	if info.Source != "none" {
		t.Errorf("source = %q", info.Source)
	}
}

func TestDescribeProxy_InUse(t *testing.T) {
	p := NoProxy{}
	info := describeProxy(p, "http://u:p@192.168.1.1:8080")
	if info.Status != ProxyStatusInUse || !info.Used {
		t.Fatalf("describeProxy in use: %+v", info)
	}
	if info.Endpoint != "192.168.1.1:8080" {
		t.Errorf("endpoint = %q", info.Endpoint)
	}
}
