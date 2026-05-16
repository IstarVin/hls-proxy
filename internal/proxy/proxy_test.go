package proxy

import (
	"net/http/httptest"
	"testing"
)

func TestResolveProxyBase_UsesForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "http://internal.local/proxy?url=https://example.com/stream.m3u8", nil)
	req.Host = "hls-proxy.istarvin.uk"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "hls-proxy.istarvin.uk")

	got := resolveProxyBase("", req)
	want := "https://hls-proxy.istarvin.uk"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveProxyBase_ConfiguredValueWins(t *testing.T) {
	req := httptest.NewRequest("GET", "http://internal.local/proxy?url=https://example.com/stream.m3u8", nil)
	req.Host = "hls-proxy.istarvin.uk"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "hls-proxy.istarvin.uk")

	got := resolveProxyBase("https://configured.example.com", req)
	want := "https://configured.example.com"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
