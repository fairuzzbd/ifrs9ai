package penjualan

// routing_test.go — 100% test coverage for routing.go (compliance-critical).
//
// Test matrix:
//   5 klasifikasi × 2 disposal types (locked=true)         = 10 cases
//   + locked=false for each klasifikasi (5 cases)
//   + unknown klasifikasi (locked=true)                     =  1 case
//   Total: 16 test cases.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── AC ──────────────────────────────────────────────────────────────────────

func TestResolveJurnalEventCode_AC_PARTIAL(t *testing.T) {
	r, err := ResolveJurnalEventCode(KlasifikasiAC, true, DisposalPartial)
	require.NoError(t, err)
	assert.Equal(t, []string{"PENJUALAN_AC"}, r.EventCodes)
	assert.False(t, r.RecycleOCI)
	assert.False(t, r.NoRecyclingFlag)
}

func TestResolveJurnalEventCode_AC_FULL(t *testing.T) {
	r, err := ResolveJurnalEventCode(KlasifikasiAC, true, DisposalFull)
	require.NoError(t, err)
	assert.Equal(t, []string{"PENJUALAN_AC"}, r.EventCodes)
	assert.False(t, r.RecycleOCI)
	assert.False(t, r.NoRecyclingFlag)
}

// ─── FVOCI (debt) ─────────────────────────────────────────────────────────────

func TestResolveJurnalEventCode_FVOCI_PARTIAL(t *testing.T) {
	r, err := ResolveJurnalEventCode(KlasifikasiFVOCI, true, DisposalPartial)
	require.NoError(t, err)
	assert.Equal(t, []string{"PENJUALAN_FVOCI_DEBT", "REKLAS_OCI_PL"}, r.EventCodes,
		"FVOCI debt disposal must include REKLAS_OCI_PL recycling leg")
	assert.True(t, r.RecycleOCI, "FVOCI debt must recycle OCI to P&L (PSAK 71 §5.7.5)")
	assert.False(t, r.NoRecyclingFlag)
}

func TestResolveJurnalEventCode_FVOCI_FULL(t *testing.T) {
	r, err := ResolveJurnalEventCode(KlasifikasiFVOCI, true, DisposalFull)
	require.NoError(t, err)
	assert.Equal(t, []string{"PENJUALAN_FVOCI_DEBT", "REKLAS_OCI_PL"}, r.EventCodes)
	assert.True(t, r.RecycleOCI)
	assert.False(t, r.NoRecyclingFlag)
}

// ─── FVOCI_ELECTION ───────────────────────────────────────────────────────────

func TestResolveJurnalEventCode_FVOCIElection_PARTIAL(t *testing.T) {
	// §B5.7.1: cumulative OCI stays in equity on disposal, NOT recycled to P&L.
	r, err := ResolveJurnalEventCode(KlasifikasiFVOCIElection, true, DisposalPartial)
	require.NoError(t, err)
	assert.Equal(t, []string{"PENJUALAN_FVOCI_ELECTION"}, r.EventCodes)
	assert.False(t, r.RecycleOCI, "FVOCI_ELECTION must NOT recycle OCI per §B5.7.1")
	assert.True(t, r.NoRecyclingFlag, "NoRecyclingFlag must be set for FVOCI_ELECTION")
}

func TestResolveJurnalEventCode_FVOCIElection_FULL(t *testing.T) {
	r, err := ResolveJurnalEventCode(KlasifikasiFVOCIElection, true, DisposalFull)
	require.NoError(t, err)
	assert.Equal(t, []string{"PENJUALAN_FVOCI_ELECTION"}, r.EventCodes)
	assert.False(t, r.RecycleOCI)
	assert.True(t, r.NoRecyclingFlag)
}

// ─── FVTPL ───────────────────────────────────────────────────────────────────

func TestResolveJurnalEventCode_FVTPL_PARTIAL(t *testing.T) {
	r, err := ResolveJurnalEventCode(KlasifikasiFVTPL, true, DisposalPartial)
	require.NoError(t, err)
	assert.Equal(t, []string{"PENJUALAN_FVTPL"}, r.EventCodes)
	assert.False(t, r.RecycleOCI)
	assert.False(t, r.NoRecyclingFlag)
}

func TestResolveJurnalEventCode_FVTPL_FULL(t *testing.T) {
	r, err := ResolveJurnalEventCode(KlasifikasiFVTPL, true, DisposalFull)
	require.NoError(t, err)
	assert.Equal(t, []string{"PENJUALAN_FVTPL"}, r.EventCodes)
	assert.False(t, r.RecycleOCI)
}

// ─── POCI ─────────────────────────────────────────────────────────────────────

func TestResolveJurnalEventCode_POCI_PARTIAL(t *testing.T) {
	r, err := ResolveJurnalEventCode(KlasifikasiPOCI, true, DisposalPartial)
	require.NoError(t, err)
	assert.Equal(t, []string{"PENJUALAN_POCI"}, r.EventCodes)
	assert.False(t, r.RecycleOCI)
	assert.False(t, r.NoRecyclingFlag)
}

func TestResolveJurnalEventCode_POCI_FULL(t *testing.T) {
	r, err := ResolveJurnalEventCode(KlasifikasiPOCI, true, DisposalFull)
	require.NoError(t, err)
	assert.Equal(t, []string{"PENJUALAN_POCI"}, r.EventCodes)
	assert.False(t, r.RecycleOCI)
}

// ─── locked = false (all 5 klasifikasi) ──────────────────────────────────────

func TestResolveJurnalEventCode_NotLocked(t *testing.T) {
	cases := []KlasifikasiPSAK71{
		KlasifikasiAC, KlasifikasiFVOCI, KlasifikasiFVOCIElection, KlasifikasiFVTPL, KlasifikasiPOCI,
	}
	for _, k := range cases {
		k := k
		t.Run("locked=false klasifikasi="+string(k), func(t *testing.T) {
			_, err := ResolveJurnalEventCode(k, false, DisposalPartial)
			require.Error(t, err, "locked=false must always return error")
			de, ok := domainerrors.IsDomainError(err)
			require.True(t, ok, "error must be DomainError")
			// routing.go uses CodeValidationFailed for not-locked case
			assert.Equal(t, domainerrors.CodeValidationFailed, de.Code())
			assert.Contains(t, de.Message(), "klasifikasi_locked")
		})
	}
}

// ─── Unknown klasifikasi (locked=true) ────────────────────────────────────────

func TestResolveJurnalEventCode_UnknownKlasifikasi(t *testing.T) {
	_, err := ResolveJurnalEventCode(KlasifikasiPSAK71("TOTALLY_UNKNOWN"), true, DisposalPartial)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok, "error must be DomainError")
	assert.Equal(t, domainerrors.CodeValidationFailed, de.Code())
	assert.Contains(t, de.Message(), "TOTALLY_UNKNOWN")
}

// ─── EventCodes never empty on success ────────────────────────────────────────

func TestResolveJurnalEventCode_EventCodesNeverEmpty(t *testing.T) {
	successCases := []struct {
		k      KlasifikasiPSAK71
		jenis  DisposalType
	}{
		{KlasifikasiAC, DisposalPartial},
		{KlasifikasiAC, DisposalFull},
		{KlasifikasiFVOCI, DisposalPartial},
		{KlasifikasiFVOCI, DisposalFull},
		{KlasifikasiFVOCIElection, DisposalPartial},
		{KlasifikasiFVOCIElection, DisposalFull},
		{KlasifikasiFVTPL, DisposalPartial},
		{KlasifikasiFVTPL, DisposalFull},
		{KlasifikasiPOCI, DisposalPartial},
		{KlasifikasiPOCI, DisposalFull},
	}
	for _, tc := range successCases {
		tc := tc
		t.Run(string(tc.k)+"_"+string(tc.jenis), func(t *testing.T) {
			r, err := ResolveJurnalEventCode(tc.k, true, tc.jenis)
			require.NoError(t, err)
			assert.NotEmpty(t, r.EventCodes, "EventCodes must never be empty on success")
		})
	}
}
