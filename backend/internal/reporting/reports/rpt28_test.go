package reports_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
	_ "blips-ifrs9.tugu-re.com/internal/reporting/reports/impl"
)

func TestRPT28_RegisteredInRegistry(t *testing.T) {
	r, ok := reports.Registry["rpt-28"]
	require.True(t, ok, "rpt-28 must be in registry")
	assert.Equal(t, "rpt-28", r.Slug())
	assert.Equal(t, "report.rpt-28.read", r.Permission())
	assert.Equal(t, "report.rpt-28.export", r.ExportPermission())
	assert.True(t, r.RegulatedFlag(), "rpt-28 must have RegulatedFlag=true")
}

func TestRPT28_QueryIsNoOp(t *testing.T) {
	r := reports.Registry["rpt-28"]
	seq, total, err := r.Query(context.Background(), nil, reports.QueryParams{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	// Iterate — must not panic
	count := 0
	for range seq {
		count++
	}
	assert.Equal(t, 0, count)
}

func TestRPT28_AllowedFilterContainsPeriodeID(t *testing.T) {
	r := reports.Registry["rpt-28"]
	found := false
	for _, f := range r.AllowedFilter() {
		if f.Col == "periode_id" {
			found = true
			break
		}
	}
	assert.True(t, found, "rpt-28 AllowedFilter must include periode_id")
}

func TestRPT28_MFAStepUpCheck_ServiceLevel(t *testing.T) {
	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	// Claims with no step-up
	claims := &auth.Claims{
		Sub:         "user-cfo",
		Permissions: []string{"report.rpt-28.export"},
		// StepupVerifiedAt nil → NeedsStepUp = true
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	_, err := svc.ExportRegulatorPack(ctx, reports.RegulatorPackRequest{PeriodeID: "PRD-2026-06", Format: "xlsx"}, claims)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeStepUpRequired, de.Code())
}

func TestRPT28_PermissionDenied_WrongRole(t *testing.T) {
	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	now := int64(9999999999) // future → step-up fresh
	claims := &auth.Claims{
		Sub:              "user-risk",
		Permissions:      []string{"report.rpt-13.read"}, // ROLE-RISK, not CFO
		StepupVerifiedAt: &now,
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	_, err := svc.ExportRegulatorPack(ctx, reports.RegulatorPackRequest{PeriodeID: "PRD-2026-06", Format: "xlsx"}, claims)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeReportPermissionDenied, de.Code())
}

func TestRPT28_MissingPeriodeID(t *testing.T) {
	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	now := int64(9999999999)
	claims := &auth.Claims{
		Sub:              "user-cfo",
		Permissions:      []string{"report.rpt-28.export"},
		StepupVerifiedAt: &now,
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	_, err := svc.ExportRegulatorPack(ctx, reports.RegulatorPackRequest{Format: "xlsx"}, claims)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeReportParamsInvalid, de.Code())
}

func TestRPT28_InvalidFormat(t *testing.T) {
	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	now := int64(9999999999)
	claims := &auth.Claims{
		Sub:              "user-cfo",
		Permissions:      []string{"report.rpt-28.export"},
		StepupVerifiedAt: &now,
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	_, err := svc.ExportRegulatorPack(ctx, reports.RegulatorPackRequest{PeriodeID: "PRD-2026-06", Format: "pdf"}, claims)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeReportParamsInvalid, de.Code())
}

func TestRPT28_NoAsynqClient(t *testing.T) {
	svc := reports.NewReportService(nil, nil, nil, nil, nil) // asynqClient nil
	now := int64(9999999999)
	claims := &auth.Claims{
		Sub:              "user-cfo",
		Permissions:      []string{"report.rpt-28.export"},
		StepupVerifiedAt: &now,
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	_, err := svc.ExportRegulatorPack(ctx, reports.RegulatorPackRequest{PeriodeID: "PRD-2026-06", Format: "xlsx"}, claims)
	require.Error(t, err) // CodeInternal: asynq client not configured
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeInternal, de.Code())
}
