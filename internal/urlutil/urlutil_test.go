package urlutil_test

import (
	"strings"
	"testing"

	"github.com/yourname/hls-proxy/internal/urlutil"
)

func TestParseProxyQuery_Basic(t *testing.T) {
	raw := "/proxy?referer=https://site.com&token=tok123&url=https://cdn.example.com/stream.m3u8"
	p, err := urlutil.ParseProxyQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.TargetURL != "https://cdn.example.com/stream.m3u8" {
		t.Errorf("unexpected TargetURL: %q", p.TargetURL)
	}
	if p.Referer != "https://site.com" {
		t.Errorf("unexpected Referer: %q", p.Referer)
	}
	if p.Token != "tok123" {
		t.Errorf("unexpected Token: %q", p.Token)
	}
}

func TestParseProxyQuery_AmpersandInTargetURL(t *testing.T) {
	// url= must survive & characters in the target URL.
	target := "https://cdn.example.com/live/stream.m3u8?token=abc&quality=hd"
	encoded := "https%3A%2F%2Fcdn.example.com%2Flive%2Fstream.m3u8%3Ftoken%3Dabc%26quality%3Dhd"
	raw := "/proxy?referer=https://site.com&url=" + encoded

	p, err := urlutil.ParseProxyQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.TargetURL != target {
		t.Errorf("got %q, want %q", p.TargetURL, target)
	}
}

func TestParseProxyQuery_NoQueryString(t *testing.T) {
	_, err := urlutil.ParseProxyQuery("/proxy")
	if err == nil {
		t.Fatal("expected error for missing query string")
	}
}

func TestParseProxyQuery_MissingURL(t *testing.T) {
	_, err := urlutil.ParseProxyQuery("/proxy?referer=x")
	if err == nil {
		t.Fatal("expected error for missing url= param")
	}
}

func TestValidateTargetURL(t *testing.T) {
	good := []string{
		"http://example.com/path",
		"https://example.com/path?q=1",
	}
	for _, u := range good {
		if err := urlutil.ValidateTargetURL(u); err != nil {
			t.Errorf("unexpected error for %q: %v", u, err)
		}
	}

	bad := []string{
		"file:///etc/passwd",
		"ftp://example.com",
		"data:text/html,hello",
		"javascript:alert(1)",
	}
	for _, u := range bad {
		if err := urlutil.ValidateTargetURL(u); err == nil {
			t.Errorf("expected error for %q", u)
		}
	}
}

func TestBuildProxyURL_URLIsLast(t *testing.T) {
	result := urlutil.BuildProxyURL("https://proxy.com", "https://cdn.com/seg.ts", "https://ref.com", "", "tok")
	// url= must be the last parameter.
	urlIdx := strings.Index(result, "url=")
	if urlIdx == -1 {
		t.Fatal("url= not found in result")
	}
	afterURL := result[urlIdx:]
	if strings.Contains(afterURL[4:], "referer=") || strings.Contains(afterURL[4:], "token=") {
		t.Errorf("url= is not the last parameter: %s", result)
	}
}
