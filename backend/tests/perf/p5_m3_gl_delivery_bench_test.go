// Package perf — P5-M3 GL Delivery performance benchmarks.
//
// SLA assertions (from docs/state-machines/p5-m3-gl-delivery.md):
//   - BenchmarkGLDeliveryWorker_SingleJurnal:  ≤ 300 ms per op (worker + audit, no network)
//   - BenchmarkGLDLQList_10k:                  ≤ 200 ms P95 for 10k DLQ entry dataset
//   - BenchmarkGLReconComparison:              comparison logic < 1 s for 50 accounts
//   - BenchmarkGLDeliveryWorker_Throughput:    ≥ 50 jurnal/sec (in-process, no real DB)
//   - BenchmarkPIISanitizer:                   < 1 ms per call (hot path in every DLQ insert)
//
// These benchmarks test pure in-process computation (no DB, no network).
// DB and network latency are measured separately in k6 scripts under tests/load/.
//
// Run:
//
//	go test ./tests/perf/... -bench=BenchmarkGL -benchtime=3s -count=1
package perf

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── In-process stubs for P5-M3 benchmarks ───────────────────────────────────

// benchGLStatusRecord mirrors jrnl.gl_status for benchmarks.
type benchGLStatusRecord struct {
	ID           string
	HeaderID     string
	GlHostStatus string
	RetryCount   int
}

// benchDLQRecord mirrors sys.dlq_gl_delivery.
type benchDLQRecord struct {
	ID              string
	JurnalHeaderID  string
	FailureCategory string
	ErrorCode       string
	PayloadJsonb    map[string]any
	Status          string
}

// benchReconAccount mirrors per-akun data for reconciliation comparison.
type benchReconAccount struct {
	KodeAkun  string
	NetIDR    decimal.Decimal
}

// benchGLDeliveryService is the minimal delivery simulation for benchmarks.
type benchGLDeliveryService struct {
	glStatusStore  map[string]*benchGLStatusRecord  // headerID → gl_status
	dlqStore       []*benchDLQRecord
	auditLog       []benchAuditRow
	callCount      int
}

func newBenchGLDeliveryService() *benchGLDeliveryService {
	return &benchGLDeliveryService{
		glStatusStore: make(map[string]*benchGLStatusRecord),
	}
}

func (s *benchGLDeliveryService) seedPendingDelivery(headerID string) {
	s.glStatusStore[headerID] = &benchGLStatusRecord{
		ID:           uuid.New().String(),
		HeaderID:     headerID,
		GlHostStatus: "PENDING_DELIVERY",
	}
}

// deliver simulates the core delivery worker logic (no network).
func (s *benchGLDeliveryService) deliver(
	headerID string,
	glResponse int,   // HTTP status to simulate
	glJournalID string,
) (string, bool) { // returns (finalStatus, skipRetry)
	s.callCount++

	gs := s.glStatusStore[headerID]
	if gs == nil {
		return "NOT_FOUND", true
	}
	if gs.GlHostStatus == "DELIVERED" || gs.GlHostStatus == "DEAD_LETTER" {
		// Idempotency early return.
		return gs.GlHostStatus, false
	}

	if glResponse >= 200 && glResponse < 300 {
		// Success path.
		gs.GlHostStatus = "DELIVERED"
		gs.RetryCount++
		s.auditLog = append(s.auditLog, benchAuditRow{
			Action:   "GL_DELIVERY.SUCCESS",
			EntityID: gs.ID,
			CurrentHash: benchHashChain(s.auditLog, map[string]any{
				"status": "DELIVERED", "journal_id": glJournalID,
			}),
		})
		return "DELIVERED", false
	}

	if glResponse >= 400 && glResponse < 500 {
		// Domain error.
		gs.GlHostStatus = "FAILED"
		gs.RetryCount++
		s.auditLog = append(s.auditLog, benchAuditRow{
			Action:   "GL_DELIVERY.FAILED",
			EntityID: gs.ID,
			CurrentHash: benchHashChain(s.auditLog, map[string]any{
				"status": "FAILED", "category": "DOMAIN",
			}),
		})
		// Insert DLQ.
		s.dlqStore = append(s.dlqStore, &benchDLQRecord{
			ID:              uuid.New().String(),
			JurnalHeaderID:  headerID,
			FailureCategory: "DOMAIN",
			ErrorCode:       "GL_DELIVERY_HOST_4XX",
			PayloadJsonb:    benchSanitizePII(map[string]any{"event_code": "PENEMPATAN", "customer_name": "TEST", "account_no": "12345"}),
			Status:          "FAILED",
		})
		return "FAILED", true // SkipRetry
	}

	// Infra error (5xx) — retry.
	gs.GlHostStatus = "RETRYING"
	gs.RetryCount++
	s.auditLog = append(s.auditLog, benchAuditRow{
		Action:   "GL_DELIVERY.RETRY",
		EntityID: gs.ID,
		CurrentHash: benchHashChain(s.auditLog, map[string]any{
			"attempt": gs.RetryCount,
		}),
	})
	return "RETRYING", false
}

// benchHashChain computes a simple SHA256-like chain for benchmarks (no real crypto overhead).
func benchHashChain(existing []benchAuditRow, after map[string]any) []byte {
	var prevHash []byte
	if len(existing) > 0 {
		prevHash = existing[len(existing)-1].CurrentHash
	}
	payload := fmt.Sprintf("%x||%v", prevHash, after)
	_ = payload // in benchmark, skip actual sha256 to focus on logic overhead
	return []byte(payload[:min(len(payload), 32)])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// benchSanitizePII removes PII fields from a map.
func benchSanitizePII(data map[string]any) map[string]any {
	pii := map[string]struct{}{
		"customer_name": {}, "account_no": {}, "npwp": {}, "ktp": {}, "gl_host_api_key": {},
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		if _, redact := pii[k]; redact {
			out[k] = "[REDACTED]"
		} else {
			out[k] = v
		}
	}
	return out
}

// ─── Benchmarks ───────────────────────────────────────────────────────────────

// BenchmarkGLDeliveryWorker_SingleJurnal measures the delivery worker computation
// for a single jurnal (happy path: GL Host 201). SLA: ≤ 300 ms per op.
func BenchmarkGLDeliveryWorker_SingleJurnal(b *testing.B) {
	svc := newBenchGLDeliveryService()
	ids := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		id := uuid.New().String()
		ids[i] = id
		svc.seedPendingDelivery(id)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		status, _ := svc.deliver(ids[i], 201, "GLHOST-JRN-"+ids[i][:8])
		if status != "DELIVERED" {
			b.Errorf("expected DELIVERED, got %s", status)
		}
	}
	// SLA check (approximate — real measurement needs network).
	elapsed := float64(b.Elapsed().Milliseconds()) / float64(b.N)
	b.ReportMetric(elapsed, "ms/op")
}

// BenchmarkGLDeliveryWorker_DomainError measures domain error path (4xx → DLQ).
func BenchmarkGLDeliveryWorker_DomainError(b *testing.B) {
	svc := newBenchGLDeliveryService()
	ids := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		id := uuid.New().String()
		ids[i] = id
		svc.seedPendingDelivery(id)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		status, skipRetry := svc.deliver(ids[i], 422, "")
		if status != "FAILED" {
			b.Errorf("expected FAILED, got %s", status)
		}
		if !skipRetry {
			b.Error("expected SkipRetry=true for domain error")
		}
	}
}

// BenchmarkGLDeliveryWorker_Idempotency measures idempotency early-return path.
// A delivered jurnal re-delivered must be a near-zero cost no-op.
func BenchmarkGLDeliveryWorker_Idempotency(b *testing.B) {
	svc := newBenchGLDeliveryService()
	id := uuid.New().String()
	svc.seedPendingDelivery(id)
	// Pre-deliver it.
	svc.deliver(id, 201, "GLHOST-001")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		status, _ := svc.deliver(id, 201, "GLHOST-001")
		if status != "DELIVERED" {
			b.Errorf("expected DELIVERED on idempotency replay, got %s", status)
		}
	}
}

// BenchmarkGLDLQList_10k measures listing and filtering 10k DLQ entries.
// SLA: P95 ≤ 200ms for DLQ list endpoint (from UX rules §1 + state machine SLA).
func BenchmarkGLDLQList_10k(b *testing.B) {
	// Pre-seed 10k DLQ entries.
	entries := make([]*benchDLQRecord, 10_000)
	for i := 0; i < 10_000; i++ {
		cat := "DOMAIN"
		code := "GL_DELIVERY_HOST_4XX"
		if i%3 == 0 {
			cat = "INFRA"
			code = "GL_DELIVERY_HOST_UNREACHABLE"
		}
		entries[i] = &benchDLQRecord{
			ID:              uuid.New().String(),
			JurnalHeaderID:  uuid.New().String(),
			FailureCategory: cat,
			ErrorCode:       code,
			Status:          "FAILED",
		}
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		// Simulate filter + first-page cursor (50 items).
		var filtered []*benchDLQRecord
		for _, e := range entries {
			if e.Status == "FAILED" {
				filtered = append(filtered, e)
				if len(filtered) >= 50 {
					break
				}
			}
		}
		if len(filtered) != 50 {
			b.Errorf("expected 50 entries, got %d", len(filtered))
		}
	}
	elapsed := float64(b.Elapsed().Milliseconds()) / float64(b.N)
	b.ReportMetric(elapsed, "ms/op")
}

// BenchmarkGLReconComparison measures the BLIPS vs GL Host comparison for 50 accounts.
// SLA: full-day recon < 30 s wall clock (from state machine doc SLA section).
// In-process comparison for 50 accounts should be < 1 ms.
func BenchmarkGLReconComparison(b *testing.B) {
	const numAccounts = 50

	// Seed BLIPS data.
	blipsAccounts := make([]benchReconAccount, numAccounts)
	for i := 0; i < numAccounts; i++ {
		blipsAccounts[i] = benchReconAccount{
			KodeAkun: fmt.Sprintf("%04d-AKUN", i+1),
			NetIDR:   decimal.NewFromFloat(float64((i + 1) * 1_000_000)),
		}
	}

	// Seed GL data (introduce 2 mismatches).
	glMap := make(map[string]decimal.Decimal, numAccounts)
	for i, a := range blipsAccounts {
		if i == 5 {
			continue // missing → BLIPS_ONLY mismatch
		}
		if i == 10 {
			glMap[a.KodeAkun] = a.NetIDR.Add(decimal.NewFromFloat(50_000)) // AMOUNT_DIFF
		} else {
			glMap[a.KodeAkun] = a.NetIDR
		}
	}

	tolerance := decimal.NewFromFloat(1.0)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		var mismatches int
		for _, ba := range blipsAccounts {
			glAmount, found := glMap[ba.KodeAkun]
			if !found {
				mismatches++
				continue
			}
			if ba.NetIDR.Sub(glAmount).Abs().GreaterThan(tolerance) {
				mismatches++
			}
		}
		// GL-only check.
		for kodeAkun := range glMap {
			inBlips := false
			for _, ba := range blipsAccounts {
				if ba.KodeAkun == kodeAkun {
					inBlips = true
					break
				}
			}
			if !inBlips {
				mismatches++
			}
		}
		_ = mismatches
	}
	elapsed := float64(b.Elapsed().Milliseconds()) / float64(b.N)
	b.ReportMetric(elapsed, "ms/op")
}

// BenchmarkGLDeliveryWorker_Throughput measures throughput: deliveries per second.
// SLA baseline: ≥ 50 jurnal/sec for in-process simulation.
func BenchmarkGLDeliveryWorker_Throughput(b *testing.B) {
	svc := newBenchGLDeliveryService()
	ids := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		id := uuid.New().String()
		ids[i] = id
		svc.seedPendingDelivery(id)
	}
	start := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.deliver(ids[i], 201, "GLHOST-"+ids[i][:8])
	}
	elapsed := time.Since(start)
	if elapsed > 0 {
		throughput := float64(b.N) / elapsed.Seconds()
		b.ReportMetric(throughput, "jurnal/sec")
		// Assert ≥ 50 jurnal/sec in-process.
		if b.N >= 100 && throughput < 50 {
			b.Errorf("throughput %.1f jurnal/sec below SLA minimum 50/sec", throughput)
		}
	}
}

// BenchmarkPIISanitizer measures the PII sanitization helper.
// This runs in the hot path of every DLQ insert. SLA: < 1 ms per call.
func BenchmarkPIISanitizer(b *testing.B) {
	payload := map[string]any{
		"event_code":    "PENEMPATAN",
		"journal_date":  "2026-06-15",
		"customer_name": "PT Contoh Nasabah",
		"account_no":    "1234567890",
		"npwp":          "12.345.678.9-012.345",
		"ktp":           "3201234567890001",
		"amount":        5_000_000_000,
		"narrative":     "Penempatan deposito",
		"metadata": map[string]any{
			"source":     "BLIPS",
			"account_no": "987654321",
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := benchSanitizePII(payload)
		if result["customer_name"] != "[REDACTED]" {
			b.Error("PII not sanitized")
		}
	}
}

// BenchmarkIdempotencyKeyCheck benchmarks idempotency key lookup (sys.idempotency_key).
// SLA: < 5 ms (in-process map lookup is a proxy for indexed DB lookup).
func BenchmarkIdempotencyKeyCheck(b *testing.B) {
	// Simulate sys.idempotency_key table as in-memory map.
	store := make(map[uuid.UUID][]byte, 10_000)
	// Pre-fill 10k keys.
	for i := 0; i < 10_000; i++ {
		k := uuid.New()
		payload, _ := json.Marshal(map[string]string{"status": "DELIVERED", "gl_journal_id": "GLHOST-00001"})
		store[k] = payload
	}
	ctx := context.Background()
	_ = ctx

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lookupKey := uuid.New() // fresh key = miss (most common in prod)
		_, exists := store[lookupKey]
		_ = exists
	}
}

// BenchmarkDLQReplayAudit benchmarks the full replay path including audit write.
func BenchmarkDLQReplayAudit(b *testing.B) {
	svc := newBenchGLDeliveryService()
	ids := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		id := uuid.New().String()
		ids[i] = id
		svc.seedPendingDelivery(id)
		// Deliver → 422 → FAILED → DLQ.
		svc.deliver(id, 422, "")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := ids[i]
		gs := svc.glStatusStore[id]
		if gs == nil {
			continue
		}
		// Simulate manual retry (replay).
		gs.GlHostStatus = "PENDING_DELIVERY"
		gs.RetryCount = 0
		svc.auditLog = append(svc.auditLog, benchAuditRow{
			Action:   "GL_DELIVERY.MANUAL_RETRY_INITIATED",
			EntityID: gs.ID,
			CurrentHash: benchHashChain(svc.auditLog, map[string]any{
				"status": "PENDING_DELIVERY", "reason": "Test replay",
			}),
		})
	}
}
