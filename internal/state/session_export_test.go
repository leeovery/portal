package state

import (
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
)

// SetBootTimeForTest fixes the boot time ClaimRule reads for the duration of
// the test.
func SetBootTimeForTest(t *testing.T, fn func() (time.Time, error)) {
	t.Helper()
	prev := bootTime
	bootTime = fn
	t.Cleanup(func() { bootTime = prev })
}

// AfterForgetReadsHooksForTest runs fn once Forget has read hooks.json, before
// it removes the hook it read, for the duration of the test.
func AfterForgetReadsHooksForTest(t *testing.T, fn func()) {
	t.Helper()
	prev := loadHooks
	loadHooks = func(store *hooks.Store, via hooks.Via) (hooks.Snapshot, error) {
		held, err := prev(store, via)
		fn()
		return held, err
	}
	t.Cleanup(func() { loadHooks = prev })
}
