package matauang_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/matauang"
)

// repoAdapter is a flexible Repository stub that delegates to whichever
// sub-stub is set. Unset stubs use zero-value behavior.
// This avoids one giant struct with every combination of fields.
type repoAdapter struct {
	stub       *stubCreate
	getByKode  *stubGetByKode
	list       *stubList
	update     *stubUpdate
	softDelete *stubSoftDelete
	export     *stubExport
}

var _ matauang.Repository = (*repoAdapter)(nil)

func (a *repoAdapter) Create(_ context.Context, tx *sql.Tx, m *matauang.MataUang) error {
	if a.stub != nil {
		return a.stub.createErr
	}
	return nil
}

func (a *repoAdapter) GetByKode(_ context.Context, kode string, includeDeleted bool) (*matauang.MataUang, error) {
	// Try all the stubs that have a getByKodeResult.
	if a.getByKode != nil {
		return a.getByKode.result, a.getByKode.err
	}
	if a.update != nil {
		return a.update.getByKodeResult, nil
	}
	if a.softDelete != nil {
		return a.softDelete.getByKodeResult, nil
	}
	return nil, nil
}

func (a *repoAdapter) GetByID(_ context.Context, id uuid.UUID) (*matauang.MataUang, error) {
	if a.getByKode != nil {
		return a.getByKode.result, a.getByKode.err
	}
	return nil, nil
}

func (a *repoAdapter) List(_ context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*matauang.MataUang, error) {
	if a.list != nil {
		return a.list.listResult, a.list.listErr
	}
	return nil, nil
}

func (a *repoAdapter) Update(_ context.Context, tx *sql.Tx, kode string, fields matauang.UpdateFields) (*matauang.MataUang, error) {
	if a.update != nil {
		return a.update.updateResult, a.update.updateErr
	}
	return nil, nil
}

func (a *repoAdapter) SoftDelete(_ context.Context, tx *sql.Tx, kode string, deletedBy uuid.UUID) (*matauang.MataUang, error) {
	if a.softDelete != nil {
		return a.softDelete.softDeleteResult, a.softDelete.softDeleteErr
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatus(_ context.Context, tx *sql.Tx, id uuid.UUID, status matauang.WorkflowStatus, updatedBy uuid.UUID) error {
	return nil
}

func (a *repoAdapter) CountReferences(_ context.Context, kode string) (int64, error) {
	if a.softDelete != nil {
		return a.softDelete.countReferencesVal, nil
	}
	return 0, nil
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	// Return a non-nil error so service code that reaches BeginTx after passing
	// guard-path checks will fail gracefully (not panic on nil tx).
	// Tests that exercise the guard path (pre-tx) don't reach here.
	return nil, errTestNoDB
}

var errTestNoDB = fmt.Errorf("test: no database available")

func (a *repoAdapter) ListAuditHistory(_ context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]matauang.AuditHistoryItem, bool, error) {
	return nil, false, nil
}

func (a *repoAdapter) ExportAll(_ context.Context, q listquery.Query) (io.Reader, int, error) {
	if a.export != nil {
		return a.export.reader, a.export.count, a.export.err
	}
	return nil, 0, nil
}
