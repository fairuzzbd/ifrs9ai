package document

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Handler adalah HTTP handler untuk document upload/download.
type Handler struct {
	svc *Service
}

// NewHandler membuat Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mendaftarkan document routes ke router group.
// Group harus sudah memiliki Idempotency + Auth middleware.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler) {
	docs := rg.Group("/documents")
	{
		docs.POST("", h.Upload)
		docs.GET("/:id", h.GetPresignedURL)
	}
}

// maxUploadSize adalah batas multipart upload yang di-enforce di handler.
// Sama dengan MaxFileSizeBytes (100MB) + 1MB untuk overhead multipart.
const maxUploadSize = MaxFileSizeBytes + (1 << 20)

// uploadRequest adalah field non-file dari multipart upload form.
// Diambil dari form values (bukan JSON body karena multipart).
type uploadRequest struct {
	EntityType  string
	EntityID    string
	Category    string
	Description string
}

// uploadResponseJSON adalah response body untuk sukses upload.
type uploadResponseJSON struct {
	DocumentID      string `json:"documentId"`
	DocRefKode      string `json:"docRefKode"`
	Filename        string `json:"filename"`
	SHA256Hash      string `json:"sha256Hash"`
	VirusScanStatus string `json:"virusScanStatus"`
	ObjectKey       string `json:"objectKey"`
	// Pesan spesifik per ux-patterns.md §2.2.
	Message string `json:"message"`
}

// downloadResponseJSON adalah response body untuk get presigned URL.
type downloadResponseJSON struct {
	DocumentID   string `json:"documentId"`
	PresignedURL string `json:"presignedUrl"`
	ExpiresAt    string `json:"expiresAt"`
}

// Upload menangani POST /api/v1/documents (multipart/form-data).
//
// Form fields:
//   - file: file binary (required)
//   - entity_type: mis. "mst.instrumen" (required)
//   - entity_id: UUID (required)
//   - category: Category (required)
//   - description: string (optional)
//
// Headers wajib (dari middleware): Idempotency-Key, Authorization: Bearer
//
// Permission: "document.create"
func (h *Handler) Upload(c *gin.Context) {
	// Permission check.
	claims := auth.ClaimsFromGin(c)
	if claims == nil {
		response.Error(c, domainerrors.ErrUnauthorized("Claims tidak ada."))
		return
	}
	if !claims.HasPermission("document.create") {
		response.Error(c, domainerrors.ErrForbidden("document.create"))
		return
	}

	// Batasi ukuran multipart.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)

	// Parse multipart form.
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request tidak valid: "+err.Error()))
		return
	}

	// Ambil file.
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Field 'file' wajib disertakan.",
			domainerrors.Detail{Field: "file", Rule: "required", Message: "File wajib diupload"},
		))
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		response.Error(c, domainerrors.Wrap(domainerrors.CodeInternal, "Gagal membuka file upload.", err))
		return
	}
	defer f.Close()

	// Baca form fields.
	req := uploadRequest{
		EntityType:  strings.TrimSpace(c.PostForm("entity_type")),
		EntityID:    strings.TrimSpace(c.PostForm("entity_id")),
		Category:    strings.TrimSpace(c.PostForm("category")),
		Description: strings.TrimSpace(c.PostForm("description")),
	}

	// Validasi entity_id UUID.
	entityID, err := uuid.Parse(req.EntityID)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"entity_id harus berformat UUID v4.",
			domainerrors.Detail{Field: "entity_id", Rule: "uuid", Message: "Format UUID tidak valid"},
		))
		return
	}

	// Deteksi MIME type dari Content-Type header multipart part atau fallback ke
	// ekstensi file.
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = detectMimeByExtension(fileHeader.Filename)
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.Error(c, domainerrors.ErrUnauthorized("User ID di claims bukan UUID valid."))
		return
	}

	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}

	in := UploadInput{
		Filename:         fileHeader.Filename,
		MimeType:         contentType,
		FileSizeBytes:    fileHeader.Size,
		EntityType:       req.EntityType,
		EntityID:         entityID,
		Category:         Category(req.Category),
		Description:      desc,
		UploadedByUserID: userID,
	}

	result, err := h.svc.Upload(c.Request.Context(), in, io.Reader(f))
	if err != nil {
		response.Error(c, err)
		return
	}

	// Pesan spesifik per ux-patterns.md §2.2.
	message := fmt.Sprintf("Dokumen %s (%s) berhasil diupload. Pemindaian virus sedang dijadwalkan.",
		result.DocRefKode, fileHeader.Filename)

	response.Created(c, uploadResponseJSON{
		DocumentID:      result.DocumentID.String(),
		DocRefKode:      result.DocRefKode,
		Filename:        fileHeader.Filename,
		SHA256Hash:      result.SHA256Hash,
		VirusScanStatus: string(result.VirusScanStatus),
		ObjectKey:       result.ObjectKey,
		Message:         message,
	})
}

// GetPresignedURL menangani GET /api/v1/documents/{id}.
//
// Mengembalikan presigned URL untuk download dokumen.
// TTL: 60 menit (konfigurabel via DOCUMENT_PRESIGN_TTL_MINUTES).
//
// Permission: "document.read"
//
// Security note:
// - URL presigned mengandung signature — jangan log di level INFO/ERROR.
// - Client wajib request ulang setelah TTL.
// - Dokumen dengan virus_scan_status = INFECTED tidak bisa di-download.
func (h *Handler) GetPresignedURL(c *gin.Context) {
	claims := auth.ClaimsFromGin(c)
	if claims == nil {
		response.Error(c, domainerrors.ErrUnauthorized("Claims tidak ada."))
		return
	}
	if !claims.HasPermission("document.read") {
		response.Error(c, domainerrors.ErrForbidden("document.read"))
		return
	}

	idStr := c.Param("id")
	docID, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Parameter 'id' harus berformat UUID.",
			domainerrors.Detail{Field: "path.id", Rule: "uuid", Message: "Format UUID tidak valid"},
		))
		return
	}

	result, err := h.svc.GetPresignedDownloadURL(c.Request.Context(), docID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, downloadResponseJSON{
		DocumentID:   result.DocumentID.String(),
		PresignedURL: result.PresignedURL,
		ExpiresAt:    result.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// detectMimeByExtension fallback MIME detection berdasarkan ekstensi file.
// Hanya untuk tipe yang umum di BLIPS (dokumen keuangan).
func detectMimeByExtension(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".xlsx"):
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case strings.HasSuffix(lower, ".xls"):
		return "application/vnd.ms-excel"
	case strings.HasSuffix(lower, ".csv"):
		return "text/csv"
	case strings.HasSuffix(lower, ".docx"):
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case strings.HasSuffix(lower, ".doc"):
		return "application/msword"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}
