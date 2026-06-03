// Package pagination menyediakan cursor-based pagination helper sesuai DEC-022.
// TIDAK ada offset pagination — cursor only.
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Params adalah parameter pagination yang di-parse dari query string.
type Params struct {
	// Cursor adalah opaque base64-encoded cursor dari response sebelumnya.
	Cursor string
	// Limit adalah jumlah item per halaman. Default 50, max 200.
	Limit int
}

// DefaultLimit adalah nilai default limit.
const DefaultLimit = 50

// MaxLimit adalah nilai maksimal limit (api-conventions.md).
const MaxLimit = 200

// CursorData adalah isi cursor yang di-encode ke base64.
// Design choice: kita encode ID + field sort untuk keakuratan cursor.
type CursorData struct {
	// ID adalah primary key dari row terakhir di halaman sebelumnya.
	ID string `json:"id"`
	// SortVal adalah nilai kolom sort utama dari row terakhir (untuk composite cursor).
	SortVal any `json:"sv,omitempty"`
}

// ParseParams mem-parse query params pagination dari string.
// cursor dan limit di-pass sebagai string dari c.Query().
func ParseParams(cursorStr, limitStr string) (Params, error) {
	p := Params{Limit: DefaultLimit}

	if cursorStr != "" {
		p.Cursor = cursorStr
	}

	if limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil {
			return p, fmt.Errorf("limit tidak valid: harus angka bulat")
		}
		if l < 1 {
			l = DefaultLimit
		}
		if l > MaxLimit {
			l = MaxLimit
		}
		p.Limit = l
	}

	return p, nil
}

// EncodeCursor meng-encode CursorData ke opaque base64 string.
func EncodeCursor(data CursorData) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// DecodeCursor mendekode opaque base64 cursor menjadi CursorData.
func DecodeCursor(cursor string) (CursorData, error) {
	// Trim padding yang mungkin hilang.
	cursor = strings.TrimRight(cursor, "=")
	// Tambah padding kembali.
	switch len(cursor) % 4 {
	case 2:
		cursor += "=="
	case 3:
		cursor += "="
	}

	b, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return CursorData{}, fmt.Errorf("cursor tidak valid: %w", err)
	}

	var data CursorData
	if err := json.Unmarshal(b, &data); err != nil {
		return CursorData{}, fmt.Errorf("cursor corrupt: %w", err)
	}
	return data, nil
}

// Result adalah hasil query dengan metadata pagination.
type Result struct {
	// HasMore menandakan ada halaman berikutnya.
	HasMore bool
	// NextCursor adalah cursor untuk halaman berikutnya (nil jika tidak ada).
	NextCursor *string
	// TotalEstimate adalah estimasi total row (dari EXPLAIN, opsional).
	TotalEstimate *int64
	// Limit adalah limit yang dipakai.
	Limit int
}

// BuildResult membangun PaginationResult dari hasil query.
// items adalah slice yang sudah di-fetch dengan limit+1.
// lastID adalah ID dari item terakhir yang ditampilkan (sebelum +1 item).
func BuildResult(fetchedCount int, limit int, lastID string, totalEstimate *int64) Result {
	hasMore := fetchedCount > limit
	r := Result{
		HasMore:       hasMore,
		TotalEstimate: totalEstimate,
		Limit:         limit,
	}

	if hasMore && lastID != "" {
		cursor, err := EncodeCursor(CursorData{ID: lastID})
		if err == nil {
			r.NextCursor = &cursor
		}
	}

	return r
}
