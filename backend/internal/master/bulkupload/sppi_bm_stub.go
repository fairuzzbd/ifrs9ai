package bulkupload

// sppi_bm_stub.go — Phase 3 SPPI+BM auto-eval stub for P5-M11.
//
// The real Phase 3 service is not yet available in P5.
// This stub returns default klasifikasi='AC' for all rows.
// Wire the real service in main.go via NewService(... , realEvaluator).
//
// NEVER import ECL/EIR packages here — P5-M11 does not touch ECL compute.

// stubSPPIBMEvaluator is the Phase 3 stub implementing SPPIBMEvaluator.
type stubSPPIBMEvaluator struct{}

// NewStubSPPIBMEvaluator creates the default stub evaluator.
// Used in P5-M11 until Phase 3 service is available.
func NewStubSPPIBMEvaluator() SPPIBMEvaluator {
	return &stubSPPIBMEvaluator{}
}

// Evaluate returns default AC klasifikasi for all rows (Phase 3 stub).
// Real evaluator will assess SPPI Q1-Q10 + BM category per row.
func (s *stubSPPIBMEvaluator) Evaluate(sheetName SheetName, rowData map[string]interface{}) (*KlasifikasiResult, error) {
	klsf := "AC"
	return &KlasifikasiResult{
		KlasifikasiPsak71: &klsf,
		SppiResult:        "PASS",
		BmResult:          "HTC",
		Ambiguous:         false,
		FlagReason:        "",
	}, nil
}
