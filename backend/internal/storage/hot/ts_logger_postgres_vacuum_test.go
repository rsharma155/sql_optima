package hot

import "testing"

func TestVacuumProgressRowStructExists(t *testing.T) {
	// compile-time guard
	var _ PostgresVacuumProgressRow
}

