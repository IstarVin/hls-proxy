package strip_test

import (
	"bytes"
	"testing"

	"github.com/yourname/hls-proxy/internal/strip"
)

func makePNGPrefixed(tsData []byte) []byte {
	// Minimal 1x1 palette PNG — 95 bytes, matches real-world samples.
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG magic
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, // IHDR
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x03, 0x00, 0x00, 0x00, 0x28, 0xcb, 0x34,
		0xbb, 0x00, 0x00, 0x00, 0x03, 0x50, 0x4c, 0x54, // PLTE
		0x45, 0xff, 0xff, 0xff, 0xa7, 0xc4, 0x1b, 0xc8,
		0x00, 0x00, 0x00, 0x01, 0x74, 0x52, 0x4e, 0x53, // tRNS
		0x00, 0x40, 0xe6, 0xd8, 0x66, 0x00, 0x00, 0x00,
		0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x62, // IDAT
		0x00, 0x00, 0x00, 0x02, 0x00, 0x01, 0xe2, 0x21,
		0xbc, 0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, // IEND
		0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
	return append(png, tsData...)
}

// makeTSPackets builds n minimal TS packets (sync byte + 187 zero bytes each).
func makeTSPackets(n int) []byte {
	data := make([]byte, 188*n)
	for i := 0; i < n; i++ {
		data[i*188] = 0x47
	}
	return data
}

func TestFindTSOffset_CleanTS(t *testing.T) {
	ts := makeTSPackets(5)
	if off := strip.FindTSOffset(ts); off != 0 {
		t.Fatalf("expected offset 0 for clean TS, got %d", off)
	}
}

func TestFindTSOffset_PNGPrefixed(t *testing.T) {
	ts := makeTSPackets(5)
	data := makePNGPrefixed(ts)
	off := strip.FindTSOffset(data)
	if off != 94 {
		t.Fatalf("expected offset 94, got %d", off)
	}
	if data[off] != 0x47 {
		t.Fatalf("byte at offset %d is 0x%02x, not 0x47", off, data[off])
	}
}

func TestFindTSOffset_TooShort(t *testing.T) {
	// Not enough bytes to confirm three sync points — should return 0 gracefully.
	short := makeTSPackets(2) // only 2 packets, need 3 for confirm
	off := strip.FindTSOffset(short)
	// Either 0 (not found) or a valid offset — just must not panic.
	_ = off
}

func TestStripWriter_PNGPrefixed(t *testing.T) {
	ts := makeTSPackets(4)
	data := makePNGPrefixed(ts)

	var buf bytes.Buffer
	sw := strip.NewStripWriter(&buf)

	_, err := sw.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), ts) {
		t.Fatalf("stripped output mismatch: got %d bytes, want %d", buf.Len(), len(ts))
	}
}

func TestStripWriter_CleanTS(t *testing.T) {
	ts := makeTSPackets(4)

	var buf bytes.Buffer
	sw := strip.NewStripWriter(&buf)
	_, err := sw.Write(ts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), ts) {
		t.Fatal("clean TS must not be modified")
	}
}

func TestStripWriter_MultipleWrites(t *testing.T) {
	ts := makeTSPackets(10)
	data := makePNGPrefixed(ts)

	// Split into first-chunk (covers the PNG) and subsequent chunks.
	firstChunk := data[:300]
	rest := data[300:]

	var buf bytes.Buffer
	sw := strip.NewStripWriter(&buf)
	sw.Write(firstChunk)
	sw.Write(rest)

	if !bytes.Equal(buf.Bytes(), ts) {
		t.Fatalf("multi-write: got %d bytes, want %d", buf.Len(), len(ts))
	}
}
