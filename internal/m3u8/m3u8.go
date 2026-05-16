// Package m3u8 handles playlist classification and URL rewriting.
package m3u8

import (
	"net/url"
	"sort"
	"strconv"
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
	if class := contentTypeClass(contentType); class != ClassPassthrough {
		return class
	}
	return urlPathClass(targetURL)
}

func contentTypeClass(contentType string) ContentClass {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "mpegurl") {
		return ClassM3U8
	}
	if strings.Contains(ct, "mp2t") {
		return ClassTS
	}
	return ClassPassthrough
}

func urlPathClass(targetURL string) ContentClass {
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
	opts := rewriteOptions{
		baseURL:   m3u8URL,
		proxyBase: proxyBase,
		referer:   referer,
		cookie:    cookie,
		token:     token,
	}
	lines := strings.Split(text, "\n")
	lines = sortVariantPlaylists(lines)
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
			out = append(out, line)

		case strings.HasPrefix(trimmed, "#"):
			// Rewrite URI="..." attributes inline.
			out = append(out, rewriteURIAttrs(trimmed, opts))

		default:
			// Bare URL line (segment or sub-playlist).
			out = append(out, proxyLine(trimmed, opts))
		}
	}

	return strings.Join(out, "\n")
}

type rewriteOptions struct {
	baseURL   string
	proxyBase string
	referer   string
	cookie    string
	token     string
}

type variantPlaylist struct {
	lines  []string
	width  int
	height int
	index  int
}

func sortVariantPlaylists(lines []string) []string {
	var variants []variantPlaylist
	first := -1
	last := -1

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "#EXT-X-STREAM-INF:") || i+1 >= len(lines) {
			continue
		}

		width, height, ok := streamResolution(trimmed)
		if !ok {
			continue
		}

		next := strings.TrimSpace(lines[i+1])
		if next == "" || strings.HasPrefix(next, "#") {
			continue
		}

		if first == -1 {
			first = i
		}
		last = i + 1
		variants = append(variants, variantPlaylist{
			lines:  []string{lines[i], lines[i+1]},
			width:  width,
			height: height,
			index:  len(variants),
		})
		i++
	}

	if len(variants) < 2 {
		return lines
	}

	sort.SliceStable(variants, func(i, j int) bool {
		if variants[i].height != variants[j].height {
			return variants[i].height > variants[j].height
		}
		if variants[i].width != variants[j].width {
			return variants[i].width > variants[j].width
		}
		return variants[i].index < variants[j].index
	})

	out := make([]string, 0, len(lines))
	out = append(out, lines[:first]...)
	for _, variant := range variants {
		out = append(out, variant.lines...)
	}
	out = append(out, lines[last+1:]...)
	return out
}

func streamResolution(line string) (int, int, bool) {
	const key = "RESOLUTION="
	upperLine := strings.ToUpper(line)
	idx := strings.Index(upperLine, key)
	if idx == -1 {
		return 0, 0, false
	}

	value := strings.ToLower(line[idx+len(key):])
	if comma := strings.IndexByte(value, ','); comma != -1 {
		value = value[:comma]
	}

	parts := strings.SplitN(value, "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}

	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return width, height, true
}

// rewriteURIAttrs rewrites all URI="..." occurrences in a tag line.
func rewriteURIAttrs(line string, opts rewriteOptions) string {
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
		sb.WriteString(proxyLine(uri, opts))
		sb.WriteByte('"')
		rest = rest[end+1:]
	}
	return sb.String()
}

func proxyLine(line string, opts rewriteOptions) string {
	abs := resolveURL(line, opts.baseURL)
	return urlutil.BuildProxyURL(opts.proxyBase, abs, opts.referer, opts.cookie, opts.token)
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
