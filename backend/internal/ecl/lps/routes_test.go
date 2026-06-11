package lps

// routes_test.go — smoke test for RegisterRoutes to verify route registration
// without running actual handlers.
//
// Tests only that routes are registered (responds to OPTIONS or returns a non-404
// for registered paths). Does not test auth or business logic.

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

func TestRegisterRoutes_RoutesRegistered(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	// Generate test RSA key for auth.Verifier.
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	verifier := auth.NewVerifier(&privKey.PublicKey, "http://test.local/realms/blips")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")

	capRow := &LPSCoverageRow{ID: uuid.New(), CoverageAmountIDR: decimal.NewFromInt(2_000_000_000)}
	agg := NewAggregatorService(
		&mockCoverageRepo{row: capRow},
		&mockDepositoRepo{},
		&mockOverrideRepo{},
		&mockKursRepo{},
	)
	ovSvc := &OverrideService{overrideRepo: &mockOverrideRepo{}, auditWriter: &mockAuditWriter{}}
	h := NewHandler(agg, ovSvc)

	RegisterRoutes(v1, h, verifier, db)

	// Check that registered GET endpoints exist (return non-404 status).
	// Auth middleware will reject with 401, but that's NOT 404.
	paths := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/ecl/lps/preview"},
		{"GET", "/api/v1/ecl/lps/preview/export"},
		{"GET", "/api/v1/ecl/lps/overrides"},
		{"GET", "/api/v1/ecl/lps/overrides/" + uuid.New().String()},
		{"POST", "/api/v1/ecl/lps/aggregate"},
		{"POST", "/api/v1/ecl/lps/aggregate/bulk"},
		{"POST", "/api/v1/ecl/lps/override/submit"},
		{"POST", "/api/v1/ecl/lps/override/" + uuid.New().String() + "/approve"},
		{"POST", "/api/v1/ecl/lps/override/" + uuid.New().String() + "/reject"},
	}
	for _, tc := range paths {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		// Route exists → not 404. Auth will return 401 (no token).
		if w.Code == http.StatusNotFound {
			t.Errorf("route %s %s not registered (got 404)", tc.method, tc.path)
		}
	}
}
