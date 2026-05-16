// Package strip detects and removes fake PNG headers prepended to MPEG-TS
// segments. Detection is fully dynamic — no hardcoded offset is assumed.
package strip

import "io"

const (
	tsSyncByte   = 0x47
	tsPacketSize = 188
	confirmCount = 3    // target consecutive sync bytes to confirm TS alignment
	maxScan      = 1024 // max bytes to scan for a header before giving up
)

var pngSignature = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

// SniffSize is enough bytes to find a TS sync pair after any supported prefix.
const SniffSize = maxScan + tsPacketSize + 1

// FindTSOffset scans the beginning of data for the first position where
// 0x47 bytes appear at 188-byte intervals. It aims for confirmCount
// confirmations but accepts 2 when the buffer is short.
// Returns the byte offset where TS data starts, or 0 if no pattern is found.
func FindTSOffset(data []byte) int {
	// Need enough room for at least one 188-byte step.
	if len(data) < tsPacketSize+1 {
		return 0
	}

	scanEnd := maxScan
	if scanEnd > len(data) {
		scanEnd = len(data)
	}

	for i := 0; i < scanEnd; i++ {
		if data[i] != tsSyncByte {
			continue
		}
		// Count how many consecutive sync positions we can verify.
		matched := 1
		for n := 1; n < confirmCount; n++ {
			pos := i + tsPacketSize*n
			if pos >= len(data) {
				break // ran out of buffer — use what we confirmed
			}
			if data[pos] != tsSyncByte {
				matched = 0
				break
			}
			matched++
		}
		// Accept if we matched at least 2 consecutive sync positions.
		if matched >= 2 {
			return i
		}
	}
	return 0
}

// IsPNGPrefixedTS reports whether data starts as PNG and contains TS packets.
func IsPNGPrefixedTS(data []byte) bool {
	return hasPrefix(pngSignature, data) && FindTSOffset(data) != 0
}

// StripWriter wraps an io.Writer and strips the fake PNG header from the first
// write only. All subsequent writes pass through untouched.
type StripWriter struct {
	w         io.Writer
	processed bool
	buf       []byte
}

// NewStripWriter returns a StripWriter that forwards stripped bytes to w.
func NewStripWriter(w io.Writer) *StripWriter {
	return &StripWriter{w: w}
}

// Write implements io.Writer. On the first call it locates and skips the fake
// header; subsequent calls are a direct forwarding.
func (s *StripWriter) Write(p []byte) (int, error) {
	if s.processed {
		return s.w.Write(p)
	}

	s.buf = append(s.buf, p...)
	if s.needsMoreData() {
		return len(p), nil
	}

	s.processed = true
	buffered := s.buf
	s.buf = nil

	offset := FindTSOffset(buffered)
	if offset == 0 {
		if _, err := s.w.Write(buffered); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	// Write the sliced view — no copy, just pointer arithmetic in the slice header.
	_, err := s.w.Write(buffered[offset:])
	// Report original len so callers don't think bytes were lost.
	return len(p), err
}

func (s *StripWriter) needsMoreData() bool {
	if len(s.buf) == 0 {
		return false
	}
	if !hasPrefix(pngSignature, s.buf) {
		return false
	}
	if FindTSOffset(s.buf) != 0 {
		return false
	}
	return len(s.buf) < SniffSize
}

func hasPrefix(prefix, data []byte) bool {
	if len(data) > len(prefix) {
		data = data[:len(prefix)]
	}
	for i := range data {
		if data[i] != prefix[i] {
			return false
		}
	}
	return true
}
