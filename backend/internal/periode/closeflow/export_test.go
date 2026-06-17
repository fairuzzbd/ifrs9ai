package closeflow

// export_test.go — Exports internal helpers for the _test package.
// This file is compiled only during testing (package closeflow, not closeflow_test).

import (
	"time"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// EmptyListQuery returns a zero-value listquery.Query for use in service tests.
func EmptyListQuery() listquery.Query {
	return listquery.Query{}
}

// ReopenMessage exports the internal reopenMessage helper for coverage testing.
func ReopenMessage(fromClosed, fxUnlocked bool) string {
	return reopenMessage(fromClosed, fxUnlocked)
}

// ExpireAllowlistCache zeroes the refreshedAt timestamp on the middleware's
// allowlistCache so that the next call to refreshAllowlistIfStale sees isStale=true.
// This is only used in tests to exercise the stale-refresh code path.
func (m *PeriodeLockMiddleware) ExpireAllowlistCache() {
	m.allowlistCache.mu.Lock()
	m.allowlistCache.refreshedAt = time.Time{} // zero value → always stale
	m.allowlistCache.mu.Unlock()
}
