// Package m3u8 handles playlist classification and URL rewriting.
package m3u8

import (
	"net/url"
	"strings"

	"github.com/IstarVin/hls-proxy/internal/urlutil"
)

// ContentClass categorises an origin response.
type ContentClass int

const (
	ClassPassthrough ContentClass = iota
	ClassM3U8
	ClassTS
)

// Classify determines how to handle a response based on Content-Type and URL.
// It also accounts for servers that send image/png for TS segments.
func Classify(contentType, targetURL string) ContentClass {
	ct := strings.ToLower(contentType)

	// Explicit playlist types.
	if strings.Contains(ct, "mpegurl") {
		return ClassM3U8
	}
	// Explicit TS MIME type.
	if strings.Contains(ct, "mp2t") || strings.Contains(ct, "png") {
		return ClassTS
	}

	// Fall back to URL extension — this also handles the image/png masquerade:
	// if the URL ends in .ts we treat the body as TS regardless of Content-Type.
	path := strings.ToLower(strings.SplitN(targetURL, "?", 2)[0])
	if strings.HasSuffix(path, ".m3u8") || strings.HasSuffix(path, ".m3") {
		return ClassM3U8
	}
	if strings.HasSuffix(path, ".ts") {
		return ClassTS
	}

	return ClassPassthrough
}

// EffectiveContentType returns the Content-Type the proxy should send to the
// player, correcting the image/png masquerade when the body is actually TS.
func EffectiveContentType(originContentType string, class ContentClass) string {
	switch class {
	case ClassM3U8:
		return "application/vnd.apple.mpegurl"
	case ClassTS:
		// Always override — even if the origin sent image/png or something else.
		return "video/mp2t"
	default:
		return originContentType
	}
}

// RewriteM3U8 rewrites all URLs inside a playlist body so they go through the
// proxy, carrying the same auth context forward.
func RewriteM3U8(text, m3u8URL, proxyBase, referer, cookie, token string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
			out = append(out, line)

		case strings.HasPrefix(trimmed, "#"):
			// Rewrite URI="..." attributes inline.
			rewritten := rewriteURIAttrs(trimmed, m3u8URL, proxyBase, referer, cookie, token)
			out = append(out, rewritten)

		default:
			// Bare URL line (segment or sub-playlist).
			abs := resolveURL(trimmed, m3u8URL)
			out = append(out, urlutil.BuildProxyURL(proxyBase, abs, referer, cookie, token))
		}
	}

	return strings.Join(out, "\n")
}

// rewriteURIAttrs rewrites all URI="..." occurrences in a tag line.
func rewriteURIAttrs(line, base, proxyBase, referer, cookie, token string) string {
	const open = `URI="`
	var sb strings.Builder
	rest := line
	for {
		idx := strings.Index(rest, open)
		if idx == -1 {
			sb.WriteString(rest)
			break
		}
		sb.WriteString(rest[:idx+len(open)])
		rest = rest[idx+len(open):]

		end := strings.IndexByte(rest, '"')
		if end == -1 {
			sb.WriteString(rest)
			break
		}
		uri := rest[:end]
		abs := resolveURL(uri, base)
		sb.WriteString(urlutil.BuildProxyURL(proxyBase, abs, referer, cookie, token))
		sb.WriteByte('"')
		rest = rest[end+1:]
	}
	return sb.String()
}

// resolveURL resolves a possibly-relative URL against a base URL.
func resolveURL(ref, base string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}
