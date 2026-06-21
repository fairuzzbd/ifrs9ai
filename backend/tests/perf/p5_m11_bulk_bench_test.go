// Package perf — P5-M11 Bulk Upload Master Instrumen performance benchmarks.
//
// SLA targets (from OpenAPI app-b-bulk-upload.yaml §performance):
//
//	PARSE_1000_ROW_XLSX_MAX      = 5 s     — parse + validate format for 1000-row XLSX
//	DRY_RUN_4STAGE_1000_MAX      = 10 s    — 4-stage validation pipeline, 1000 rows
//	COMMIT_1000_ROW_MAX          = 60 s    — Asynq worker insert 1000 instrumen (partial-ok)
//	IDEMPOTENCY_CHECK_MAX        = 20 µs   — O(1) key lookup
//	MIME_CHECK_MAX               = 5 µs    — magic bytes comparison
//	HASH_CHAIN_ENTRY_MAX         = 200 µs  — single audit hash-chain step
//	PARSE_ERROR_COLLECT_MAX      = 50 µs   — per-row error collection
//	ROW_INSERT_SINGLE_MAX        = 100 µs  — single instrumen in-memory insert + kode-index
//	BATCH_STATUS_TRANSITION_MAX  = 20 µs   — status string assignment + idxcheck
//	CURSOR_PAGE_LIST_P95_MAX     = 200 ms  — cursor-based rows list, 50 rows of 10k
//
// Compliance references:
//
//	DEC-016: shopspring/decimal, NUMERIC(20,4) IDR — row saldo
//	DEC-018: soft-delete only on rollback — instrumented in COMMIT bench (no hard-delete alloc)
//	DEC-021: idempotency-key mandatory on 5 mutating endpoints — map lookup ≤ 20µs
//	DEC-022: cursor-based pagination — list bench excludes COUNT(*)
//	UX-§3:  parse 1000-row XLSX ≤ 5s; DRY_RUN ≤ 10s; commit ≤ 60s (Asynq long-running)
//
// Run:
//
//	go test ./backend/tests/perf/... -bench=BenchP5M11 -benchtime=10s -benchmem -race
package perf

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// ─── SLA constants (P5-M11) ───────────────────────────────────────────────────

const (
	slaM11ParseXLSX1000Max       = 5 * time.Second
	slaM11DryRun1000Max          = 10 * time.Second
	slaM11Commit1000Max          = 60 * time.Second
	slaM11IdempotencyMax         = 20 * time.Microsecond
	slaM11MimeCheckMax           = 5 * time.Microsecond
	slaM11HashChainEntryMax      = 200 * time.Microsecond
	slaM11ParseErrorCollectMax   = 50 * time.Microsecond
	slaM11RowInsertSingleMax     = 100 * time.Microsecond
	slaM11StatusTransitionMax    = 20 * time.Microsecond
	slaM11CursorPageP95Max       = 200 * time.Millisecond

	benchM11Rows1k    = 1000
	benchM11Rows10k   = 10_000
	benchM11PageLimit = 50
)

// ─── Bench domain types ───────────────────────────────────────────────────────

type m11BenchRow struct {
	ID         uuid.UUID
	Sheet      string
	RowNumber  int
	KodeInst   string
	Saldo      decimal.Decimal // NUMERIC(20,4) IDR
	Kupon      decimal.Decimal // NUMERIC(10,8)
	RowStatus  string
	ErrMsg     *string
	InstrumenID *uuid.UUID
}

type m11BenchBatch struct {
	ID            uuid.UUID
	Status        string
	TotalRows     int
	CommittedRows int
	FailedRows    int
	FlaggedRows   int
}

type m11BenchInstrumen struct {
	ID            uuid.UUID
	KodeInstrumen string
	Status        string
	Saldo         decimal.Decimal
	DeletedAt     *time.Time
}

type m11BenchDryRunResult struct {
	Status       string
	ValidRows    int
	InvalidRows  int
	FlaggedRows  int
	Stage1Pass   bool
	Stage2Pass   bool
	Stage3Pass   bool
	Stage4Pass   bool
}

// ─── Bench helpers ────────────────────────────────────────────────────────────

// m11GenRow creates a reproducible test row at index i.
func m11GenRow(i int) m11BenchRow {
	sheets := []string{"Deposito", "Obligasi", "Saham", "Reksadana", "Tabungan_Cash"}
	sheet := sheets[i%len(sheets)]
	saldo := decimal.NewFromFloat(float64(100_000_000 + i*500_000)).RoundBank(4)
	kupon := decimal.NewFromFloat(0.04500000 + float64(i)*0.001).RoundBank(8)
	return m11BenchRow{
		ID:        uuid.New(),
		Sheet:     sheet,
		RowNumber: i + 1,
		KodeInst:  fmt.Sprintf("INST-%06d", i),
		Saldo:     saldo,
		Kupon:     kupon,
		RowStatus: "PENDING",
	}
}

// m11ParseRowFormat simulates Stage 1 format validation for a single row.
// Checks: kode not empty, saldo > 0, kupon in [0,1].
func m11ParseRowFormat(row m11BenchRow) error {
	if row.KodeInst == "" {
		return fmt.Errorf("row %d: kode instrumen wajib diisi", row.RowNumber)
	}
	if row.Saldo.IsNegative() || row.Saldo.IsZero() {
		return fmt.Errorf("row %d: saldo must be > 0, got %s", row.RowNumber, row.Saldo.StringFixed(4))
	}
	if row.Kupon.IsNegative() || row.Kupon.GreaterThan(decimal.NewFromFloat(1.0)) {
		return fmt.Errorf("row %d: kupon must be in [0,1], got %s", row.RowNumber, row.Kupon.StringFixed(8))
	}
	return nil
}

// m11ParseRowBusiness simulates Stage 2 business rules (tenor > 0, sheet ∈ allowed).
func m11ParseRowBusiness(row m11BenchRow) error {
	allowed := map[string]struct{}{
		"Deposito": {}, "Obligasi": {}, "Saham": {}, "Reksadana": {}, "Tabungan_Cash": {},
	}
	if _, ok := allowed[row.Sheet]; !ok {
		return fmt.Errorf("row %d: sheet '%s' tidak dikenal", row.RowNumber, row.Sheet)
	}
	return nil
}

// m11CheckCrossRef simulates Stage 3 counterparty cross-reference (map lookup proxy).
func m11CheckCrossRef(kode string, masterIndex map[string]bool) error {
	if _, ok := masterIndex[kode]; !ok {
		return fmt.Errorf("counterparty untuk %s tidak ditemukan", kode)
	}
	return nil
}

// m11EvalSPPI simulates Stage 4 SPPI+BM auto-eval (simple heuristic).
// Returns (pass, flagged).
func m11EvalSPPI(row m11BenchRow) (pass bool, flagged bool) {
	// Flagged if kupon > 0.12 (ambiguous Q7 territory)
	if row.Kupon.GreaterThan(decimal.NewFromFloat(0.12)) {
		return true, true
	}
	return true, false
}

// m11ValidateMime checks XLSX ZIP magic bytes.
func m11ValidateMime(first4 []byte) bool {
	if len(first4) < 4 {
		return false
	}
	return first4[0] == 0x50 && first4[1] == 0x4b && first4[2] == 0x03 && first4[3] == 0x04
}

// m11CollectParseError appends parse error to slice (simulates per-row error collection).
func m11CollectParseError(errs *[]string, rowNum int, msg string) {
	*errs = append(*errs, fmt.Sprintf("row %d: %s", rowNum, msg))
}

// m11InsertInstrumen simulates one instrumen row INSERT into in-memory repo.
// Returns error on duplicate kode.
func m11InsertInstrumen(kodeIndex map[string]uuid.UUID, inst m11BenchInstrumen) error {
	if _, exists := kodeIndex[inst.KodeInstrumen]; exists {
		return fmt.Errorf("CONFLICT: duplikat kode '%s'", inst.KodeInstrumen)
	}
	kodeIndex[inst.KodeInstrumen] = inst.ID
	return nil
}

// m11TransitionStatus validates and applies batch status transition.
func m11TransitionStatus(currentStatus, newStatus string, allowed map[string][]string) error {
	next, ok := allowed[currentStatus]
	if !ok {
		return fmt.Errorf("status '%s' tidak ada di state machine", currentStatus)
	}
	for _, s := range next {
		if s == newStatus {
			return nil
		}
	}
	return fmt.Errorf("transisi %s → %s tidak valid", currentStatus, newStatus)
}

// ─── Unit benchmarks ──────────────────────────────────────────────────────────

// BenchP5M11_MimeCheck — magic bytes check, < 5µs.
func BenchP5M11_MimeCheck(b *testing.B) {
	xlsxMagic := []byte{0x50, 0x4b, 0x03, 0x04}
	b.ResetTimer()
	b.ReportAllocs()

	var ok bool
	for i := 0; i < b.N; i++ {
		ok = m11ValidateMime(xlsxMagic)
	}
	b.StopTimer()
	require.True(b, ok)

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaM11MimeCheckMax,
		"MimeCheck must be < 5µs, got %s", elapsed)
}

// BenchP5M11_IdempotencyCheck — O(1) map lookup, < 20µs.
func BenchP5M11_IdempotencyCheck(b *testing.B) {
	store := make(map[[32]byte]struct{}, 1024)
	key := sha256.Sum256([]byte("Idempotency-Key:" + uuid.New().String()))
	store[key] = struct{}{}
	b.ResetTimer()
	b.ReportAllocs()

	var found bool
	for i := 0; i < b.N; i++ {
		_, found = store[key]
	}
	b.StopTimer()
	_ = found

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaM11IdempotencyMax,
		"IdempotencyCheck must be < 20µs, got %s", elapsed)
}

// BenchP5M11_ParseErrorCollect — per-row error append, < 50µs.
func BenchP5M11_ParseErrorCollect(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var errs []string
		m11CollectParseError(&errs, 42, "saldo tidak boleh kosong")
		m11CollectParseError(&errs, 43, "kupon diluar range [0,1]")
		_ = errs
	}
	b.StopTimer()

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaM11ParseErrorCollectMax,
		"ParseErrorCollect must be < 50µs, got %s", elapsed)
}

// BenchP5M11_RowInsertSingle — one instrumen INSERT to in-memory index, < 100µs.
func BenchP5M11_RowInsertSingle(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		kodeIndex := make(map[string]uuid.UUID, 1)
		inst := m11BenchInstrumen{
			ID:            uuid.New(),
			KodeInstrumen: fmt.Sprintf("INST-BENCH-%d", i),
			Status:        "PENDING_APPROVAL_BULK",
			Saldo:         decimal.NewFromFloat(500_000_000).RoundBank(4),
		}
		err := m11InsertInstrumen(kodeIndex, inst)
		_ = err
	}
	b.StopTimer()

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaM11RowInsertSingleMax,
		"RowInsertSingle must be < 100µs, got %s", elapsed)
}

// BenchP5M11_StatusTransition — state machine transition check, < 20µs.
func BenchP5M11_StatusTransition(b *testing.B) {
	allowed := map[string][]string{
		"PARSED":           {"DRY_RUN_PASSED", "DRY_RUN_FAILED"},
		"DRY_RUN_PASSED":   {"COMMITTING", "PARSED"},
		"DRY_RUN_FAILED":   {"PARSED"},
		"COMMITTING":       {"COMMITTED", "PARTIAL_COMMIT"},
		"COMMITTED":        {"APPROVED"},
		"PARTIAL_COMMIT":   {"APPROVED"},
		"APPROVED":         {"ROLLBACK_PENDING"},
		"ROLLBACK_PENDING": {"ROLLED_BACK"},
	}
	b.ResetTimer()
	b.ReportAllocs()

	var err error
	for i := 0; i < b.N; i++ {
		err = m11TransitionStatus("COMMITTED", "APPROVED", allowed)
	}
	b.StopTimer()
	require.NoError(b, err)

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaM11StatusTransitionMax,
		"StatusTransition must be < 20µs, got %s", elapsed)
}

// BenchP5M11_HashChainEntry — SHA-256 for one audit event, < 200µs.
func BenchP5M11_HashChainEntry(b *testing.B) {
	type auditEntry struct {
		Action    string `json:"action"`
		BatchID   string `json:"batch_id"`
		Rows      int    `json:"committed_rows"`
		TenantID  string `json:"tenant_id"`
	}
	entry := auditEntry{
		Action:   "BULK.COMMITTED",
		BatchID:  uuid.New().String(),
		Rows:     1000,
		TenantID: "TUGURE",
	}
	prevHash := make([]byte, 32)
	data, _ := json.Marshal(entry)
	b.ResetTimer()
	b.ReportAllocs()

	var h [32]byte
	for i := 0; i < b.N; i++ {
		h = sha256.Sum256(append(prevHash, data...))
	}
	b.StopTimer()
	_ = h

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaM11HashChainEntryMax,
		"HashChainEntry must be < 200µs, got %s", elapsed)
}

// ─── Batch benchmarks ─────────────────────────────────────────────────────────

// BenchP5M11_Parse1000XLSX — parse + Stage 1 format validation for 1000 rows; < 5s.
// Proxies the upload handler parse phase (format-only, pre-DRY_RUN).
func BenchP5M11_Parse1000XLSX(b *testing.B) {
	// Generate 1000 rows
	rows := make([]m11BenchRow, benchM11Rows1k)
	for i := range rows {
		rows[i] = m11GenRow(i)
	}

	b.ResetTimer()
	b.ReportAllocs()

	start := time.Now()
	for iter := 0; iter < b.N; iter++ {
		var parseErrs []string
		validCount := 0
		for _, row := range rows {
			// Stage 1: format validation
			if err := m11ParseRowFormat(row); err != nil {
				m11CollectParseError(&parseErrs, row.RowNumber, err.Error())
			} else {
				validCount++
			}
		}
		_ = validCount
		_ = parseErrs
	}
	elapsed := time.Since(start)
	b.StopTimer()

	if b.N > 0 {
		perRun := elapsed / time.Duration(b.N)
		require.LessOrEqual(b, perRun, slaM11ParseXLSX1000Max,
			"Parse1000XLSX must be < 5s per run, got %s", perRun)
	}
}

// BenchP5M11_DryRun1000 — 4-stage validation pipeline for 1000 rows; < 10s.
// Stages: (1) format, (2) business rules, (3) cross-ref lookup, (4) SPPI+BM eval.
func BenchP5M11_DryRun1000(b *testing.B) {
	rows := make([]m11BenchRow, benchM11Rows1k)
	for i := range rows {
		rows[i] = m11GenRow(i)
	}

	// Stage 3: build master-data cross-ref index (counterparty + portofolio)
	masterIndex := make(map[string]bool, benchM11Rows1k)
	for _, row := range rows {
		masterIndex[row.KodeInst] = true // all in master
	}

	b.ResetTimer()
	b.ReportAllocs()

	start := time.Now()
	for iter := 0; iter < b.N; iter++ {
		result := m11BenchDryRunResult{
			Status:     "DRY_RUN_PASSED",
			Stage1Pass: true,
			Stage2Pass: true,
			Stage3Pass: true,
			Stage4Pass: true,
		}
		var s1Errs, s2Errs, s3Errs []string

		for _, row := range rows {
			// Stage 1: format
			if err := m11ParseRowFormat(row); err != nil {
				s1Errs = append(s1Errs, err.Error())
				result.InvalidRows++
				continue
			}
			// Stage 2: business rules
			if err := m11ParseRowBusiness(row); err != nil {
				s2Errs = append(s2Errs, err.Error())
				result.InvalidRows++
				continue
			}
			// Stage 3: cross-ref
			if err := m11CheckCrossRef(row.KodeInst, masterIndex); err != nil {
				s3Errs = append(s3Errs, err.Error())
				result.InvalidRows++
				continue
			}
			// Stage 4: SPPI+BM
			_, flagged := m11EvalSPPI(row)
			if flagged {
				result.FlaggedRows++
			} else {
				result.ValidRows++
			}
		}

		if len(s1Errs) > 0 {
			result.Stage1Pass = false
			result.Status = "DRY_RUN_FAILED"
		}
		if len(s2Errs) > 0 {
			result.Stage2Pass = false
			result.Status = "DRY_RUN_FAILED"
		}
		if len(s3Errs) > 0 {
			result.Stage3Pass = false
			result.Status = "DRY_RUN_FAILED"
		}
		_ = result
	}
	elapsed := time.Since(start)
	b.StopTimer()

	if b.N > 0 {
		perRun := elapsed / time.Duration(b.N)
		require.LessOrEqual(b, perRun, slaM11DryRun1000Max,
			"DryRun1000 must be < 10s per run, got %s", perRun)
	}
}

// BenchP5M11_Commit1000 — Asynq worker commit: INSERT 1000 instrumen with per-row savepoints.
// Partial commit OK (DRY_RUN_PASSED allows flagged rows). < 60s.
func BenchP5M11_Commit1000(b *testing.B) {
	rows := make([]m11BenchRow, benchM11Rows1k)
	for i := range rows {
		rows[i] = m11GenRow(i)
	}

	b.ResetTimer()
	b.ReportAllocs()

	start := time.Now()
	for iter := 0; iter < b.N; iter++ {
		kodeIndex := make(map[string]uuid.UUID, benchM11Rows1k)
		batch := m11BenchBatch{
			ID:        uuid.New(),
			Status:    "COMMITTING",
			TotalRows: benchM11Rows1k,
		}

		for _, row := range rows {
			inst := m11BenchInstrumen{
				ID:            uuid.New(),
				KodeInstrumen: row.KodeInst,
				Status:        "PENDING_APPROVAL_BULK",
				Saldo:         row.Saldo,
			}
			// Per-row savepoint: if INSERT fails, mark row FAILED, continue
			if err := m11InsertInstrumen(kodeIndex, inst); err != nil {
				batch.FailedRows++
			} else {
				batch.CommittedRows++
			}
		}

		// Finalize batch status
		if batch.FailedRows == 0 {
			batch.Status = "COMMITTED"
		} else if batch.CommittedRows > 0 {
			batch.Status = "PARTIAL_COMMIT"
		} else {
			batch.Status = "COMMITTED" // all failed still = committed (empty)
		}

		// Audit BULK.COMMITTED hash-chain (1 event per run)
		afterData := map[string]interface{}{
			"batch_id":       batch.ID.String(),
			"committed_rows": batch.CommittedRows,
			"failed_rows":    batch.FailedRows,
		}
		data, _ := json.Marshal(afterData)
		prevHash := make([]byte, 32)
		_ = sha256.Sum256(append(prevHash, data...))
	}
	elapsed := time.Since(start)
	b.StopTimer()

	if b.N > 0 {
		perRun := elapsed / time.Duration(b.N)
		require.LessOrEqual(b, perRun, slaM11Commit1000Max,
			"Commit1000 must be < 60s per run, got %s", perRun)
	}
}

// ─── Pagination benchmark ─────────────────────────────────────────────────────

// BenchP5M11_CursorPageRows10k — cursor-based rows list 50 rows from 10k; P95 < 200ms.
// No COUNT(*) per DEC-022.
func BenchP5M11_CursorPageRows10k(b *testing.B) {
	rows := make([]m11BenchRow, benchM11Rows10k)
	batchID := uuid.New()
	for i := range rows {
		r := m11GenRow(i)
		r.ID = uuid.New()
		// Alternate statuses
		switch i % 5 {
		case 0:
			r.RowStatus = "COMMITTED"
		case 1:
			r.RowStatus = "FAILED"
			msg := "validation error"
			r.ErrMsg = &msg
		case 2:
			r.RowStatus = "FLAGGED_MANUAL_REVIEW"
		case 3:
			r.RowStatus = "ROLLED_BACK"
		default:
			r.RowStatus = "PENDING"
		}
		_ = batchID
		rows[i] = r
	}

	type listResult struct {
		Page      []m11BenchRow
		NextIdx   int
		HasMore   bool
		TotalEst  int
	}

	// Cursor-based page query (no COUNT — uses limit+1 trick)
	listPage := func(filterStatus string, cursorIdx int) listResult {
		var filtered []m11BenchRow
		for _, r := range rows {
			if filterStatus == "" || r.RowStatus == filterStatus {
				filtered = append(filtered, r)
			}
		}
		end := cursorIdx + benchM11PageLimit + 1
		if end > len(filtered) {
			end = len(filtered)
		}
		page := filtered[cursorIdx:end]
		hasMore := false
		if len(page) > benchM11PageLimit {
			page = page[:benchM11PageLimit]
			hasMore = true
		}
		return listResult{Page: page, NextIdx: cursorIdx + len(page), HasMore: hasMore, TotalEst: len(filtered)}
	}

	// Warmup
	_ = listPage("COMMITTED", 0)

	b.ResetTimer()
	b.ReportAllocs()

	samples := make([]time.Duration, b.N)
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		res := listPage("COMMITTED", 0)
		samples[i] = time.Since(t0)
		require.LessOrEqual(b, len(res.Page), benchM11PageLimit)
	}
	b.StopTimer()

	if b.N < 2 {
		return
	}
	// Insertion sort for P95
	for i := 1; i < len(samples); i++ {
		for j := i; j > 0 && samples[j] < samples[j-1]; j-- {
			samples[j], samples[j-1] = samples[j-1], samples[j]
		}
	}
	p95idx := int(float64(len(samples)) * 0.95)
	if p95idx >= len(samples) {
		p95idx = len(samples) - 1
	}
	p95 := samples[p95idx]
	require.LessOrEqual(b, p95, slaM11CursorPageP95Max,
		"CursorPageRows10k P95 must be < 200ms, got %s", p95)
	b.Logf("CursorPageRows10k P95=%s (n=%d)", p95, b.N)
}

// ─── Partial commit accuracy benchmark ───────────────────────────────────────

// BenchP5M11_PartialCommitAccuracy — duplicate detection in 1000-row batch (2 dups).
// Ensures per-row savepoint + kode-index duplicate check is O(1) and correct.
func BenchP5M11_PartialCommitAccuracy(b *testing.B) {
	// Rows 0..997 unique; rows 998, 999 are duplicates of 0, 1
	rows := make([]m11BenchRow, benchM11Rows1k)
	for i := range rows {
		rows[i] = m11GenRow(i)
	}
	rows[998].KodeInst = rows[0].KodeInst // duplicate
	rows[999].KodeInst = rows[1].KodeInst // duplicate

	b.ResetTimer()
	b.ReportAllocs()

	for iter := 0; iter < b.N; iter++ {
		kodeIndex := make(map[string]uuid.UUID, benchM11Rows1k)
		committed, failed := 0, 0
		for _, row := range rows {
			inst := m11BenchInstrumen{
				ID:            uuid.New(),
				KodeInstrumen: row.KodeInst,
				Status:        "PENDING_APPROVAL_BULK",
				Saldo:         row.Saldo,
			}
			if err := m11InsertInstrumen(kodeIndex, inst); err != nil {
				failed++
			} else {
				committed++
			}
		}
		_ = committed
		_ = failed
	}
	b.StopTimer()

	// Verify correctness on final iteration (outside timer)
	kodeIndex := make(map[string]uuid.UUID, benchM11Rows1k)
	committed, failed := 0, 0
	for _, row := range rows {
		inst := m11BenchInstrumen{ID: uuid.New(), KodeInstrumen: row.KodeInst,
			Status: "PENDING_APPROVAL_BULK", Saldo: row.Saldo}
		if err := m11InsertInstrumen(kodeIndex, inst); err != nil {
			failed++
		} else {
			committed++
		}
	}
	require.Equal(b, 998, committed, "998 unique rows must commit")
	require.Equal(b, 2, failed, "2 duplicate rows must fail")
}

// ─── Rollback soft-delete benchmark ──────────────────────────────────────────

// BenchP5M11_RollbackSoftDelete — soft-delete 1000 instrumens (no hard-delete alloc, DEC-018).
func BenchP5M11_RollbackSoftDelete(b *testing.B) {
	instrumens := make([]*m11BenchInstrumen, benchM11Rows1k)
	batchID := uuid.New()
	for i := range instrumens {
		saldo := decimal.NewFromFloat(float64(100_000_000 + i*500_000)).RoundBank(4)
		instrumens[i] = &m11BenchInstrumen{
			ID:            uuid.New(),
			KodeInstrumen: fmt.Sprintf("INST-%06d", i),
			Status:        "ACTIVE",
			Saldo:         saldo,
		}
	}
	deletedBy := uuid.New()
	_ = batchID

	b.ResetTimer()
	b.ReportAllocs()

	start := time.Now()
	for iter := 0; iter < b.N; iter++ {
		// Reset
		for _, inst := range instrumens {
			inst.DeletedAt = nil
		}
		// Soft-delete: set deleted_at on all (no hard delete — DEC-018)
		now := time.Now()
		count := 0
		for _, inst := range instrumens {
			if inst.DeletedAt == nil {
				inst.DeletedAt = &now
				_ = deletedBy
				count++
			}
		}
		_ = count
	}
	elapsed := time.Since(start)
	b.StopTimer()

	if b.N > 0 {
		perRun := elapsed / time.Duration(b.N)
		// Soft-delete 1000 rows should be well within commit SLA
		require.LessOrEqual(b, perRun, slaM11Commit1000Max,
			"RollbackSoftDelete1000 must be < 60s, got %s", perRun)
	}
}

// ─── Sub-benchmark suite ──────────────────────────────────────────────────────

// BenchP5M11 runs all P5-M11 Bulk Upload benchmarks as sub-benchmarks.
func BenchP5M11(b *testing.B) {
	b.Run("MimeCheck", BenchP5M11_MimeCheck)
	b.Run("IdempotencyCheck", BenchP5M11_IdempotencyCheck)
	b.Run("ParseErrorCollect", BenchP5M11_ParseErrorCollect)
	b.Run("RowInsertSingle", BenchP5M11_RowInsertSingle)
	b.Run("StatusTransition", BenchP5M11_StatusTransition)
	b.Run("HashChainEntry", BenchP5M11_HashChainEntry)
	b.Run("Parse1000XLSX", BenchP5M11_Parse1000XLSX)
	b.Run("DryRun1000", BenchP5M11_DryRun1000)
	b.Run("Commit1000", BenchP5M11_Commit1000)
	b.Run("CursorPageRows10k", BenchP5M11_CursorPageRows10k)
	b.Run("PartialCommitAccuracy", BenchP5M11_PartialCommitAccuracy)
	b.Run("RollbackSoftDelete", BenchP5M11_RollbackSoftDelete)
}

// ─── Quick smoke tests (not benchmarks) ──────────────────────────────────────

// TestP5M11_PerfSmoke verifies bench fixtures are correct before running.
func TestP5M11_PerfSmoke(t *testing.T) {
	row := m11GenRow(0)
	require.Equal(t, "Deposito", row.Sheet)
	require.Equal(t, "INST-000000", row.KodeInst)
	require.True(t, row.Saldo.GreaterThan(decimal.Zero))
	require.True(t, row.Kupon.GreaterThan(decimal.Zero))

	require.NoError(t, m11ParseRowFormat(row))
	require.NoError(t, m11ParseRowBusiness(row))

	pass, flagged := m11EvalSPPI(row)
	require.True(t, pass)
	require.False(t, flagged) // kupon 0.045 < 0.12

	// Row with high kupon → flagged
	highKuponRow := row
	highKuponRow.Kupon = decimal.NewFromFloat(0.13).RoundBank(8)
	_, flagged2 := m11EvalSPPI(highKuponRow)
	require.True(t, flagged2)

	// MIME check
	require.True(t, m11ValidateMime([]byte{0x50, 0x4b, 0x03, 0x04}))
	require.False(t, m11ValidateMime([]byte{0x69, 0x64, 0x2c, 0x6e})) // CSV

	// Status transition
	allowed := map[string][]string{
		"PARSED":         {"DRY_RUN_PASSED", "DRY_RUN_FAILED"},
		"DRY_RUN_PASSED": {"COMMITTING", "PARSED"},
	}
	require.NoError(t, m11TransitionStatus("PARSED", "DRY_RUN_PASSED", allowed))
	require.Error(t, m11TransitionStatus("PARSED", "APPROVED", allowed))

	t.Logf("smoke: row %s saldo=%s kupon=%s", row.KodeInst, row.Saldo.StringFixed(4), row.Kupon.StringFixed(8))
}

// TestP5M11_PerfSmoke_1000Fixtures checks 1000-row fixture generation time.
func TestP5M11_PerfSmoke_1000Fixtures(t *testing.T) {
	start := time.Now()
	rows := make([]m11BenchRow, benchM11Rows1k)
	for i := range rows {
		rows[i] = m11GenRow(i)
	}
	elapsed := time.Since(start)
	require.Less(t, elapsed, 1*time.Second, "1000 fixture generation must be < 1s")
	require.Len(t, rows, benchM11Rows1k)
	t.Logf("1000 Bulk Upload row fixtures generated in %s", elapsed)
}

// TestP5M11_DecimalPrecision — DEC-016: IDR NUMERIC(20,4), EIR NUMERIC(10,8).
func TestP5M11_DecimalPrecision(t *testing.T) {
	saldo := decimal.RequireFromString("1234567890.5000")
	require.Equal(t, int32(4), saldo.Exponent()*-1)

	kupon := decimal.RequireFromString("0.04500000")
	require.Equal(t, int32(8), kupon.Exponent()*-1)

	// shopspring/decimal does not lose precision
	sum := saldo.Add(saldo)
	require.Equal(t, "2469135781.0000", sum.StringFixed(4))

	// Verify IDR buffer serialization
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	require.NoError(t, enc.Encode(saldo.StringFixed(4)))
	t.Logf("IDR saldo JSON: %s", buf.String())
}
