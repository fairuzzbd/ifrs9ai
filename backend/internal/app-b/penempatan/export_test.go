package penempatan

// export_test.go exposes internals needed by *_test.go files in package penempatan_test.
// Only compiled during `go test`. No production binary impact.

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// DirectAuditWriter is the exported alias for the directAuditWriter interface,
// used by tests to inject a mock SoD audit writer.
type DirectAuditWriter = directAuditWriter

// WithSoDWriter replaces the SoD violation audit writer with a test double.
// Only for use in tests — allows asserting that SoD violations are audited.
func (s *Service) WithSoDWriter(w DirectAuditWriter) *Service {
	s.sodWriter = w
	return s
}

// AuditEventFromContext re-exports audit.EventFromContext for test packages.
var AuditEventFromContext = audit.EventFromContext

// RegisterRoutesForTest registers routes without auth / idempotency middleware
// (those are tested separately; here we want to test handler logic only).
func RegisterRoutesForTest(rg *gin.RouterGroup, h *Handler) {
	g := rg.Group("/trx/penempatan-deposito")
	{
		g.POST("", h.CreatePenempatan)
		g.GET("", h.ListPenempatan)
		g.GET("/:id", h.GetPenempatan)
		g.PATCH("/:id", h.UpdatePenempatan)
		g.DELETE("/:id", h.WithdrawPenempatan)
		g.POST("/:id/submit", h.SubmitPenempatan)
		g.POST("/:id/review", h.ReviewPenempatan)
		g.POST("/:id/approve", h.ApprovePenempatan)
		g.POST("/:id/reject", h.RejectPenempatan)
		g.POST("/:id/terminate", h.TerminatePenempatan)
		g.POST("/:id/terminate-review", h.TerminateReviewPenempatan)
		g.POST("/:id/terminate-approve", h.TerminateApprovePenempatan)
		g.POST("/:id/terminate-reject", h.TerminateRejectPenempatan)
		g.GET("/:id/eir-preview", h.GetEIRPreview)
		g.GET("/:id/audit-timeline", h.GetAuditTimeline)
	}
}

// StubService adapter that satisfies ServiceIface and delegates to the test stub.
// stubHolder is defined in handler_test.go within the test package.
// We need a bridge since the test file is in package penempatan_test but ServiceIface
// is in package penempatan. We expose NewStubService as a public test helper.

// TestStubHooks is the interface test files implement to supply stub functions.
type TestStubHooks interface {
	DoCreate(ctx context.Context, req CreateRequest, claims *auth.Claims) (*Penempatan, error)
	DoList(ctx context.Context, q listquery.Query, includeDeleted bool, claims *auth.Claims) (ListResult, error)
	DoGetByID(ctx context.Context, id uuid.UUID, claims *auth.Claims) (*Penempatan, error)
	DoUpdate(ctx context.Context, id uuid.UUID, req UpdateRequest, claims *auth.Claims) (*Penempatan, error)
	DoWithdraw(ctx context.Context, id uuid.UUID, claims *auth.Claims) error
	DoSubmit(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*Penempatan, error)
	DoReview(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*Penempatan, error)
	DoApprove(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*ApproveResult, error)
	DoReject(ctx context.Context, id uuid.UUID, req RejectActionRequest, claims *auth.Claims) (*Penempatan, error)
	DoTerminateRequest(ctx context.Context, id uuid.UUID, req TerminateRequestBody, claims *auth.Claims) (*Penempatan, error)
	DoTerminateReview(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*Penempatan, error)
	DoTerminateApprove(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*Penempatan, error)
	DoTerminateReject(ctx context.Context, id uuid.UUID, req RejectActionRequest, claims *auth.Claims) (*Penempatan, error)
	DoEIRPreview(ctx context.Context, id uuid.UUID, claims *auth.Claims) (*EIRPreviewResult, error)
	DoAuditTimeline(ctx context.Context, id uuid.UUID, claims *auth.Claims) ([]AuditTimelineEvent, error)
}

// hookAdapter wraps TestStubHooks as ServiceIface.
type hookAdapter struct{ h TestStubHooks }

func (a *hookAdapter) Create(ctx context.Context, req CreateRequest, cl *auth.Claims) (*Penempatan, error) {
	return a.h.DoCreate(ctx, req, cl)
}
func (a *hookAdapter) List(ctx context.Context, q listquery.Query, inc bool, cl *auth.Claims) (ListResult, error) {
	return a.h.DoList(ctx, q, inc, cl)
}
func (a *hookAdapter) GetByID(ctx context.Context, id uuid.UUID, cl *auth.Claims) (*Penempatan, error) {
	return a.h.DoGetByID(ctx, id, cl)
}
func (a *hookAdapter) Update(ctx context.Context, id uuid.UUID, req UpdateRequest, cl *auth.Claims) (*Penempatan, error) {
	return a.h.DoUpdate(ctx, id, req, cl)
}
func (a *hookAdapter) Withdraw(ctx context.Context, id uuid.UUID, cl *auth.Claims) error {
	return a.h.DoWithdraw(ctx, id, cl)
}
func (a *hookAdapter) Submit(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, cl *auth.Claims) (*Penempatan, error) {
	return a.h.DoSubmit(ctx, id, req, cl)
}
func (a *hookAdapter) Review(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, cl *auth.Claims) (*Penempatan, error) {
	return a.h.DoReview(ctx, id, req, cl)
}
func (a *hookAdapter) Approve(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, cl *auth.Claims) (*ApproveResult, error) {
	return a.h.DoApprove(ctx, id, req, cl)
}
func (a *hookAdapter) Reject(ctx context.Context, id uuid.UUID, req RejectActionRequest, cl *auth.Claims) (*Penempatan, error) {
	return a.h.DoReject(ctx, id, req, cl)
}
func (a *hookAdapter) TerminateRequest(ctx context.Context, id uuid.UUID, req TerminateRequestBody, cl *auth.Claims) (*Penempatan, error) {
	return a.h.DoTerminateRequest(ctx, id, req, cl)
}
func (a *hookAdapter) TerminateReview(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, cl *auth.Claims) (*Penempatan, error) {
	return a.h.DoTerminateReview(ctx, id, req, cl)
}
func (a *hookAdapter) TerminateApprove(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, cl *auth.Claims) (*Penempatan, error) {
	return a.h.DoTerminateApprove(ctx, id, req, cl)
}
func (a *hookAdapter) TerminateReject(ctx context.Context, id uuid.UUID, req RejectActionRequest, cl *auth.Claims) (*Penempatan, error) {
	return a.h.DoTerminateReject(ctx, id, req, cl)
}
func (a *hookAdapter) EIRPreview(ctx context.Context, id uuid.UUID, cl *auth.Claims) (*EIRPreviewResult, error) {
	return a.h.DoEIRPreview(ctx, id, cl)
}
func (a *hookAdapter) AuditTimeline(ctx context.Context, id uuid.UUID, cl *auth.Claims) ([]AuditTimelineEvent, error) {
	return a.h.DoAuditTimeline(ctx, id, cl)
}

// NewHandlerWithHooks creates a Handler backed by the given TestStubHooks (for tests).
func NewHandlerWithHooks(h TestStubHooks) *Handler {
	return &Handler{svc: &hookAdapter{h: h}}
}
