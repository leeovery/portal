package spawn

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// fakeOsascriptRunner is a test double for osascriptRunner: it records the argv
// it is handed and returns a fabricated (out, exitCode, err) outcome, so the
// OpenWindow exec boundary and mapGhosttyResult are unit-testable without ever
// running real osascript or opening a real window.
type fakeOsascriptRunner struct {
	gotArgv  []string
	out      string
	exitCode int
	err      error
}

func (f *fakeOsascriptRunner) Run(argv []string) (string, int, error) {
	f.gotArgv = append([]string(nil), argv...)
	return f.out, f.exitCode, f.err
}

func TestGhosttyOpenWindow(t *testing.T) {
	t.Run("it hands the osascript argv to the runner", func(t *testing.T) {
		cmd := realAttachArgv()
		fake := &fakeOsascriptRunner{}
		adapter := &ghosttyAdapter{runner: fake}

		adapter.OpenWindow(cmd)

		want := ghosttyOpenArgv(cmd)
		if !slices.Equal(fake.gotArgv, want) {
			t.Errorf("runner received argv %#v, want ghosttyOpenArgv(command) %#v", fake.gotArgv, want)
		}
	})

	t.Run("it maps a clean osascript exit to success with an opaque detail", func(t *testing.T) {
		fake := &fakeOsascriptRunner{out: "", exitCode: 0, err: nil}
		adapter := &ghosttyAdapter{runner: fake}

		result := adapter.OpenWindow(realAttachArgv())

		if result.Outcome != OutcomeSuccess {
			t.Errorf("Outcome = %v, want OutcomeSuccess", result.Outcome)
		}
		if strings.TrimSpace(result.Detail) == "" {
			t.Errorf("Detail = %q, want a non-empty opaque detail", result.Detail)
		}
	})

	t.Run("it maps a non-zero osascript exit to spawn-failed with the opaque output", func(t *testing.T) {
		const body = "0:47: execution error: Ghostty got an error: AppleScript error (-1728)"
		fake := &fakeOsascriptRunner{out: body, exitCode: 1, err: nil}
		adapter := &ghosttyAdapter{runner: fake}

		result := adapter.OpenWindow(realAttachArgv())

		if result.Outcome != OutcomeSpawnFailed {
			t.Errorf("Outcome = %v, want OutcomeSpawnFailed", result.Outcome)
		}
		if !strings.Contains(result.Detail, body) {
			t.Errorf("Detail = %q, want it to carry the opaque combined output %q", result.Detail, body)
		}
	})

	t.Run("it maps an osascript execution error to spawn-failed", func(t *testing.T) {
		execErr := errors.New(`exec: "osascript": executable file not found in $PATH`)
		fake := &fakeOsascriptRunner{out: "", exitCode: 0, err: execErr}
		adapter := &ghosttyAdapter{runner: fake}

		result := adapter.OpenWindow(realAttachArgv())

		if result.Outcome != OutcomeSpawnFailed {
			t.Errorf("Outcome = %v, want OutcomeSpawnFailed (never a panic, never Success)", result.Outcome)
		}
		if strings.TrimSpace(result.Detail) == "" {
			t.Errorf("Detail = %q, want it to carry the execution-error text", result.Detail)
		}
	})
}

// TestMapGhosttyResult pins the pure outcome mapping directly (no runner), so
// the Phase-2 rule "every non-clean exit is spawn-failed, never
// permission-required" is asserted at the mapping seam itself.
func TestMapGhosttyResult(t *testing.T) {
	t.Run("it never returns permission-required in phase 2", func(t *testing.T) {
		// AppleEvent permission codes (-1743/-1712) still fold to spawn-failed
		// in Phase 2 — the permission-code mapping is deferred to Phase 3.
		cases := []struct {
			name     string
			out      string
			exitCode int
			err      error
		}{
			{name: "not-authorised -1743", out: "execution error: Not authorised (-1743)", exitCode: 1},
			{name: "no-user-interaction -1712", out: "execution error: timed out (-1712)", exitCode: 1},
			{name: "execution error", out: "", exitCode: 0, err: errors.New("boom")},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				result := mapGhosttyResult(tc.out, tc.exitCode, tc.err)
				if result.Outcome == OutcomePermissionRequired {
					t.Errorf("Outcome = OutcomePermissionRequired, want OutcomeSpawnFailed (permission mapping is Phase 3)")
				}
				if result.Outcome != OutcomeSpawnFailed {
					t.Errorf("Outcome = %v, want OutcomeSpawnFailed", result.Outcome)
				}
			})
		}
	})
}
