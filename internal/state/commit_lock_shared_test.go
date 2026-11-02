package state_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/state"
)

// The commit cycle and the session index store serialise only if they take one
// lock file, and callers judge contention by one sentinel whichever took it.
func TestCommitLockIsTheSharedCommitlock(t *testing.T) {
	if got, want := state.CommitLock("/s"), commitlock.Path("/s"); got != want {
		t.Errorf("CommitLock = %q, want commitlock.Path %q", got, want)
	}
	if !errors.Is(state.ErrCommitLockHeld, commitlock.ErrHeld) || !errors.Is(commitlock.ErrHeld, state.ErrCommitLockHeld) {
		t.Errorf("ErrCommitLockHeld and commitlock.ErrHeld are distinct errors")
	}
}
