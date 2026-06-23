// Package exporter implements CSV/XLSX/PDF export with watermark + SHA-256.
// P5-M13-S3: every format has watermark + SHA-256 dual (file content + audit log chain).
package exporter

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
)

// SHA256Writer wraps an io.Writer and computes SHA-256 of all bytes written.
// After writing is complete, call Sum() to retrieve the hex digest.
type SHA256Writer struct {
	w io.Writer
	h hash.Hash
}

// NewSHA256Writer creates a SHA256Writer wrapping the given writer.
func NewSHA256Writer(w io.Writer) *SHA256Writer {
	return &SHA256Writer{w: w, h: sha256.New()}
}

// Write writes p to the underlying writer and updates the SHA-256 state.
func (sw *SHA256Writer) Write(p []byte) (int, error) {
	n, err := sw.w.Write(p)
	if n > 0 {
		sw.h.Write(p[:n])
	}
	return n, err
}

// Sum returns the hex-encoded SHA-256 digest of all bytes written so far.
func (sw *SHA256Writer) Sum() string {
	return hex.EncodeToString(sw.h.Sum(nil))
}

// SHA256OfBytes computes SHA-256 of a byte slice and returns hex string.
func SHA256OfBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
