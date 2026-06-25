package pocidelta

// db_adapter.go — SQLDBAdapter wraps *sql.DB to satisfy DBTxBeginner (B1 fix).
// Injected via NewServiceWithDB in cmd/api/main.go.

import (
	"context"
	"database/sql"
)

// SQLDBAdapter wraps *sql.DB and implements DBTxBeginner.
// Use NewSQLDBAdapter to create; inject into NewServiceWithDB.
type SQLDBAdapter struct {
	db *sql.DB
}

// NewSQLDBAdapter creates a DBTxBeginner backed by the given *sql.DB.
// Returns nil if db is nil (NewServiceWithDB handles nil gracefully).
func NewSQLDBAdapter(db *sql.DB) *SQLDBAdapter {
	if db == nil {
		return nil
	}
	return &SQLDBAdapter{db: db}
}

// BeginTxContext opens a new read-write transaction.
// Implements DBTxBeginner.
func (a *SQLDBAdapter) BeginTxContext(ctx context.Context) (*sql.Tx, error) {
	return a.db.BeginTx(ctx, nil)
}
