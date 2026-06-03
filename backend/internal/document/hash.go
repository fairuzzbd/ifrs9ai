package document

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

// HashReader adalah komponen yang lebih sederhana menggunakan sha256.Hash langsung.
type HashReader struct {
	inner  io.Reader
	hasher io.Writer
	hsum   func() []byte
	read   int64
}

// NewHashReader membuat HashReader yang menghitung SHA-256 saat membaca.
func NewHashReader(r io.Reader) *HashReader {
	h := sha256.New()
	return &HashReader{
		inner:  io.TeeReader(r, h),
		hasher: h,
		hsum:   func() []byte { return h.Sum(nil) },
	}
}

// Read mengimplementasikan io.Reader. Setiap byte yang dibaca juga di-hash.
func (hr *HashReader) Read(p []byte) (int, error) {
	n, err := hr.inner.Read(p)
	hr.read += int64(n)
	return n, err
}

// SHA256Hex mengembalikan hex string dari SHA-256 hash yang sudah dihitung.
// HARUS dipanggil SETELAH semua data dibaca (io.EOF atau io.ReadAll selesai).
func (hr *HashReader) SHA256Hex() string {
	return hex.EncodeToString(hr.hsum())
}

// BytesRead mengembalikan total bytes yang sudah dibaca.
func (hr *HashReader) BytesRead() int64 {
	return hr.read
}

// VerifyHash memverifikasi bahwa hash dari data cocok dengan expected.
// expected adalah hex-encoded SHA-256.
func VerifyHash(data []byte, expectedHex string) error {
	actual := sha256.Sum256(data)
	actualHex := hex.EncodeToString(actual[:])
	if actualHex != expectedHex {
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHex, actualHex)
	}
	return nil
}

// ComputeSHA256Hex menghitung SHA-256 dari reader dan mengembalikan hex string.
// Data di-consume sepenuhnya.
func ComputeSHA256Hex(r io.Reader) (string, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", 0, fmt.Errorf("compute sha256: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
