package reporting

import (
	"context"
	"database/sql"
	"log/slog"
)

// ChooseDB returns the appropriate *sql.DB for the given ReadIntent.
// ReadIntentReporting routes to the replica (MV_DSN) if available.
// Falls back to primary with a WARN log if replica is nil.
//
// S1-AC2: query rpt.mv_* → read-replica DSN.
// S1-AC3: MV_DSN not set → fallback primary + WARN log, no error.
func ChooseDB(primary, replica *sql.DB, intent ReadIntent) *sql.DB {
	if intent == ReadIntentReporting && replica != nil {
		slog.Default().Info("MV query routed to read-replica DSN")
		return replica
	}
	if intent == ReadIntentReporting && replica == nil {
		slog.Default().Warn("MV_DSN not set — falling back to primary DSN. Set MV_DSN for read-replica routing.")
	}
	return primary
}

// ChooseDBWithContext is ChooseDB with context for trace propagation.
func ChooseDBWithContext(ctx context.Context, primary, replica *sql.DB, intent ReadIntent) *sql.DB {
	if intent == ReadIntentReporting && replica != nil {
		slog.Default().InfoContext(ctx, "MV query routed to read-replica DSN")
		return replica
	}
	if intent == ReadIntentReporting && replica == nil {
		slog.Default().WarnContext(ctx, "MV_DSN not set — falling back to primary DSN. Set MV_DSN for read-replica routing.")
	}
	return primary
}
