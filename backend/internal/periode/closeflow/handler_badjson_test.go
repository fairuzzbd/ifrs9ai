package closeflow_test

// handler_badjson_test.go — Tests for each handler's JSON body parse error path.
// Covers the `c.ShouldBindJSON` failure branch in every handler method.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

func TestSoftCloseRequest_BadJSON_Returns400(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)
	r := setupRouter(t, h, claimsWithPermission(closeflow.PermPeriodeSoftcloseRequest))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+uuid.New().String()+"/soft-close-request",
		bytes.NewReader([]byte(`{invalid json}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSoftCloseApprove_BadJSON_Returns400(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)
	r := setupRouter(t, h, claimsWithPermission(closeflow.PermPeriodeSoftcloseApprove))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+uuid.New().String()+"/soft-close-approve",
		bytes.NewReader([]byte(`{invalid json}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHardCloseRequest_BadJSON_Returns400(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)
	r := setupRouter(t, h, claimsWithPermission(closeflow.PermPeriodeHardcloseRequest))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+uuid.New().String()+"/hard-close-request",
		bytes.NewReader([]byte(`{invalid json}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHardCloseApprove_BadJSON_Returns400(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)
	stepupTS := int64(9999999999)
	claims := &auth.Claims{
		Sub:              uuid.New().String(),
		Roles:            []string{"ROLE-CFO"},
		Permissions:      []string{closeflow.PermPeriodeHardcloseApprove},
		TenantID:         "TUGURE",
		MFAVerified:      true,
		StepupVerifiedAt: &stepupTS,
	}
	r := setupRouter(t, h, claims)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+uuid.New().String()+"/hard-close-approve",
		bytes.NewReader([]byte(`{invalid json}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	req.Header.Set("X-Step-Up-Token", "some-valid-step-up-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHardCloseReject_BadJSON_Returns400(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)
	r := setupRouter(t, h, claimsWithPermission(closeflow.PermPeriodeHardcloseApprove))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+uuid.New().String()+"/hard-close-reject",
		bytes.NewReader([]byte(`{invalid json}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReopenRequest_BadJSON_Returns400(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)
	r := setupRouter(t, h, claimsWithPermission(closeflow.PermPeriodeReopenRequest))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+uuid.New().String()+"/reopen-request",
		bytes.NewReader([]byte(`{invalid json}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReopenApprove_BadJSON_Returns400(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)
	r := setupRouter(t, h, claimsWithPermission(closeflow.PermPeriodeReopenApprove))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+uuid.New().String()+"/reopen-approve",
		bytes.NewReader([]byte(`{invalid json}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
