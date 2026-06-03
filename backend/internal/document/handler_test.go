package document

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// setupTestRouter membuat Gin router dengan handler document (tanpa auth/idempotency middleware
// agar bisa ditest secara unit).
func setupTestRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	docs := r.Group("/api/v1/documents")
	{
		docs.POST("", h.Upload)
		docs.GET("/:id", h.GetPresignedURL)
	}
	return r
}

// TestHandler_Upload_NoAuth memverifikasi 401 ketika tidak ada auth claims.
func TestHandler_Upload_NoAuth(t *testing.T) {
	svc := NewService(&DBRepository{db: nil}, nil, nil, nil)
	h := NewHandler(svc)
	r := setupTestRouter(h)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "test.pdf")
	fw.Write([]byte("test content"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rw := httptest.NewRecorder()

	r.ServeHTTP(rw, req)

	// Tanpa auth claims → 401
	if rw.Code != http.StatusUnauthorized {
		t.Errorf("Upload tanpa auth = %d, want %d", rw.Code, http.StatusUnauthorized)
	}
}

// TestHandler_GetPresignedURL_NoAuth memverifikasi 401 ketika tidak ada auth claims.
func TestHandler_GetPresignedURL_NoAuth(t *testing.T) {
	svc := NewService(&DBRepository{db: nil}, nil, nil, nil)
	h := NewHandler(svc)
	r := setupTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+uuid.New().String(), nil)
	rw := httptest.NewRecorder()

	r.ServeHTTP(rw, req)

	if rw.Code != http.StatusUnauthorized {
		t.Errorf("GetPresignedURL tanpa auth = %d, want %d", rw.Code, http.StatusUnauthorized)
	}
}

// TestHandler_GetPresignedURL_InvalidID memverifikasi 400 untuk ID bukan UUID.
// Menggunakan *auth.Claims nyata di-inject ke Gin context agar permission check lulus.
func TestHandler_GetPresignedURL_InvalidID(t *testing.T) {
	svc := NewService(&DBRepository{db: nil}, nil, nil, nil)
	h := NewHandler(svc)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/documents/:id", func(c *gin.Context) {
		// Inject real *auth.Claims dengan permission document.read.
		claims := &auth.Claims{
			Sub:         uuid.New().String(),
			Permissions: []string{"document.read"},
			TenantID:    "TUGURE",
		}
		c.Set("claims", claims)
		h.GetPresignedURL(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/not-a-uuid", nil)
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, req)

	if rw.Code != http.StatusBadRequest {
		t.Errorf("GetPresignedURL invalid ID = %d, want 400", rw.Code)
	}
}

// TestDetectMimeByExtension memverifikasi MIME detection untuk semua ekstensi yang didukung.
func TestDetectMimeByExtension(t *testing.T) {
	cases := []struct {
		filename string
		want     string
	}{
		{"report.pdf", "application/pdf"},
		{"data.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"data.xls", "application/vnd.ms-excel"},
		{"data.csv", "text/csv"},
		{"contract.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"contract.doc", "application/msword"},
		{"photo.png", "image/png"},
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"unknown.xyz", "application/octet-stream"},
		{"REPORT.PDF", "application/pdf"}, // case insensitive
	}
	for _, tc := range cases {
		got := detectMimeByExtension(tc.filename)
		if got != tc.want {
			t.Errorf("detectMimeByExtension(%q) = %q, want %q", tc.filename, got, tc.want)
		}
	}
}

// Catatan untuk qa-engineer:
// Handler tests yang membutuhkan upload end-to-end (MinIO + DB + antivirus scan)
// ditandai sebagai integration test candidates. Jalankan dengan:
//   INTEGRATION_TEST=true go test ./internal/document/... -run TestIntegration
// Butuh: live MinIO di localhost:9000, PostgreSQL blips_db, Redis.
