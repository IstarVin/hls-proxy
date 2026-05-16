# hls-proxy

A fast Go HLS proxy that:
- Strips fake PNG headers prepended to `.ts` segments (dynamic detection, no hardcoded offset)
- **Fixes `Content-Type: image/png` misreporting** — segments served as `image/png` are detected by URL extension and re-typed as `video/mp2t`
- Rewrites M3U8 playlists so all sub-playlist and segment URLs route through the proxy
- Propagates `Referer`, `Cookie`, and `Authorization` headers through every level of a playlist
- Streams TS segments without buffering (first bytes reach the player before the full segment downloads)

## Quick start

```sh
go run ./cmd/hls-proxy          # listens on :3000
PORT=8080 go run ./cmd/hls-proxy
```

### Docker

```sh
docker build -t hls-proxy .
docker run -p 3000:3000 -e PROXY_BASE=https://my.server.com hls-proxy
```

## Endpoint

```
GET /proxy?[referer=R&][cookie=C&][token=T&]url=<TARGET_URL>
```

`url=` **must be the last parameter** — everything after `url=` is taken as the raw target URL so that `&` inside the target URL survives intact.

| Param | Forwarded as | Description |
|---|---|---|
| `referer` | `Referer: <value>` | Origin referer |
| `cookie` | `Cookie: <value>` | Session cookies |
| `token` | `Authorization: Bearer <value>` | Bearer token |
| `url` | — | Target URL (always last) |

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `3000` | Listen port |
| `PROXY_BASE` | `http://<host>` | Base URL used when rewriting M3U8 playlists. Self-configures from the incoming request host if unset. |

## PNG masquerade handling

Some origin servers send `.ts` segments with:
- `Content-Type: image/png` in the HTTP response header
- An actual 1×1 PNG prepended to the raw MPEG-TS data

The proxy handles both problems independently:

1. **Wrong Content-Type** — Classification falls back to URL extension when the MIME type isn't a recognised TS/M3U8 type. A `.ts` URL classified as TS always gets `Content-Type: video/mp2t` in the response regardless of what the origin sent.

2. **PNG header bytes** — `FindTSOffset` scans the first chunk for three consecutive TS sync bytes (`0x47`) at 188-byte intervals. The header is stripped in the first `Write` call; all subsequent writes are zero-overhead passthroughs.

## Running tests

```sh
go test ./...
```

## Project structure

```
cmd/hls-proxy/     main entrypoint
internal/
  m3u8/            classify(), EffectiveContentType(), RewriteM3U8()
  strip/           FindTSOffset(), StripWriter
  headers/         outbound + response header builders
  urlutil/         ParseProxyQuery(), BuildProxyURL(), ValidateTargetURL()
```
