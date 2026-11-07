package saver

import (
	"testing"
	"time"
)

// SetConfirmBudget bounds a poll's confirmations together at d for the rest
// of t, so a test of one running past the budget need not wait out the real
// one.
func SetConfirmBudget(t testing.TB, d time.Duration) {
	t.Helper()
	old := confirmBudget
	confirmBudget = d
	t.Cleanup(func() { confirmBudget = old })
}
