// Package proxy implements the /proxy HTTP handler.
package proxy

import (
	"fmt"
	"io"
	"log"
	"net/http"
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
		// OPTIONS preflight.
		if r.Method == http.MethodOptions {
			for k, vs := range headers.BuildCORSPreflight() {
				for _, v := range vs {
					w.Header().Set(k, v)
				}
			}
			w.WriteHeader(http.StatusNoContent)
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
		ctx := r.Context()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, params.TargetURL, nil)
		if err != nil {
			http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
			return
		}
		outbound := headers.BuildOutbound(r.Header, params.Referer, params.Cookie, params.Token)
		req.Header = outbound

		// Per-request timeout via a timed http.Client copy.
		timedClient := &http.Client{
			Transport:     client.Transport,
			CheckRedirect: client.CheckRedirect,
			Timeout:       timeout,
		}

		resp, err := timedClient.Do(req)
		if err != nil {
			log.Printf("upstream fetch error: %v", err)
			http.Error(w, "bad gateway: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Re-classify now that we have the actual Content-Type from the origin.
		originCT := resp.Header.Get("Content-Type")
		class := m3u8.Classify(originCT, params.TargetURL)

		// Effective Content-Type fixes image/png masquerade for TS segments.
		effectiveCT := m3u8.EffectiveContentType(originCT, class)

		// Forward non-2xx errors verbatim (keep CORS headers so the player can read them).
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			for k, vs := range headers.BuildCORSPreflight() {
				for _, v := range vs {
					w.Header().Set(k, v)
				}
			}
			http.Error(w, fmt.Sprintf("origin returned %d", resp.StatusCode), resp.StatusCode)
			return
		}

		base := proxyBase
		if base == "" {
			base = "http://" + r.Host
		}
		log.Printf("Base: %s", base)

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

	responseHeaders := headers.BuildResponse(resp.Header, contentType)
	for k, vs := range responseHeaders {
		for _, v := range vs {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, rewritten)
}

// handleTS streams the TS segment, stripping any fake PNG header on the fly.
func handleTS(w http.ResponseWriter, resp *http.Response, contentType string) {
	responseHeaders := headers.BuildResponse(resp.Header, contentType)
	for k, vs := range responseHeaders {
		for _, v := range vs {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	sw := strip.NewStripWriter(w)
	if _, err := io.Copy(sw, resp.Body); err != nil {
		log.Printf("TS stream error: %v", err)
	}
}

// handlePassthrough streams the response body with no modification.
func handlePassthrough(w http.ResponseWriter, resp *http.Response, contentType string) {
	responseHeaders := headers.BuildResponse(resp.Header, contentType)
	for k, vs := range responseHeaders {
		for _, v := range vs {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("passthrough stream error: %v", err)
	}
}
