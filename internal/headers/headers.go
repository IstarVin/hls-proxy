// Package headers builds outbound and inbound HTTP headers for the proxy.
package headers

import "net/http"

var forwardFromPlayer = []string{
	"User-Agent",
	"Accept",
	"Accept-Language",
	"Range",
}

var forwardFromOrigin = []string{
	"Cache-Control",
	"Expires",
	"Last-Modified",
	"Etag",
	"Accept-Ranges",
}

// BuildOutbound constructs headers to send to the origin server.
// BuildOutbound constructs headers to send to the origin server.
// If origin is non-empty it will be set as the `Origin` header.
func BuildOutbound(playerHeaders http.Header, referer, cookie, token, origin string) http.Header {
	h := make(http.Header)
	for _, name := range forwardFromPlayer {
		if v := playerHeaders.Get(name); v != "" {
			h.Set(name, v)
		}
	}
	if referer != "" {
		h.Set("Origin", referer)
		h.Set("Referer", referer)
	}
	if cookie != "" {
		h.Set("Cookie", cookie)
	}
	if token != "" {
		h.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		h.Set("Origin", origin)
	}

	return h
}

// BuildResponse constructs response headers to send back to the player.
// contentType is the already-corrected MIME type (never passes image/png for TS).
// Content-Length is intentionally dropped because stripping the PNG header
// makes the body shorter than what the origin advertised.
func BuildResponse(originHeaders http.Header, contentType string) http.Header {
	h := make(http.Header)
	h.Set("Content-Type", contentType)
	for _, name := range forwardFromOrigin {
		if v := originHeaders.Get(name); v != "" {
			h.Set(name, v)
		}
	}
	setCORS(h)
	return h
}

// BuildCORSPreflight returns headers for OPTIONS preflight responses.
func BuildCORSPreflight() http.Header {
	h := make(http.Header)
	setCORS(h)
	return h
}

func setCORS(h http.Header) {
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Range, Authorization")
	h.Set("Access-Control-Expose-Headers", "Content-Length")
}
