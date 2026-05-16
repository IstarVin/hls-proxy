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

	prefix := params.Encode()
	encoded := url.QueryEscape(targetURL)

	if prefix != "" {
		return proxyBase + "/proxy?" + prefix + "&url=" + encoded
	}
	return proxyBase + "/proxy?url=" + encoded
}

// OriginHost returns the scheme+host of a URL for restricting auth forwarding.
func OriginHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
