package mtm

// routing_test.go — 100% branch coverage for routing.go.
//
// BLOCKING gate: ifrs9-compliance-reviewer must approve any change to routing.go.
// Test matrix:
//
//	+------------------+-------+--------+----------+
//	| Klasifikasi       | IDR   | FCY    | isPOCI   |
//	+------------------+-------+--------+----------+
//	| AC               |   SKIP|   SKIP |  n/a     |
//	| FVOCI_DEBT       | FVOCI | FVOCI+ |  false   |
//	|                  |       |  FX_OCI|          |
//	| FVOCI_ELECTION   | ELEC  | ELEC   |  any     |
//	| FVTPL            | FVTPL | FVTPL  |  any     |
//	| POCI             | POCI  | POCI   |  true    |
//	| unknown          | ERR   | ERR    |  n/a     |
//	+------------------+-------+--------+----------+

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveJurnalEventCode_AC(t *testing.T) {
	codes, err := ResolveJurnalEventCode(KlasifikasiAC, "IDR", false)
	require.Error(t, err)
	assert.Nil(t, codes)
	assert.Equal(t, ErrMTMInstrumenACSkip.Error(), err.Error())
}

func TestResolveJurnalEventCode_AC_FCY(t *testing.T) {
	// AC is rejected regardless of currency
	codes, err := ResolveJurnalEventCode(KlasifikasiAC, "USD", false)
	require.Error(t, err)
	assert.Nil(t, codes)
	assert.Equal(t, ErrMTMInstrumenACSkip.Error(), err.Error())
}

func TestResolveJurnalEventCode_AC_POCI(t *testing.T) {
	// AC + isPOCI=true: still AC → skip
	codes, err := ResolveJurnalEventCode(KlasifikasiAC, "IDR", true)
	require.Error(t, err)
	assert.Nil(t, codes)
}

// ─── FVOCI_DEBT ───────────────────────────────────────────────────────────────

func TestResolveJurnalEventCode_FVOCIDebt_IDR(t *testing.T) {
	// PSAK 71 §5.7.10, IDR only → single entry MTM_FVOCI
	codes, err := ResolveJurnalEventCode(KlasifikasiFVOCIDebt, "IDR", false)
	require.NoError(t, err)
	assert.Equal(t, []string{EventCodeMTMFVOCI}, codes)
}

func TestResolveJurnalEventCode_FVOCIDebt_FCY(t *testing.T) {
	// §B5.7.2A: two SEPARATE entries for FCY
	codes, err := ResolveJurnalEventCode(KlasifikasiFVOCIDebt, "USD", false)
	require.NoError(t, err)
	require.Len(t, codes, 2)
	assert.Equal(t, EventCodeMTMFVOCI, codes[0])
	assert.Equal(t, EventCodeMTMFXOCIReserve, codes[1])
}

func TestResolveJurnalEventCode_FVOCIDebt_EUR(t *testing.T) {
	// EUR (non-IDR) → same as USD: two entries
	codes, err := ResolveJurnalEventCode(KlasifikasiFVOCIDebt, "EUR", false)
	require.NoError(t, err)
	require.Len(t, codes, 2)
	assert.Equal(t, EventCodeMTMFVOCI, codes[0])
	assert.Equal(t, EventCodeMTMFXOCIReserve, codes[1])
}

func TestResolveJurnalEventCode_FVOCIDebt_JPY(t *testing.T) {
	// Another non-IDR currency
	codes, err := ResolveJurnalEventCode(KlasifikasiFVOCIDebt, "JPY", false)
	require.NoError(t, err)
	require.Len(t, codes, 2)
}

func TestResolveJurnalEventCode_FVOCIDebt_IDR_isPOCITrue(t *testing.T) {
	// isPOCI doesn't affect FVOCI_DEBT routing — klasifikasi wins
	codes, err := ResolveJurnalEventCode(KlasifikasiFVOCIDebt, "IDR", true)
	require.NoError(t, err)
	assert.Equal(t, []string{EventCodeMTMFVOCI}, codes)
}

// ─── FVOCI_ELECTION ──────────────────────────────────────────────────────────

func TestResolveJurnalEventCode_FVOCIElection_IDR(t *testing.T) {
	// §5.7.5: irrevocable election, single code, no P&L recycling
	codes, err := ResolveJurnalEventCode(KlasifikasiFVOCIElection, "IDR", false)
	require.NoError(t, err)
	assert.Equal(t, []string{EventCodeMTMFVOCIElection}, codes)
}

func TestResolveJurnalEventCode_FVOCIElection_FCY(t *testing.T) {
	// FCY FVOCI_ELECTION: all FX changes stay in OCI → still single code (no split)
	codes, err := ResolveJurnalEventCode(KlasifikasiFVOCIElection, "USD", false)
	require.NoError(t, err)
	assert.Equal(t, []string{EventCodeMTMFVOCIElection}, codes)
}

func TestResolveJurnalEventCode_FVOCIElection_FCY_isPOCI(t *testing.T) {
	codes, err := ResolveJurnalEventCode(KlasifikasiFVOCIElection, "USD", true)
	require.NoError(t, err)
	assert.Equal(t, []string{EventCodeMTMFVOCIElection}, codes)
}

// ─── FVTPL ───────────────────────────────────────────────────────────────────

func TestResolveJurnalEventCode_FVTPL_IDR(t *testing.T) {
	// §5.7.7: all FV changes (incl FX) → P&L
	codes, err := ResolveJurnalEventCode(KlasifikasiFVTPL, "IDR", false)
	require.NoError(t, err)
	assert.Equal(t, []string{EventCodeMTMFVTPL}, codes)
}

func TestResolveJurnalEventCode_FVTPL_FCY(t *testing.T) {
	// FCY FVTPL: FX component also to P&L (no OCI split)
	codes, err := ResolveJurnalEventCode(KlasifikasiFVTPL, "USD", false)
	require.NoError(t, err)
	assert.Equal(t, []string{EventCodeMTMFVTPL}, codes)
	assert.Len(t, codes, 1, "FVTPL FCY must NOT produce two entries — only MTM_FVTPL")
}

func TestResolveJurnalEventCode_FVTPL_FCY_isPOCI(t *testing.T) {
	codes, err := ResolveJurnalEventCode(KlasifikasiFVTPL, "EUR", true)
	require.NoError(t, err)
	assert.Equal(t, []string{EventCodeMTMFVTPL}, codes)
}

// ─── POCI ─────────────────────────────────────────────────────────────────────

func TestResolveJurnalEventCode_POCI_IDR(t *testing.T) {
	// OQ-M6-1: ECL is independent; MTM posts to MTM_FVTPL_POCI
	codes, err := ResolveJurnalEventCode(KlasifikasiPOCI, "IDR", true)
	require.NoError(t, err)
	assert.Equal(t, []string{EventCodeMTMFVTPLPOCI}, codes)
}

func TestResolveJurnalEventCode_POCI_FCY(t *testing.T) {
	codes, err := ResolveJurnalEventCode(KlasifikasiPOCI, "USD", true)
	require.NoError(t, err)
	assert.Equal(t, []string{EventCodeMTMFVTPLPOCI}, codes)
}

func TestResolveJurnalEventCode_POCI_isPOCIFalse(t *testing.T) {
	// isPOCI=false but klasifikasi=POCI: routing.go trusts klasifikasi
	codes, err := ResolveJurnalEventCode(KlasifikasiPOCI, "IDR", false)
	require.NoError(t, err)
	assert.Equal(t, []string{EventCodeMTMFVTPLPOCI}, codes)
}

// ─── Unknown / default branch ─────────────────────────────────────────────────

func TestResolveJurnalEventCode_Unknown(t *testing.T) {
	codes, err := ResolveJurnalEventCode("INVALID_KLASIFIKASI", "IDR", false)
	require.Error(t, err)
	assert.Nil(t, codes)
	assert.Equal(t, ErrUnknownKlasifikasi.Error(), err.Error())
}

func TestResolveJurnalEventCode_EmptyString(t *testing.T) {
	codes, err := ResolveJurnalEventCode("", "IDR", false)
	require.Error(t, err)
	assert.Nil(t, codes)
}

func TestResolveJurnalEventCode_EmptyMataUang(t *testing.T) {
	// Empty mataUang treated as IDR (not "IDR" string → no FCY branch)
	codes, err := ResolveJurnalEventCode(KlasifikasiFVOCIDebt, "", false)
	require.NoError(t, err)
	// "" != "IDR" so it triggers the FCY branch (2 codes)
	assert.Len(t, codes, 2)
}

// ─── Exported wrapper delegates to private function ───────────────────────────

func TestResolveJurnalEventCode_ExportedWrapper_Delegates(t *testing.T) {
	// ResolveJurnalEventCode must delegate to resolveJurnalEventCode identically.
	tests := []struct {
		klasifikasi string
		mataUang    string
		isPOCI      bool
	}{
		{KlasifikasiAC, "IDR", false},
		{KlasifikasiFVOCIDebt, "IDR", false},
		{KlasifikasiFVOCIDebt, "USD", false},
		{KlasifikasiFVOCIElection, "IDR", false},
		{KlasifikasiFVOCIElection, "EUR", false},
		{KlasifikasiFVTPL, "IDR", false},
		{KlasifikasiFVTPL, "USD", false},
		{KlasifikasiPOCI, "IDR", true},
		{"BOGUS", "IDR", false},
	}
	for _, tt := range tests {
		exportedCodes, exportedErr := ResolveJurnalEventCode(tt.klasifikasi, tt.mataUang, tt.isPOCI)
		privateCodes, privateErr := resolveJurnalEventCode(tt.klasifikasi, tt.mataUang, tt.isPOCI)

		if exportedErr != nil || privateErr != nil {
			if exportedErr == nil || privateErr == nil {
				t.Errorf("[%s/%s] error mismatch: exported=%v private=%v", tt.klasifikasi, tt.mataUang, exportedErr, privateErr)
			} else {
				assert.Equal(t, privateErr.Error(), exportedErr.Error(), "klasifikasi=%s mataUang=%s", tt.klasifikasi, tt.mataUang)
			}
		} else {
			assert.Equal(t, privateCodes, exportedCodes, "klasifikasi=%s mataUang=%s", tt.klasifikasi, tt.mataUang)
		}
	}
}

// ─── Sentinel error identity ──────────────────────────────────────────────────

func TestErrMTMInstrumenACSkip_IsNotErrUnknownKlasifikasi(t *testing.T) {
	// Two distinct sentinel errors — should not be equal.
	assert.NotEqual(t, ErrMTMInstrumenACSkip.Error(), ErrUnknownKlasifikasi.Error())
}

func TestErrMTMInstrumenACSkip_IsError(t *testing.T) {
	var e error = ErrMTMInstrumenACSkip
	assert.True(t, errors.Is(e, ErrMTMInstrumenACSkip))
}

func TestErrUnknownKlasifikasi_IsError(t *testing.T) {
	var e error = ErrUnknownKlasifikasi
	assert.True(t, errors.Is(e, ErrUnknownKlasifikasi))
}

// ─── Event code constant values (stable strings per migration 000040) ─────────

func TestEventCodeConstants(t *testing.T) {
	assert.Equal(t, "MTM_FVOCI", EventCodeMTMFVOCI)
	assert.Equal(t, "MTM_FX_OCI_RESERVE", EventCodeMTMFXOCIReserve)
	assert.Equal(t, "MTM_FVOCI_ELECTION", EventCodeMTMFVOCIElection)
	assert.Equal(t, "MTM_FVTPL", EventCodeMTMFVTPL)
	assert.Equal(t, "MTM_FVTPL_POCI", EventCodeMTMFVTPLPOCI)
}

func TestKlasifikasiConstants(t *testing.T) {
	assert.Equal(t, "AC", KlasifikasiAC)
	assert.Equal(t, "FVOCI_DEBT", KlasifikasiFVOCIDebt)
	assert.Equal(t, "FVOCI_ELECTION", KlasifikasiFVOCIElection)
	assert.Equal(t, "FVTPL", KlasifikasiFVTPL)
	assert.Equal(t, "POCI", KlasifikasiPOCI)
}

// ─── Return value invariants ──────────────────────────────────────────────────

func TestResolveJurnalEventCode_NeverReturnsEmptySliceOnSuccess(t *testing.T) {
	validCases := []struct {
		k string
		m string
	}{
		{KlasifikasiFVOCIDebt, "IDR"},
		{KlasifikasiFVOCIDebt, "USD"},
		{KlasifikasiFVOCIElection, "IDR"},
		{KlasifikasiFVOCIElection, "USD"},
		{KlasifikasiFVTPL, "IDR"},
		{KlasifikasiFVTPL, "USD"},
		{KlasifikasiPOCI, "IDR"},
	}
	for _, tc := range validCases {
		codes, err := ResolveJurnalEventCode(tc.k, tc.m, false)
		require.NoError(t, err, "klasifikasi=%s mataUang=%s", tc.k, tc.m)
		assert.NotEmpty(t, codes, "must return ≥1 event code for klasifikasi=%s", tc.k)
	}
}

func TestResolveJurnalEventCode_OnlyFVOCIDebtFCYReturnsTwoCodes(t *testing.T) {
	// Exactly one case returns 2 codes: FVOCI_DEBT + non-IDR.
	cases2 := []struct{ k, m string }{{KlasifikasiFVOCIDebt, "USD"}, {KlasifikasiFVOCIDebt, "SGD"}}
	cases1 := []struct{ k, m string }{
		{KlasifikasiFVOCIDebt, "IDR"},
		{KlasifikasiFVOCIElection, "USD"},
		{KlasifikasiFVTPL, "USD"},
		{KlasifikasiPOCI, "USD"},
	}
	for _, tc := range cases2 {
		codes, _ := ResolveJurnalEventCode(tc.k, tc.m, false)
		assert.Len(t, codes, 2, "%s/%s must return 2 codes", tc.k, tc.m)
	}
	for _, tc := range cases1 {
		codes, _ := ResolveJurnalEventCode(tc.k, tc.m, false)
		assert.Len(t, codes, 1, "%s/%s must return exactly 1 code", tc.k, tc.m)
	}
}
