package restore

import (
	"strings"
	"testing"
)

// TestBuildHydrateCommand_BareForm pins the post-Fix-3 bare invocation shape
// at the unit (unexported function) level. Companion to the integration-style
// snapshot in session_test.go (TestSessionRestorer_HydrateCommandFormat) —
// this file exists as a white-box test (package restore) so unexported
// helpers can be exercised directly with synthetic edge-case inputs.
func TestBuildHydrateCommand_BareForm(t *testing.T) {
	t.Run("typical inputs produce bare invocation without sh -c envelope", func(t *testing.T) {
		got := buildHydrateCommand("/x.fifo", "/y.bin", "work:0.0")
		want := "portal state hydrate --fifo /x.fifo --file /y.bin --hook-key work:0.0"
		if got != want {
			t.Errorf("buildHydrateCommand:\n got %q\nwant %q", got, want)
		}
	})

	t.Run("empty hookKey produces well-formed bare invocation", func(t *testing.T) {
		got := buildHydrateCommand("/x.fifo", "/y.bin", "")
		want := "portal state hydrate --fifo /x.fifo --file /y.bin --hook-key "
		if got != want {
			t.Errorf("buildHydrateCommand empty hook-key:\n got %q\nwant %q", got, want)
		}
	})

	t.Run("single-quote bearing inputs round-trip through shellQuoteSingle", func(t *testing.T) {
		// shellQuoteSingle replaces each ' with '\'' (close, escaped quote,
		// reopen). Defensive parity with the prior call-site contract so any
		// future re-introduction of an outer single-quoted envelope would not
		// change embedded-quote semantics.
		got := buildHydrateCommand("/x'.fifo", "/y'.bin", "sess'name:0.0")
		want := `portal state hydrate --fifo /x'\''.fifo --file /y'\''.bin --hook-key sess'\''name:0.0`
		if got != want {
			t.Errorf("buildHydrateCommand quoted:\n got %q\nwant %q", got, want)
		}
	})

	t.Run("output contains no sh -c envelope or exec $SHELL trailer", func(t *testing.T) {
		// Defect-D regression guard: the wrapper drop must not silently
		// reappear via a future refactor. Asserts the negative directly.
		got := buildHydrateCommand("/x.fifo", "/y.bin", "work:0.0")
		if strings.Contains(got, "sh -c") {
			t.Errorf("buildHydrateCommand output %q must not contain `sh -c`", got)
		}
		if strings.Contains(got, "exec $SHELL") {
			t.Errorf("buildHydrateCommand output %q must not contain `exec $SHELL`", got)
		}
	})
}
