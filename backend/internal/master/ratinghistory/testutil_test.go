package ratinghistory_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/ratinghistory"
)

// ─── Repository stub ─────────────────────────────────────────────────────────

type repoAdapter struct {
	createErr  error
	getByID    *stubGetByID
	getActive  *stubGetByID // used for GetActiveByCounterparty
	list       *stubList
	update     *stubUpdate
	softDelete *stubSoftDelete
	export     *stubExport
}

var _ ratinghistory.Repository = (*repoAdapter)(nil)

type stubGetByID struct {
	rh  *ratinghistory.RatingHistory
	err error
}

type stubList struct {
	items []*ratinghistory.RatingHistory
	err   error
}

type stubUpdate struct {
	updated *ratinghistory.RatingHistory
	err     error
}

type stubSoftDelete struct {
	deleted *ratinghistory.RatingHistory
	err     error
}

type stubExport struct {
	reader io.Reader
	count  int
	err    error
}

// ─── Interface implementation ─────────────────────────────────────────────────

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, _ *ratinghistory.RatingHistory) error {
	return a.createErr
}

func (a *repoAdapter) GetByID(_ context.Context, _ uuid.UUID) (*ratinghistory.RatingHistory, error) {
	if a.getByID == nil {
		return nil, nil
	}
	return a.getByID.rh, a.getByID.err
}

func (a *repoAdapter) GetByKode(_ context.Context, _ string) (*ratinghistory.RatingHistory, error) {
	if a.getByID == nil {
		return nil, nil
	}
	return a.getByID.rh, a.getByID.err
}

func (a *repoAdapter) GetActiveByCounterparty(_ context.Context, _ uuid.UUID) (*ratinghistory.RatingHistory, error) {
	if a.getActive == nil {
		return nil, nil
	}
	return a.getActive.rh, a.getActive.err
}

func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int) ([]*ratinghistory.RatingHistory, error) {
	if a.list != nil {
		return a.list.items, a.list.err
	}
	return nil, nil
}

func (a *repoAdapter) ListByCounterparty(_ context.Context, _ uuid.UUID, _ string, _ int) ([]*ratinghistory.RatingHistory, error) {
	if a.list != nil {
		return a.list.items, a.list.err
	}
	return nil, nil
}

func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ ratinghistory.UpdateFields) (*ratinghistory.RatingHistory, error) {
	if a.update != nil {
		return a.update.updated, a.update.err
	}
	return nil, nil
}

func (a *repoAdapter) CloseActiveRating(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ string, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*ratinghistory.RatingHistory, error) {
	if a.softDelete != nil {
		return a.softDelete.deleted, a.softDelete.err
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ ratinghistory.WorkflowStatus, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) SetSICRFlags(_ context.Context, _ *sql.Tx, _ uuid.UUID, _, _ bool, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	return nil, errTestNoDB
}

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]ratinghistory.AuditHistoryItem, bool, error) {
	return nil, false, nil
}

func (a *repoAdapter) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	if a.export != nil {
		return a.export.reader, a.export.count, a.export.err
	}
	return nil, 0, nil
}

var errTestNoDB = fmt.Errorf("test: no database available")

// ─── Counterparty repo stub ───────────────────────────────────────────────────

type stubCPRepo struct {
	updateCacheErr error
}

func (s *stubCPRepo) UpdateRatingCache(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ *string, _ uuid.UUID) error {
	return s.updateCacheErr
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

func testRatingHistory() *ratinghistory.RatingHistory {
	makerID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	cpID := uuid.MustParse("00000000-0000-0000-0000-000000000010")
	now := time.Now()
	return &ratinghistory.RatingHistory{
		ID:                     uuid.MustParse("00000000-0000-0000-0000-000000000020"),
		RatingHistoryIDKode:    "RH-2026-001",
		CounterpartyID:         cpID,
		TanggalBerlaku:         "2026-01-01",
		TanggalBerakhir:        nil,
		RatingPefindo:          "idA",
		SumberRating:           "PEFINDO",
		TanggalPublikasiRating: "2026-01-01",
		ActionType:             ratinghistory.ActionInitial,
		NotchChange:            0,
		SicrTriggered:          false,
		DefaultTriggered:       false,
		MakerID:                makerID,
		CreatedAt:              now,
		RowVersion:             1,
		TenantID:               "TUGURE",
		WorkflowStatus:         ratinghistory.WorkflowStatusDraft,
	}
}
