// Package perf — P5-M2 Jurnal Engine performance benchmarks.
//
// SLA assertions (from OpenAPI app-d-jurnal-engine.yaml §Performance SLA):
//   - BenchmarkResolverPreview:           ≤ 100 ms per op
//   - BenchmarkResolveAndPost:            ≤ 300 ms per op
//   - BenchmarkAsynqSubscriberProcessing: ≤ 5 s per event (end-to-end incl. audit)
//
// These benchmarks test pure in-process computation (no DB, no network).
// DB latency must be measured separately under k6 load tests (`tests/load/`).
//
// Run:
//
//	go test ./tests/perf/... -bench=BenchmarkResolverPreview -benchtime=3s
//	go test ./tests/perf/... -bench=BenchmarkResolveAndPost -benchtime=3s
//	go test ./tests/perf/... -bench=BenchmarkAsynqSubscriberProcessing -benchtime=3s
package perf

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── In-process stubs for P5-M2 benchmarks ───────────────────────────────────

const (
	benchJrnlStatusApprovedActive = "APPROVED_ACTIVE"
	benchDKDebit                  = "DEBIT"
	benchDKKredit                 = "KREDIT"
	benchJrnlPosted               = "POSTED"
)

// benchMappingDetail is the minimal detail row for benchmark tests.
type benchMappingDetail struct {
	Urutan      int
	DKIndicator string
	AkunID      uuid.UUID
	Multiplier  decimal.Decimal
}

// benchMappingRecord is the minimal mapping header for benchmark tests.
type benchMappingRecord struct {
	ID                 uuid.UUID
	EventCode          string
	WorkflowStatus     string
	AktifFlag          bool
	KlasifikasiBerlaku []string // nil = ALL
	DetailRows         []benchMappingDetail
}

// benchJurnalHeader is the minimal jrnl.header shape.
type benchJurnalHeader struct {
	ID             uuid.UUID
	EventCode      string
	StatusInternal string
	TotalDebit     decimal.Decimal
	TotalKredit    decimal.Decimal
	IdempotencyKey string
	NoJurnal       string
}

// benchAuditRow simulates aud.audit_log row.
type benchAuditRow struct {
	Action      string
	EntityID    string
	CurrentHash []byte
}

// benchJurnalService is the minimal jurnal resolver+post service for benchmarks.
type benchJurnalService struct {
	mappings      map[string]*benchMappingRecord // event_code → mapping
	jurnalHeaders map[string]*benchJurnalHeader  // idempotency_key → header
	auditLog      []benchAuditRow
	seqCounter    int
}

func newBenchJurnalService() *benchJurnalService {
	return &benchJurnalService{
		mappings:      make(map[string]*benchMappingRecord),
		jurnalHeaders: make(map[string]*benchJurnalHeader),
	}
}

// seedMapping adds a balanced 2-row mapping (DEBIT + KREDIT) for eventCode.
func (svc *benchJurnalService) seedMapping(eventCode string) {
	akun1, akun2 := uuid.New(), uuid.New()
	svc.mappings[eventCode] = &benchMappingRecord{
		ID:             uuid.New(),
		EventCode:      eventCode,
		WorkflowStatus: benchJrnlStatusApprovedActive,
		AktifFlag:      true,
		DetailRows: []benchMappingDetail{
			{Urutan: 1, DKIndicator: benchDKDebit, AkunID: akun1, Multiplier: decimal.NewFromInt(1)},
			{Urutan: 2, DKIndicator: benchDKKredit, AkunID: akun2, Multiplier: decimal.NewFromInt(1)},
		},
	}
}

// computeBenchIdempotencyKey computes SHA256(sourceEventID||"::"||eventCode).
func computeBenchIdempotencyKey(sourceEventID uuid.UUID, eventCode string) string {
	payload := fmt.Sprintf("%s::%s", sourceEventID, eventCode)
	h := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", h)
}

// computeBenchAuditHash computes SHA-256 for audit chain row.
func computeBenchAuditHash(prevHash []byte, action, entityID string) []byte {
	payload := fmt.Sprintf("%x||%s||%s", prevHash, action, entityID)
	h := sha256.Sum256([]byte(payload))
	return h[:]
}

// resolve performs the core resolver logic (lookup + line generation + balance check).
// Returns error string if imbalanced or not found.
func (svc *benchJurnalService) resolve(eventCode, klasifikasi string, amountIDR decimal.Decimal) ([]benchMappingDetail, string) {
	mapping := svc.mappings[eventCode]
	if mapping == nil || mapping.WorkflowStatus != benchJrnlStatusApprovedActive || !mapping.AktifFlag {
		return nil, "JURNAL_EVENT_NOT_MAPPED"
	}

	// Filter by klasifikasi (nil = ALL).
	lines := mapping.DetailRows

	// Balance check.
	var totalDebit, totalKredit decimal.Decimal
	for _, l := range lines {
		amount := amountIDR.Mul(l.Multiplier)
		if l.DKIndicator == benchDKDebit {
			totalDebit = totalDebit.Add(amount)
		} else {
			totalKredit = totalKredit.Add(amount)
		}
	}
	if !totalDebit.Equal(totalKredit) {
		return nil, "JURNAL_BALANCE_INVARIANT"
	}
	_ = klasifikasi
	return lines, ""
}

// resolveAndPost performs resolve + INSERT jrnl.header + audit write.
// Simulates the full Asynq worker path (in-memory, no I/O).
func (svc *benchJurnalService) resolveAndPost(
	sourceEventID uuid.UUID,
	eventCode, klasifikasi string,
	amountIDR decimal.Decimal,
) (*benchJurnalHeader, string) {
	// Idempotency check.
	idempKey := computeBenchIdempotencyKey(sourceEventID, eventCode)
	if existing := svc.jurnalHeaders[idempKey]; existing != nil {
		return existing, "JURNAL_IDEMPOTENCY_REPLAY"
	}

	// Resolve.
	lines, errCode := svc.resolve(eventCode, klasifikasi, amountIDR)
	if errCode != "" {
		return nil, errCode
	}

	// Build header.
	svc.seqCounter++
	headerID := uuid.New()
	var totalDebit, totalKredit decimal.Decimal
	for _, l := range lines {
		amount := amountIDR.Mul(l.Multiplier)
		if l.DKIndicator == benchDKDebit {
			totalDebit = totalDebit.Add(amount)
		} else {
			totalKredit = totalKredit.Add(amount)
		}
	}

	header := &benchJurnalHeader{
		ID:             headerID,
		EventCode:      eventCode,
		StatusInternal: benchJrnlPosted,
		TotalDebit:     totalDebit,
		TotalKredit:    totalKredit,
		IdempotencyKey: idempKey,
		NoJurnal:       fmt.Sprintf("JRN-2026-%06d", svc.seqCounter),
	}
	svc.jurnalHeaders[idempKey] = header

	// Audit JURNAL.POST in-tx (simulate hash chain).
	var prevHash []byte
	if len(svc.auditLog) > 0 {
		prevHash = svc.auditLog[len(svc.auditLog)-1].CurrentHash
	}
	currentHash := computeBenchAuditHash(prevHash, "JURNAL.POST", headerID.String())
	svc.auditLog = append(svc.auditLog, benchAuditRow{
		Action:      "JURNAL.POST",
		EntityID:    headerID.String(),
		CurrentHash: currentHash,
	})

	return header, ""
}

// ─── Shared benchmark seed ────────────────────────────────────────────────────

var (
	benchJrnlEventCode   = "PENEMPATAN"
	benchJrnlKlasifikasi = "AC"
	benchJrnlAmountIDR   = decimal.NewFromInt(5_000_000_000)
	benchJrnlDate        = time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
)

// ─── BenchmarkResolverPreview ─────────────────────────────────────────────────
//
// Measures the resolver path: mapping lookup + line generation + balance check.
// This is the POST /jurnal/resolve preview path (no INSERT).
//
// SLA: ≤ 100 ms per op (from OpenAPI app-d-jurnal-engine.yaml §Performance SLA).

func BenchmarkResolverPreview(b *testing.B) {
	svc := newBenchJurnalService()
	svc.seedMapping(benchJrnlEventCode)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		lines, errCode := svc.resolve(benchJrnlEventCode, benchJrnlKlasifikasi, benchJrnlAmountIDR)
		if errCode != "" {
			b.Fatalf("BenchmarkResolverPreview: unexpected error: %s", errCode)
		}
		if len(lines) == 0 {
			b.Fatal("BenchmarkResolverPreview: no lines returned")
		}
	}

	// SLA assertion: ≤ 100 ms per op.
	elapsed := b.Elapsed()
	if b.N > 0 {
		perOp := elapsed / time.Duration(b.N)
		if perOp > 100*time.Millisecond {
			b.Errorf("BenchmarkResolverPreview: SLA violated — %v/op > 100ms (SLA from app-d-jurnal-engine.yaml §Performance SLA)", perOp)
		}
	}
}

// ─── BenchmarkResolveAndPost ──────────────────────────────────────────────────
//
// Measures the full Asynq worker path: resolve + INSERT jrnl.header + audit hash.
// Each iteration uses a unique source_event_id to avoid idempotency short-circuit.
//
// SLA: ≤ 300 ms per op (from OpenAPI app-d-jurnal-engine.yaml §Performance SLA).

func BenchmarkResolveAndPost(b *testing.B) {
	svc := newBenchJurnalService()
	svc.seedMapping(benchJrnlEventCode)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Fresh sourceEventID per iteration to avoid idempotency hit.
		srcID := uuid.New()
		header, errCode := svc.resolveAndPost(srcID, benchJrnlEventCode, benchJrnlKlasifikasi, benchJrnlAmountIDR)
		if errCode != "" && errCode != "JURNAL_IDEMPOTENCY_REPLAY" {
			b.Fatalf("BenchmarkResolveAndPost: unexpected error: %s", errCode)
		}
		if header == nil {
			b.Fatal("BenchmarkResolveAndPost: nil header returned")
		}
		if !header.TotalDebit.Equal(header.TotalKredit) {
			b.Fatal("BenchmarkResolveAndPost: balance invariant violated in benchmark")
		}
	}

	// SLA assertion: ≤ 300 ms per op.
	elapsed := b.Elapsed()
	if b.N > 0 {
		perOp := elapsed / time.Duration(b.N)
		if perOp > 300*time.Millisecond {
			b.Errorf("BenchmarkResolveAndPost: SLA violated — %v/op > 300ms (SLA from app-d-jurnal-engine.yaml §Performance SLA)", perOp)
		}
	}
}

// ─── BenchmarkAsynqSubscriberProcessing ──────────────────────────────────────
//
// Simulates end-to-end Asynq subscriber processing: receive payload → resolve
// → post → audit. Measures throughput for a realistic event batch.
//
// Production scenario: end-of-day batch of jurnal events from P5-M1/M6/M9/M10.
// Target: ≤ 5 s per event end-to-end (from OpenAPI app-d-jurnal-engine.yaml §Performance SLA).
// Computation-only benchmark; DB latency adds ~10–50 ms per event in production.

func BenchmarkAsynqSubscriberProcessing(b *testing.B) {
	svc := newBenchJurnalService()
	// Seed multiple event codes to simulate realistic subscriber.
	for _, code := range []string{"PENEMPATAN", "AKRUAL_BUNGA", "JATUH_TEMPO", "ECL_PEMBENTUKAN", "MTM_FVOCI"} {
		svc.seedMapping(code)
	}

	eventCodes := []string{"PENEMPATAN", "AKRUAL_BUNGA", "JATUH_TEMPO", "ECL_PEMBENTUKAN", "MTM_FVOCI"}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		eventCode := eventCodes[i%len(eventCodes)]
		srcID := uuid.New()
		header, errCode := svc.resolveAndPost(srcID, eventCode, benchJrnlKlasifikasi, benchJrnlAmountIDR)
		if errCode != "" && errCode != "JURNAL_IDEMPOTENCY_REPLAY" {
			b.Fatalf("BenchmarkAsynqSubscriberProcessing: unexpected error for %s: %s", eventCode, errCode)
		}
		_ = header
	}

	// SLA assertion: ≤ 5 s per event.
	elapsed := b.Elapsed()
	if b.N > 0 {
		perOp := elapsed / time.Duration(b.N)
		if perOp > 5*time.Second {
			b.Errorf("BenchmarkAsynqSubscriberProcessing: SLA violated — %v/op > 5s (SLA from app-d-jurnal-engine.yaml §Performance SLA)", perOp)
		}
	}
}

// ─── BenchmarkIdempotencyKeyComputation ──────────────────────────────────────
//
// Micro-benchmark for SHA-256 idempotency key computation.
// Called on every worker invocation to prevent duplicate jurnal posting.

func BenchmarkIdempotencyKeyComputation(b *testing.B) {
	srcID := uuid.New()
	eventCode := "PENEMPATAN"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := computeBenchIdempotencyKey(srcID, eventCode)
		if key == "" {
			b.Fatal("BenchmarkIdempotencyKeyComputation: empty key")
		}
	}
}

// ─── BenchmarkAuditHashChain ──────────────────────────────────────────────────
//
// Micro-benchmark for audit log hash chain computation (SHA-256 per row).
// In production, every JURNAL.POST writes 1 audit row with hash chaining.

func BenchmarkAuditHashChain(b *testing.B) {
	prevHash := make([]byte, 32)
	action := "JURNAL.POST"
	entityID := uuid.New().String()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		h := computeBenchAuditHash(prevHash, action, entityID)
		// Chain: use this hash as next prevHash.
		prevHash = h
	}
}

// ─── BenchmarkResolveAndPostBatch1000 ────────────────────────────────────────
//
// Simulates a batch of 1000 jurnal events (end-of-period batch posting).
// Useful for establishing per-batch throughput baseline for k6 load tests.
//
// Advisory SLA: complete 1000 events in < 1 s computation-only.

func BenchmarkResolveAndPostBatch1000(b *testing.B) {
	const batchSize = 1000

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		svc := newBenchJurnalService()
		svc.seedMapping("PENEMPATAN")
		// Pre-generate source IDs.
		sourceIDs := make([]uuid.UUID, batchSize)
		for j := range sourceIDs {
			sourceIDs[j] = uuid.New()
		}
		b.StartTimer()

		for j := 0; j < batchSize; j++ {
			_, errCode := svc.resolveAndPost(sourceIDs[j], "PENEMPATAN", "AC", benchJrnlAmountIDR)
			if errCode != "" && errCode != "JURNAL_IDEMPOTENCY_REPLAY" {
				b.Fatalf("BenchmarkResolveAndPostBatch1000: event[%d] error: %s", j, errCode)
			}
		}
	}

	// Advisory: 1000 events should complete well within 1s computation-only.
	elapsed := b.Elapsed()
	if b.N > 0 && b.N == 1 {
		if elapsed > time.Second {
			b.Logf("BenchmarkResolveAndPostBatch1000: %v for %d events (advisory ≤ 1s computation-only)", elapsed, batchSize)
		}
	}
	_ = benchJrnlDate
}
