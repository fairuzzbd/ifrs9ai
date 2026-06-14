package middleware_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// TestRequestID_InjectsTraceID verifies trace ID is set when not present.
func TestRequestID_InjectsTraceID(t *testing.T) {
	router := gin.New()
	router.Use(middleware.RequestID())

	var capturedTraceID string
	router.GET("/test", func(c *gin.Context) {
		traceID, exists := c.Get(response.TraceIDKey)
		if !exists {
			t.Error("X-Trace-Id not set in Gin context")
		}
		capturedTraceID, _ = traceID.(string)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if capturedTraceID == "" {
		t.Error("trace ID should be generated when not in request")
	}
	if w.Header().Get("X-Trace-Id") == "" {
		t.Error("X-Trace-Id should be set in response header")
	}
}

// TestRequestID_PropagatesExistingTraceID verifies existing trace ID is preserved.
func TestRequestID_PropagatesExistingTraceID(t *testing.T) {
	router := gin.New()
	router.Use(middleware.RequestID())

	var capturedTraceID string
	router.GET("/test", func(c *gin.Context) {
		v, _ := c.Get(response.TraceIDKey)
		capturedTraceID, _ = v.(string)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Trace-Id", "existing-trace-id-12345")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if capturedTraceID != "existing-trace-id-12345" {
		t.Errorf("expected existing trace ID, got %s", capturedTraceID)
	}
}

// TestRequestID_ContextPropagation verifies trace ID is in request context.
func TestRequestID_ContextPropagation(t *testing.T) {
	router := gin.New()
	router.Use(middleware.RequestID())

	var ctxTraceID string
	router.GET("/test", func(c *gin.Context) {
		ctxTraceID = middleware.TraceIDFromContext(c.Request.Context())
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Trace-Id", "ctx-trace-99")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if ctxTraceID != "ctx-trace-99" {
		t.Errorf("expected ctx-trace-99 in request context, got %s", ctxTraceID)
	}
}

// TestLogger_DoesNotPanic verifies Logger middleware doesn't panic.
func TestLogger_DoesNotPanic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(middleware.Logger(logger))

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestRecovery_PanicsReturn500 verifies Recovery middleware converts panics to 500.
func TestRecovery_PanicsReturn500(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(middleware.Recovery(logger))

	router.GET("/panic", func(c *gin.Context) {
		panic("test panic — expected in unit test")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 after panic, got %d", w.Code)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("response body should not be empty after panic")
	}
}

// TestTraceIDFromContext_Empty verifies empty string when not in context.
func TestTraceIDFromContext_Empty(t *testing.T) {
	traceID := middleware.TraceIDFromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context())
	if traceID != "" {
		t.Errorf("expected empty, got %s", traceID)
	}
}
