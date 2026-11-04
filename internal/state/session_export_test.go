package state

import (
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
)

// SetBootTimeForTest fixes the boot time ClaimRule reads for the duration of
// the test, with the process's claim-rule warning not yet given.
func SetBootTimeForTest(t *testing.T, fn func() (time.Time, error)) {
	t.Helper()
	prev, warned := bootTime, warnedNoBootTime.Load()
	bootTime = fn
	warnedNoBootTime.Store(false)
	t.Cleanup(func() {
		bootTime = prev
		warnedNoBootTime.Store(warned)
	})
}

// BeforeForgetDiscardsHooksForTest runs fn with the commands Forget is about
// to discard, before each discard it makes, for the duration of the test.
func BeforeForgetDiscardsHooksForTest(t *testing.T, fn func(shown map[string]string)) {
	t.Helper()
	prev := discardHooks
	discardHooks = func(store *hooks.Store, event hooks.Event, shown map[string]string, via hooks.Via) ([]string, error) {
		fn(shown)
		return prev(store, event, shown, via)
	}
	t.Cleanup(func() { discardHooks = prev })
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
