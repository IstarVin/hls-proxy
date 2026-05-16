// Package proxy implements the /proxy HTTP handler.
package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/IstarVin/hls-proxy/internal/headers"
	"github.com/IstarVin/hls-proxy/internal/m3u8"
	"github.com/IstarVin/hls-proxy/internal/strip"
	"github.com/IstarVin/hls-proxy/internal/urlutil"
)

var timeouts = map[m3u8.ContentClass]time.Duration{
	m3u8.ClassM3U8:        10 * time.Second,
	m3u8.ClassTS:          30 * time.Second,
	m3u8.ClassPassthrough: 15 * time.Second,
}

// Handler returns an http.Handler for the /proxy route.
func Handler(proxyBase string) http.Handler {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			writeHeaders(w.Header(), headers.BuildCORSPreflight())
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeHeaders(w.Header(), headers.BuildCORSPreflight())
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse proxy query params.
		params, err := urlutil.ParseProxyQuery(r.URL.RequestURI())
		if err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Validate target URL scheme.
		if err := urlutil.ValidateTargetURL(params.TargetURL); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Determine content class from the URL first so we can set the right
		// timeout before even making the request.
		preClass := m3u8.Classify("", params.TargetURL)
		timeout := timeouts[preClass]

		// Build outbound request.
		req, err := http.NewRequestWithContext(r.Context(), r.Method, params.TargetURL, nil)
		if err != nil {
			http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
			return
		}
		outbound := headers.BuildOutbound(r.Header, params.Referer, params.Cookie, params.Token, params.Origin)
		req.Header = outbound

		// Per-request timeout via a timed http.Client copy.
		timedClient := &http.Client{
			Transport:     client.Transport,
			CheckRedirect: client.CheckRedirect,
			Timeout:       timeout,
		}

		resp, err := timedClient.Do(req)
		if err != nil {
			logUpstreamFetchError(err)
			http.Error(w, "bad gateway: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Re-classify now that we have the actual Content-Type from the origin.
		originCT := resp.Header.Get("Content-Type")
		class := m3u8.Classify(originCT, params.TargetURL)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 &&
			class == m3u8.ClassPassthrough &&
			strings.Contains(strings.ToLower(originCT), "image/png") {
			class = sniffPNGClass(resp)
		}

		// Effective Content-Type fixes image/png masquerade for TS segments.
		effectiveCT := m3u8.EffectiveContentType(originCT, class)

		// Forward non-2xx errors verbatim (keep CORS headers so the player can read them).
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			writeHeaders(w.Header(), headers.BuildCORSPreflight())
			http.Error(w, fmt.Sprintf("origin returned %d", resp.StatusCode), resp.StatusCode)
			return
		}

		base := resolveProxyBase(proxyBase, r)

		switch class {
		case m3u8.ClassM3U8:
			handleM3U8(w, resp, params, base, effectiveCT)

		case m3u8.ClassTS:
			handleTS(w, resp, effectiveCT)

		default:
			handlePassthrough(w, resp, effectiveCT)
		}
	})
}

func resolveProxyBase(proxyBase string, r *http.Request) string {
	if proxyBase != "" {
		return strings.TrimRight(proxyBase, "/")
	}

	scheme := forwardedScheme(r)
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	host := forwardedHost(r)
	if host == "" {
		host = r.Host
	}

	return scheme + "://" + host
}

func forwardedScheme(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-Proto"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	return ""
}

func forwardedHost(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-Host"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	return ""
}

func sniffPNGClass(resp *http.Response) m3u8.ContentClass {
	buf := make([]byte, strip.SniffSize)
	n, err := io.ReadFull(resp.Body, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf[:n]), resp.Body))
		return m3u8.ClassPassthrough
	}
	buf = buf[:n]
	resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf), resp.Body))
	if strip.IsPNGPrefixedTS(buf) {
		return m3u8.ClassTS
	}
	return m3u8.ClassPassthrough
}

// handleM3U8 buffers the playlist, rewrites URLs, and responds.
func handleM3U8(w http.ResponseWriter, resp *http.Response, params *urlutil.ProxyParams, proxyBase, contentType string) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read playlist", http.StatusBadGateway)
		return
	}

	rewritten := m3u8.RewriteM3U8(
		string(body),
		params.TargetURL,
		proxyBase,
		params.Referer,
		params.Cookie,
		params.Token,
	)

	writeHeaders(w.Header(), headers.BuildResponse(resp.Header, contentType))
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, rewritten)
}

// handleTS streams the TS segment, stripping any fake PNG header on the fly.
func handleTS(w http.ResponseWriter, resp *http.Response, contentType string) {
	writeHeaders(w.Header(), headers.BuildResponse(resp.Header, contentType))
	w.WriteHeader(resp.StatusCode)

	sw := strip.NewStripWriter(w)
	if _, err := io.Copy(sw, resp.Body); err != nil {
		logStreamError("TS", err)
	}
}

// handlePassthrough streams the response body with no modification.
func handlePassthrough(w http.ResponseWriter, resp *http.Response, contentType string) {
	writeHeaders(w.Header(), headers.BuildResponse(resp.Header, contentType))
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		logStreamError("passthrough", err)
	}
}

func writeHeaders(dst, src http.Header) {
	for name, values := range src {
		for _, value := range values {
			dst.Set(name, value)
		}
	}
}

func logStreamError(label string, err error) {
	if isBenignStreamError(err) {
		return
	}
	log.Printf("%s stream error: %v", label, err)
}

func logUpstreamFetchError(err error) {
	if isBenignStreamError(err) {
		return
	}
	log.Printf("upstream fetch error: %v", err)
}

func isBenignStreamError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "context canceled")
}
