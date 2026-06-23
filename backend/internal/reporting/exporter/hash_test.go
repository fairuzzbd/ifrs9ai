package exporter_test

import (
	"bytes"
	"strings"
	"testing"

	"blips-ifrs9.tugu-re.com/internal/reporting/exporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSHA256OfBytes_KnownVector(t *testing.T) {
	// SHA-256 of empty byte slice.
	got := exporter.SHA256OfBytes([]byte{})
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", got)
}

func TestSHA256OfBytes_Content(t *testing.T) {
	got := exporter.SHA256OfBytes([]byte("hello world"))
	// Actual SHA-256 of "hello world" (verified with sha256sum).
	assert.Equal(t, "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9", got)
}

func TestSHA256Writer_MatchesDirectHash(t *testing.T) {
	data := []byte("BLIPS IFRS9 test data for SHA-256 dual")

	var buf bytes.Buffer
	sw := exporter.NewSHA256Writer(&buf)
	n, err := sw.Write(data)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)

	// Written bytes should match.
	assert.Equal(t, data, buf.Bytes())

	// SHA-256 from writer must match direct hash.
	direct := exporter.SHA256OfBytes(data)
	assert.Equal(t, direct, sw.Sum())
}

func TestSHA256Writer_MultipleWrites(t *testing.T) {
	parts := []string{"part1", "part2", "part3"}
	combined := strings.Join(parts, "")

	var buf bytes.Buffer
	sw := exporter.NewSHA256Writer(&buf)
	for _, p := range parts {
		_, err := sw.Write([]byte(p))
		require.NoError(t, err)
	}

	assert.Equal(t, exporter.SHA256OfBytes([]byte(combined)), sw.Sum())
	assert.Equal(t, combined, buf.String())
}

func TestSHA256Writer_Empty(t *testing.T) {
	var buf bytes.Buffer
	sw := exporter.NewSHA256Writer(&buf)
	assert.Equal(t, exporter.SHA256OfBytes([]byte{}), sw.Sum())
}
