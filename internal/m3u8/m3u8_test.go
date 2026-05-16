package m3u8_test

import (
	"strings"
	"testing"

	"github.com/IstarVin/hls-proxy/internal/m3u8"
)

func TestClassify_ContentType(t *testing.T) {
	cases := []struct {
		ct       string
		url      string
		expected m3u8.ContentClass
	}{
		{"application/vnd.apple.mpegurl", "https://cdn.com/stream.m3u8", m3u8.ClassM3U8},
		{"application/x-mpegURL", "https://cdn.com/x", m3u8.ClassM3U8},
		{"video/mp2t", "https://cdn.com/seg.ts", m3u8.ClassTS},
		// The key case: origin sends image/png but URL ends in .ts
		{"image/png", "https://cdn.com/seg001.ts", m3u8.ClassTS},
		// image/png for a non-TS URL => passthrough
		{"image/png", "https://cdn.com/thumb.png", m3u8.ClassPassthrough},
		// No CT, rely on extension
		{"", "https://cdn.com/playlist.m3u8", m3u8.ClassM3U8},
		{"", "https://cdn.com/seg.ts?t=123", m3u8.ClassTS},
		{"", "https://cdn.com/key", m3u8.ClassPassthrough},
	}

	for _, c := range cases {
		got := m3u8.Classify(c.ct, c.url)
		if got != c.expected {
			t.Errorf("Classify(%q, %q) = %v, want %v", c.ct, c.url, got, c.expected)
		}
	}
}

func TestEffectiveContentType_TSMasquerade(t *testing.T) {
	// image/png masquerading as TS must be corrected.
	got := m3u8.EffectiveContentType("image/png", m3u8.ClassTS)
	if got != "video/mp2t" {
		t.Errorf("got %q, want video/mp2t", got)
	}
}

func TestEffectiveContentType_M3U8AlwaysCanonical(t *testing.T) {
	got := m3u8.EffectiveContentType("text/plain", m3u8.ClassM3U8)
	if got != "application/vnd.apple.mpegurl" {
		t.Errorf("got %q", got)
	}
}

func TestEffectiveContentType_Passthrough(t *testing.T) {
	got := m3u8.EffectiveContentType("image/png", m3u8.ClassPassthrough)
	if got != "image/png" {
		t.Errorf("expected passthrough to preserve image/png, got %q", got)
	}
}

func TestRewriteM3U8_RelativeURLs(t *testing.T) {
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXTINF:6.0,
seg001.ts
#EXTINF:6.0,
seg002.ts
#EXT-X-ENDLIST`

	result := m3u8.RewriteM3U8(
		playlist,
		"https://cdn.example.com/hls/index.m3u8",
		"https://proxy.com",
		"", "", "",
	)

	for _, seg := range []string{"seg001.ts", "seg002.ts"} {
		// Must not appear bare anymore.
		if strings.Contains(result, "\n"+seg+"\n") {
			t.Errorf("bare segment %q still present in output", seg)
		}
		// Must appear as an absolute URL routed through the proxy.
		expected := strings.ReplaceAll("https%3A%2F%2Fcdn.example.com%2Fhls%2F"+seg, ".", "%2E")
		if !strings.Contains(result, expected) {
			t.Errorf("rewritten segment %q not found in output:\n%s", seg, result)
		}
	}
}

func TestRewriteM3U8_URIAttribute(t *testing.T) {
	playlist := `#EXTM3U
#EXT-X-KEY:METHOD=AES-128,URI="https://keys.example.com/key"
#EXTINF:6.0,
seg001.ts`

	result := m3u8.RewriteM3U8(
		playlist,
		"https://cdn.example.com/hls/index.m3u8",
		"https://proxy.com",
		"", "", "",
	)

	if strings.Contains(result, `URI="https://keys.example.com/key"`) {
		t.Error("key URI was not rewritten")
	}
	if !strings.Contains(result, `URI="https://proxy.com/proxy?`) || !strings.Contains(result, "url=") {
		t.Errorf("key URI not routed through proxy:\n%s", result)
	}
}

func TestRewriteM3U8_AuthPropagation(t *testing.T) {
	playlist := "#EXTM3U\n#EXTINF:6.0,\nseg001.ts"
	result := m3u8.RewriteM3U8(
		playlist,
		"https://cdn.example.com/hls/index.m3u8",
		"https://proxy.com",
		"https://site.com", "", "mytoken",
	)

	if !strings.Contains(result, "referer=") {
		t.Error("referer not propagated")
	}
	if !strings.Contains(result, "token=") {
		t.Error("token not propagated")
	}
}
