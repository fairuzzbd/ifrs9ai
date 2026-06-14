package penempatan

// service_iface.go — ServiceIface interface extracted from Service for handler injection.
// Having Handler depend on ServiceIface (interface) instead of *Service (concrete)
// enables unit-testing of handlers without a DB.

import (
	"context"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ServiceIface is the contract fulfilled by *Service.
// All handler methods call through this interface.
type ServiceIface interface {
	Create(ctx context.Context, req CreateRequest, claims *auth.Claims) (*Penempatan, error)
	List(ctx context.Context, q listquery.Query, includeDeleted bool, claims *auth.Claims) (ListResult, error)
	GetByID(ctx context.Context, id uuid.UUID, claims *auth.Claims) (*Penempatan, error)
	Update(ctx context.Context, id uuid.UUID, req UpdateRequest, claims *auth.Claims) (*Penempatan, error)
	Withdraw(ctx context.Context, id uuid.UUID, claims *auth.Claims) error
	Submit(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*Penempatan, error)
	Review(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*Penempatan, error)
	Approve(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*ApproveResult, error)
	Reject(ctx context.Context, id uuid.UUID, req RejectActionRequest, claims *auth.Claims) (*Penempatan, error)
	TerminateRequest(ctx context.Context, id uuid.UUID, req TerminateRequestBody, claims *auth.Claims) (*Penempatan, error)
	TerminateReview(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*Penempatan, error)
	TerminateApprove(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*Penempatan, error)
	TerminateReject(ctx context.Context, id uuid.UUID, req RejectActionRequest, claims *auth.Claims) (*Penempatan, error)
	EIRPreview(ctx context.Context, id uuid.UUID, claims *auth.Claims) (*EIRPreviewResult, error)
	AuditTimeline(ctx context.Context, id uuid.UUID, claims *auth.Claims) ([]AuditTimelineEvent, error)
}

// compile-time check: *Service satisfies ServiceIface.
var _ ServiceIface = (*Service)(nil)
