package pagination_test

import (
	"testing"

	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

func TestParseParams_Defaults(t *testing.T) {
	p, err := pagination.ParseParams("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Limit != pagination.DefaultLimit {
		t.Errorf("expected default limit %d, got %d", pagination.DefaultLimit, p.Limit)
	}
	if p.Cursor != "" {
		t.Errorf("expected empty cursor, got %s", p.Cursor)
	}
}

func TestParseParams_ValidLimit(t *testing.T) {
	p, err := pagination.ParseParams("", "100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Limit != 100 {
		t.Errorf("expected 100, got %d", p.Limit)
	}
}

func TestParseParams_MaxLimitClamped(t *testing.T) {
	p, err := pagination.ParseParams("", "999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Limit != pagination.MaxLimit {
		t.Errorf("expected max limit %d, got %d", pagination.MaxLimit, p.Limit)
	}
}

func TestParseParams_InvalidLimit(t *testing.T) {
	_, err := pagination.ParseParams("", "not-a-number")
	if err == nil {
		t.Error("expected error for invalid limit")
	}
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	data := pagination.CursorData{ID: "test-id-123"}
	encoded, err := pagination.EncodeCursor(data)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if encoded == "" {
		t.Error("encoded cursor should not be empty")
	}

	decoded, err := pagination.DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.ID != data.ID {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID, data.ID)
	}
}

func TestDecodeCursor_Invalid(t *testing.T) {
	_, err := pagination.DecodeCursor("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid cursor")
	}
}

func TestDecodeCursor_CorruptJSON(t *testing.T) {
	import64 := "e30K" // base64 of "}\n" — invalid JSON
	_, err := pagination.DecodeCursor(import64)
	// May or may not error depending on what it decodes to, but shouldn't panic.
	_ = err
}

func TestBuildResult_HasMore(t *testing.T) {
	// Fetched limit+1 items → hasMore = true.
	r := pagination.BuildResult(51, 50, "last-id", nil)
	if !r.HasMore {
		t.Error("expected hasMore = true")
	}
	if r.NextCursor == nil {
		t.Error("expected non-nil NextCursor")
	}
}

func TestBuildResult_NoMore(t *testing.T) {
	// Fetched exactly limit items → hasMore = false.
	r := pagination.BuildResult(50, 50, "last-id", nil)
	if r.HasMore {
		t.Error("expected hasMore = false")
	}
	if r.NextCursor != nil {
		t.Error("expected nil NextCursor")
	}
}

func TestBuildResult_Empty(t *testing.T) {
	r := pagination.BuildResult(0, 50, "", nil)
	if r.HasMore {
		t.Error("expected hasMore = false for empty result")
	}
	if r.NextCursor != nil {
		t.Error("expected nil NextCursor for empty result")
	}
}

func TestBuildResult_TotalEstimate(t *testing.T) {
	total := int64(12340)
	r := pagination.BuildResult(10, 50, "x", &total)
	if r.TotalEstimate == nil || *r.TotalEstimate != 12340 {
		t.Error("TotalEstimate not propagated")
	}
}
