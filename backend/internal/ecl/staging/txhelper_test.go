// Package staging_test — test helper for providing a no-op *sql.Tx in service tests.
//
// Because the service layer calls tx.Commit() / tx.Rollback() on the tx returned
// by repo.BeginTx(), we register a minimal "noop" sql driver that accepts all
// operations without doing any real I/O.
package staging_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"sync"
)

// noopDriver is a database/sql/driver that silently accepts all operations.
// Query results return 0 rows (io.EOF immediately).
type noopDriver struct{}

func (noopDriver) Open(_ string) (driver.Conn, error) { return &noopConn{}, nil }

type noopConn struct{}

func (*noopConn) Prepare(_ string) (driver.Stmt, error) { return &noopStmt{}, nil }
func (*noopConn) Close() error                          { return nil }
func (*noopConn) Begin() (driver.Tx, error)             { return &noopTx{}, nil }

type noopTx struct{}

func (*noopTx) Commit() error   { return nil }
func (*noopTx) Rollback() error { return nil }

type noopStmt struct{}

func (*noopStmt) Close() error                                 { return nil }
func (*noopStmt) NumInput() int                                { return -1 }
func (*noopStmt) Exec(_ []driver.Value) (driver.Result, error) { return driver.RowsAffected(0), nil }
func (*noopStmt) Query(_ []driver.Value) (driver.Rows, error)  { return &noopRows{}, nil }

// noopRows returns 0 rows (EOF immediately).
type noopRows struct{ done bool }

func (r *noopRows) Columns() []string { return nil }
func (r *noopRows) Close() error      { return nil }
func (r *noopRows) Next(_ []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	return io.EOF
}

// ─── oneRowDriver ─────────────────────────────────────────────────────────────
// oneRowDriver is a variant that returns ONE row of nil values per query.
// This allows `for rows.Next()` loop bodies to execute (with scan errors on typed fields),
// increasing statement coverage of repository scan-loop code.

var oneRowDriverRegistered sync.Once

type oneRowDriver struct{}

func (oneRowDriver) Open(_ string) (driver.Conn, error) { return &oneRowConn{}, nil }

type oneRowConn struct{}

func (*oneRowConn) Prepare(_ string) (driver.Stmt, error) { return &oneRowStmt{}, nil }
func (*oneRowConn) Close() error                          { return nil }
func (*oneRowConn) Begin() (driver.Tx, error)             { return &oneRowTx{}, nil }

type oneRowTx struct{}

func (*oneRowTx) Commit() error   { return nil }
func (*oneRowTx) Rollback() error { return nil }

type oneRowStmt struct{}

func (*oneRowStmt) Close() error                                 { return nil }
func (*oneRowStmt) NumInput() int                                { return -1 }
func (*oneRowStmt) Exec(_ []driver.Value) (driver.Result, error) { return driver.RowsAffected(1), nil }
func (*oneRowStmt) Query(_ []driver.Value) (driver.Rows, error) {
	return &oneRowRows{cols: []string{"c1"}}, nil
}

// oneRowRows returns exactly one row of nil values, then EOF.
type oneRowRows struct {
	cols []string
	used bool
}

func (r *oneRowRows) Columns() []string { return r.cols }
func (r *oneRowRows) Close() error      { return nil }
func (r *oneRowRows) Next(dest []driver.Value) error {
	if r.used {
		return io.EOF
	}
	r.used = true
	// Fill all destinations with nil (NULL).
	for i := range dest {
		dest[i] = nil
	}
	return nil
}

var oneRowTestDB *sql.DB

var noopTestDB *sql.DB

func init() {
	sql.Register("noop", noopDriver{})
	var err error
	noopTestDB, err = sql.Open("noop", "")
	if err != nil {
		panic("staging_test: failed to open noop db: " + err.Error())
	}

	oneRowDriverRegistered.Do(func() {
		sql.Register("onerow", oneRowDriver{})
	})
	oneRowTestDB, err = sql.Open("onerow", "")
	if err != nil {
		panic("staging_test: failed to open onerow db: " + err.Error())
	}
}

// beginNoopTx returns a real *sql.Tx backed by the noop driver.
// This satisfies service callers that do tx.Commit() / tx.Rollback().
func beginNoopTx(ctx context.Context) (*sql.Tx, error) {
	return noopTestDB.BeginTx(ctx, nil)
}
