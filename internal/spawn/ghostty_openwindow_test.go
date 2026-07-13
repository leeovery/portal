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
// the Phase-3 permission-code recognition and the preserved Phase-2 catch-all
// behaviour are asserted at the mapping seam itself.
func TestMapGhosttyResult(t *testing.T) {
	t.Run("it maps a -1743 or -1712 osascript outcome to permission-required with driver-composed guidance", func(t *testing.T) {
		// -1743 = AppleEvent not-permitted/denied; -1712 = AppleEvent timeout.
		// Both are the OS's permission-wall signals, mapped to the generic
		// permission-required outcome with the driver-composed opaque guidance.
		cases := []struct {
			name string
			out  string
		}{
			{name: "not-permitted -1743", out: "0:42: execution error: Ghostty got an error: AppleScript error: Not authorised to send Apple events (-1743)"},
			{name: "timeout -1712", out: "0:42: execution error: AppleEvent timed out (-1712)"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				result := mapGhosttyResult(tc.out, 1, nil)

				if result.Outcome != OutcomePermissionRequired {
					t.Fatalf("Outcome = %v, want OutcomePermissionRequired", result.Outcome)
				}
				// The opaque combined output rides up verbatim as Detail.
				if !strings.Contains(result.Detail, tc.out) {
					t.Errorf("Detail = %q, want it to carry the opaque output %q", result.Detail, tc.out)
				}
				// The driver-composed guidance names the target terminal and the
				// Automation-settings hint (opaque; general code never parses it).
				if strings.TrimSpace(result.Guidance) == "" {
					t.Fatalf("Guidance = %q, want a non-empty driver-composed hint", result.Guidance)
				}
				if !strings.Contains(result.Guidance, "Ghostty") {
					t.Errorf("Guidance = %q, want it to name the target terminal %q", result.Guidance, "Ghostty")
				}
				if !strings.Contains(result.Guidance, "Automation") {
					t.Errorf("Guidance = %q, want it to carry the Automation-settings hint", result.Guidance)
				}
			})
		}
	})

	t.Run("it still maps a non-permission non-zero exit to spawn-failed (regression)", func(t *testing.T) {
		// -1728 (no such object) is a genuine spawn failure with no permission
		// signal, so the Phase-2 catch-all still folds it to spawn-failed.
		const body = "0:47: execution error: Ghostty got an error: AppleScript error (-1728)"
		result := mapGhosttyResult(body, 1, nil)

		if result.Outcome != OutcomeSpawnFailed {
			t.Errorf("Outcome = %v, want OutcomeSpawnFailed for a non-permission non-zero exit", result.Outcome)
		}
		if result.Guidance != "" {
			t.Errorf("Guidance = %q, want empty on the spawn-failed path", result.Guidance)
		}
	})

	t.Run("it still maps an execution error to spawn-failed (regression)", func(t *testing.T) {
		result := mapGhosttyResult("", 0, errors.New("boom"))

		if result.Outcome != OutcomeSpawnFailed {
			t.Errorf("Outcome = %v, want OutcomeSpawnFailed for an execution error", result.Outcome)
		}
	})

	t.Run("it still maps a clean exit to success (regression)", func(t *testing.T) {
		result := mapGhosttyResult("", 0, nil)

		if result.Outcome != OutcomeSuccess {
			t.Errorf("Outcome = %v, want OutcomeSuccess for a clean exit", result.Outcome)
		}
	})
}
