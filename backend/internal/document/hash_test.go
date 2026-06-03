package document

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
)

// TestHashReader_ComputesCorrectHash memverifikasi bahwa HashReader menghasilkan
// SHA-256 yang sama dengan sha256.Sum256 standar.
func TestHashReader_ComputesCorrectHash(t *testing.T) {
	data := []byte("BLIPS IFRS9 document content test 12345")
	expected := sha256.Sum256(data)
	expectedHex := hex.EncodeToString(expected[:])

	hr := NewHashReader(bytes.NewReader(data))
	_, err := io.ReadAll(hr)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}

	got := hr.SHA256Hex()
	if got != expectedHex {
		t.Errorf("SHA256Hex() = %q, want %q", got, expectedHex)
	}
}

// TestHashReader_BytesRead memverifikasi BytesRead akurat.
func TestHashReader_BytesRead(t *testing.T) {
	data := []byte("test data for byte count")
	hr := NewHashReader(bytes.NewReader(data))
	_, _ = io.ReadAll(hr)

	if hr.BytesRead() != int64(len(data)) {
		t.Errorf("BytesRead() = %d, want %d", hr.BytesRead(), len(data))
	}
}

// TestHashReader_EmptyInput memverifikasi hash untuk input kosong.
func TestHashReader_EmptyInput(t *testing.T) {
	expected := sha256.Sum256([]byte{})
	expectedHex := hex.EncodeToString(expected[:])

	hr := NewHashReader(bytes.NewReader([]byte{}))
	_, _ = io.ReadAll(hr)

	if hr.SHA256Hex() != expectedHex {
		t.Errorf("SHA256Hex() empty = %q, want %q", hr.SHA256Hex(), expectedHex)
	}
	if hr.BytesRead() != 0 {
		t.Errorf("BytesRead() = %d, want 0", hr.BytesRead())
	}
}

// TestHashReader_LargeInput memverifikasi HashReader untuk data besar (> 1MB).
func TestHashReader_LargeInput(t *testing.T) {
	// 2MB data
	data := bytes.Repeat([]byte("A"), 2*1024*1024)
	expected := sha256.Sum256(data)
	expectedHex := hex.EncodeToString(expected[:])

	hr := NewHashReader(bytes.NewReader(data))
	_, err := io.ReadAll(hr)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}

	if hr.SHA256Hex() != expectedHex {
		t.Errorf("SHA256Hex() large input mismatch")
	}
	if hr.BytesRead() != int64(len(data)) {
		t.Errorf("BytesRead() = %d, want %d", hr.BytesRead(), len(data))
	}
}

// TestHashReader_PartialRead memverifikasi hash benar meski dibaca bertahap.
func TestHashReader_PartialRead(t *testing.T) {
	data := []byte("hello world from BLIPS")
	expected := sha256.Sum256(data)
	expectedHex := hex.EncodeToString(expected[:])

	hr := NewHashReader(bytes.NewReader(data))
	buf := make([]byte, 4)
	for {
		_, err := hr.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read error: %v", err)
		}
	}

	if hr.SHA256Hex() != expectedHex {
		t.Errorf("SHA256Hex() partial read = %q, want %q", hr.SHA256Hex(), expectedHex)
	}
}

// TestVerifyHash_Match memverifikasi bahwa VerifyHash tidak error untuk hash yang benar.
func TestVerifyHash_Match(t *testing.T) {
	data := []byte("verify hash test data")
	sum := sha256.Sum256(data)
	hashHex := hex.EncodeToString(sum[:])

	if err := VerifyHash(data, hashHex); err != nil {
		t.Errorf("VerifyHash harus tidak error untuk hash yang benar, got: %v", err)
	}
}

// TestVerifyHash_Mismatch memverifikasi bahwa VerifyHash error untuk hash yang salah.
func TestVerifyHash_Mismatch(t *testing.T) {
	data := []byte("verify hash test data")
	wrongHash := strings.Repeat("0", 64) // all zeros

	err := VerifyHash(data, wrongHash)
	if err == nil {
		t.Error("VerifyHash harus error untuk hash yang tidak cocok")
	}
}

// TestComputeSHA256Hex memverifikasi ComputeSHA256Hex.
func TestComputeSHA256Hex(t *testing.T) {
	data := []byte("compute sha256 test")
	expected := sha256.Sum256(data)
	expectedHex := hex.EncodeToString(expected[:])

	got, n, err := ComputeSHA256Hex(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ComputeSHA256Hex error: %v", err)
	}
	if got != expectedHex {
		t.Errorf("hash = %q, want %q", got, expectedHex)
	}
	if n != int64(len(data)) {
		t.Errorf("n = %d, want %d", n, len(data))
	}
}

// TestHashDeterminism memverifikasi bahwa hash deterministik untuk input yang sama.
func TestHashDeterminism(t *testing.T) {
	data := []byte("deterministic test")

	hr1 := NewHashReader(bytes.NewReader(data))
	_, _ = io.ReadAll(hr1)
	h1 := hr1.SHA256Hex()

	hr2 := NewHashReader(bytes.NewReader(data))
	_, _ = io.ReadAll(hr2)
	h2 := hr2.SHA256Hex()

	if h1 != h2 {
		t.Errorf("hash tidak deterministik: %q != %q", h1, h2)
	}
}

// TestHashDifferentInputs memverifikasi bahwa hash berbeda untuk input berbeda.
func TestHashDifferentInputs(t *testing.T) {
	data1 := []byte("input one")
	data2 := []byte("input two")

	hr1 := NewHashReader(bytes.NewReader(data1))
	_, _ = io.ReadAll(hr1)

	hr2 := NewHashReader(bytes.NewReader(data2))
	_, _ = io.ReadAll(hr2)

	if hr1.SHA256Hex() == hr2.SHA256Hex() {
		t.Error("hash berbeda untuk input berbeda seharusnya tidak sama")
	}
}
