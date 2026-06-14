package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRouter(handler func(*gin.Context)) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Any("/test", handler)
	return r
}

// TestOK_SuccessEnvelope verifies OK writes proper success envelope.
func TestOK_SuccessEnvelope(t *testing.T) {
	router := setupRouter(func(c *gin.Context) {
		response.OK(c, map[string]any{"key": "value"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var env response.SuccessEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Data == nil {
		t.Error("data should not be nil")
	}
	if env.Meta.TraceID == "" {
		t.Error("traceId should be set in meta")
	}
}

// TestCreated_Returns201 verifies Created returns 201.
func TestCreated_Returns201(t *testing.T) {
	router := setupRouter(func(c *gin.Context) {
		response.Created(c, map[string]any{"id": "123"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

// TestError_DomainError verifies domain error is mapped correctly.
func TestError_DomainError(t *testing.T) {
	router := setupRouter(func(c *gin.Context) {
		response.Error(c, domainerrors.ErrNotFound("instrumen"))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var env response.ErrorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error.Code != string(domainerrors.CodeNotFound) {
		t.Errorf("expected NOT_FOUND, got %s", env.Error.Code)
	}
	if env.Error.TraceID == "" {
		t.Error("traceId should be set in error envelope")
	}
	if env.Error.Details == nil {
		t.Error("details should not be nil (should be empty slice)")
	}
}

// TestError_UnknownError verifies non-domain error returns 500 INTERNAL.
func TestError_UnknownError(t *testing.T) {
	router := setupRouter(func(c *gin.Context) {
		response.Error(c, http.ErrAbortHandler) // plain error
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}

	var env response.ErrorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL, got %s", env.Error.Code)
	}
	// Must NOT expose raw error message (security: mask internal errors).
	if env.Error.Message == http.ErrAbortHandler.Error() {
		t.Error("SECURITY: raw internal error message exposed in response")
	}
}

// TestError_SoDViolation verifies SoD violation returns 403.
func TestError_SoDViolation(t *testing.T) {
	router := setupRouter(func(c *gin.Context) {
		response.Error(c, domainerrors.ErrSoDViolation("maker cannot review"))
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}

	var env response.ErrorEnvelope
	json.Unmarshal(w.Body.Bytes(), &env)
	if env.Error.Code != "SOD_VIOLATION" {
		t.Errorf("expected SOD_VIOLATION, got %s", env.Error.Code)
	}
}

// TestList_ListEnvelope verifies List writes proper list envelope.
func TestList_ListEnvelope(t *testing.T) {
	router := setupRouter(func(c *gin.Context) {
		nextCursor := "eyJpZCI6IjEyMyJ9"
		response.List(c, []any{"item1", "item2"}, &response.PaginationMeta{
			NextCursor: &nextCursor,
			HasMore:    true,
			Limit:      50,
		}, []response.SortApplied{{Col: "created_at", Dir: "desc"}}, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var env response.ListEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !env.Pagination.HasMore {
		t.Error("hasMore should be true")
	}
	if env.Meta.TraceID == "" {
		t.Error("traceId should be set")
	}
}

// TestAccepted_Returns202 verifies Accepted returns 202.
func TestAccepted_Returns202(t *testing.T) {
	router := setupRouter(func(c *gin.Context) {
		response.Accepted(c, map[string]any{"jobId": "job_123"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

// TestError_NilDetails verifies nil details are converted to empty slice (not null in JSON).
func TestError_NilDetails(t *testing.T) {
	router := setupRouter(func(c *gin.Context) {
		// DomainError with no details.
		response.Error(c, domainerrors.New(domainerrors.CodeConflict, "conflict"))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	// Details should be [] not null in JSON.
	if !contains(body, `"details":[]`) {
		t.Errorf("details should be empty array [], not null. body=%s", body)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr ||
		len(s) >= len(substr) &&
			func() bool {
				for i := 0; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())
}
