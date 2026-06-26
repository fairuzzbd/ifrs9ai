package pocidelta

// repo_test.go — Repository stub interface compliance test.
// Verifies stubRepo implements Repository at compile time.
// Full sqlx integration tests are in the integration test suite (db_test tag).

import (
	"testing"
)

// compile-time interface compliance check
var _ Repository = (*stubRepo)(nil)

func TestStubRepo_InterfaceCompliance(t *testing.T) {
	// This test passes if stubRepo compiles — it confirms the stub satisfies
	// the Repository interface. Actual DB behaviour is integration-tested.
	t.Log("stubRepo implements Repository interface")
}
