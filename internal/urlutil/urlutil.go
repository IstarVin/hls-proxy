package urlutil

import (
	"fmt"
	"net/url"
	"strings"
)

// ProxyParams holds parsed proxy query parameters.
type ProxyParams struct {
	TargetURL string
	Referer   string
	Cookie    string
	Token     string
	Origin    string
}

// ParseProxyQuery parses the raw request URL.
// url= must be the LAST query parameter; everything after "url=" is the raw
// target URL (decoded once) so that target URLs containing "&" survive intact.
func ParseProxyQuery(rawRequestURL string) (*ProxyParams, error) {
	qIdx := strings.IndexByte(rawRequestURL, '?')
	if qIdx == -1 {
		return nil, fmt.Errorf("no query string")
	}

	rawQuery := rawRequestURL[qIdx+1:]

	const urlMarker = "url="
	urlIdx := strings.Index(rawQuery, urlMarker)
	if urlIdx == -1 {
		return nil, fmt.Errorf("missing url= parameter")
	}

	// Parse everything before url= normally.
	beforeURL := rawQuery[:urlIdx]
	normalParams, err := url.ParseQuery(strings.TrimSuffix(beforeURL, "&"))
	if err != nil {
		return nil, fmt.Errorf("parse query params: %w", err)
	}

	// Everything after "url=" is the raw target URL (decode once).
	rawTarget := rawQuery[urlIdx+len(urlMarker):]
	targetURL, err := url.QueryUnescape(rawTarget)
	if err != nil {
		// If it wasn't percent-encoded, use as-is.
		targetURL = rawTarget
	}

	return &ProxyParams{
		TargetURL: targetURL,
		Referer:   normalParams.Get("referer"),
		Cookie:    normalParams.Get("cookie"),
		Token:     normalParams.Get("token"),
		Origin:    normalParams.Get("origin"),
	}, nil
}

// ValidateTargetURL ensures the target is http or https only.
func ValidateTargetURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https URLs are allowed, got %q", u.Scheme)
	}
	return nil
}

// BuildProxyURL builds a proxy URL for a given target, propagating auth params.
// url= is always appended last so ParseProxyQuery can use the raw-split strategy.
func BuildProxyURL(proxyBase, targetURL, referer, cookie, token string) string {
	proxyPath := "/proxy"
	if LooksLikeSegment(targetURL) {
		proxyPath = "/proxy/segment.ts"
	}

	params := url.Values{}
	if referer != "" {
		params.Set("referer", referer)
	}
	if cookie != "" {
		params.Set("cookie", cookie)
	}
	if token != "" {
		params.Set("token", token)
	}

	// Include origin (scheme://host) for the target so handlers can set
	// the Origin header or otherwise make origin-restricted decisions.
	if o := OriginHost(targetURL); o != "" {
		params.Set("origin", o)
	}

	prefix := params.Encode()
	encoded := encodeTargetURL(targetURL)

	if prefix != "" {
		return proxyBase + proxyPath + "?" + prefix + "&url=" + encoded
	}
	return proxyBase + proxyPath + "?url=" + encoded
}

func encodeTargetURL(targetURL string) string {
	return strings.ReplaceAll(url.QueryEscape(targetURL), ".", "%2E")
}

// LooksLikeSegment reports whether a URL is likely an HLS media segment.
func LooksLikeSegment(raw string) bool {
	path := strings.ToLower(strings.SplitN(raw, "?", 2)[0])
	return strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".png")
}

// OriginHost returns the scheme+host of a URL for restricting auth forwarding.
func OriginHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
