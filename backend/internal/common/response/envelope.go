// Package response menyediakan helper untuk menulis success dan error envelope
// sesuai api-conventions.md §"Success envelope" dan §"Error envelope".
//
// Handler WAJIB pakai helper ini — jangan tulis JSON raw di handler.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// Meta adalah bagian meta dari setiap response.
type Meta struct {
	TraceID string `json:"traceId"`
}

// SuccessEnvelope adalah struktur response sukses (non-list).
type SuccessEnvelope struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}

// ListEnvelope adalah struktur response list (dengan pagination).
type ListEnvelope struct {
	Data          any            `json:"data"`
	Pagination    *PaginationMeta `json:"pagination"`
	AppliedSort   []SortApplied  `json:"appliedSort,omitempty"`
	AppliedFilter map[string]any `json:"appliedFilter,omitempty"`
	Meta          Meta           `json:"meta"`
}

// PaginationMeta adalah metadata pagination cursor-based (DEC-022).
type PaginationMeta struct {
	NextCursor    *string `json:"nextCursor"`
	HasMore       bool    `json:"hasMore"`
	TotalEstimate *int64  `json:"totalEstimate,omitempty"`
	Limit         int     `json:"limit"`
}

// SortApplied adalah satu kolom sort yang sedang aktif.
type SortApplied struct {
	Col string `json:"col"`
	Dir string `json:"dir"`
}

// ErrorEnvelope adalah struktur response error.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody adalah isi dari error envelope.
type ErrorBody struct {
	Code    string                  `json:"code"`
	Message string                  `json:"message"`
	Details []domainerrors.Detail   `json:"details"`
	TraceID string                  `json:"traceId"`
}

// traceIDKey adalah key context untuk trace ID di Gin.
const TraceIDKey = "X-Trace-Id"

// getTraceID mengambil trace ID dari Gin context.
func getTraceID(c *gin.Context) string {
	if v, ok := c.Get(TraceIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return c.GetHeader("X-Trace-Id")
}

// OK menulis success envelope dengan HTTP 200.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, SuccessEnvelope{
		Data: data,
		Meta: Meta{TraceID: getTraceID(c)},
	})
}

// Created menulis success envelope dengan HTTP 201.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, SuccessEnvelope{
		Data: data,
		Meta: Meta{TraceID: getTraceID(c)},
	})
}

// List menulis list envelope dengan HTTP 200.
func List(c *gin.Context, data any, pagination *PaginationMeta, sorts []SortApplied, filters map[string]any) {
	c.JSON(http.StatusOK, ListEnvelope{
		Data:          data,
		Pagination:    pagination,
		AppliedSort:   sorts,
		AppliedFilter: filters,
		Meta:          Meta{TraceID: getTraceID(c)},
	})
}

// Accepted menulis response 202 untuk async job submission.
func Accepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, SuccessEnvelope{
		Data: data,
		Meta: Meta{TraceID: getTraceID(c)},
	})
}

// Error menulis error envelope.
// Jika err adalah *DomainError, gunakan code + message + details-nya.
// Jika tidak, gunakan INTERNAL 500.
func Error(c *gin.Context, err error) {
	traceID := getTraceID(c)

	if de, ok := domainerrors.IsDomainError(err); ok {
		details := de.Details()
		if details == nil {
			details = []domainerrors.Detail{}
		}
		c.JSON(de.HTTPStatus(), ErrorEnvelope{
			Error: ErrorBody{
				Code:    string(de.Code()),
				Message: de.Message(),
				Details: details,
				TraceID: traceID,
			},
		})
		return
	}

	// Non-domain error: mask internal details, hanya expose traceID.
	c.JSON(http.StatusInternalServerError, ErrorEnvelope{
		Error: ErrorBody{
			Code:    string(domainerrors.CodeInternal),
			Message: "Terjadi kesalahan internal. Hubungi admin dengan traceId.",
			Details: []domainerrors.Detail{},
			TraceID: traceID,
		},
	})
}

// ErrorWithStatus menulis error envelope dengan HTTP status eksplisit
// (untuk kasus replay idempotency yang butuh return status original).
func ErrorWithStatus(c *gin.Context, status int, code domainerrors.Code, message string, details []domainerrors.Detail) {
	traceID := getTraceID(c)
	if details == nil {
		details = []domainerrors.Detail{}
	}
	c.JSON(status, ErrorEnvelope{
		Error: ErrorBody{
			Code:    string(code),
			Message: message,
			Details: details,
			TraceID: traceID,
		},
	})
}

// NoContent menulis HTTP 204 tanpa body.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}
