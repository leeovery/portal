// Tests in this file mutate package-level state via Cobra and MUST NOT use t.Parallel.
package cmd

// Firing-side regression gate complementing
// internal/tmux/hooks_register_six_event_routing_test.go's
// registration-level invariant.
//
// Spec references:
//   - § Acceptance Criteria item 9 — "Six other events untouched. ...
//     must continue to fire portal state notify (the cheap dirty-flag
//     touch). No structural capture cost is added to these events."
//   - § Acceptance Criteria item 13 — "Existing portal state notify
//     semantics preserved. notify still performs zero tmux calls, zero
//     sessions.json writes, and only touches save.requested."
//   - § Testing Requirements → Regression Tests → "Six-event eventual
//     consistency".
//
// Why this gate exists alongside TestStateNotify_DoesNotReadOrCreateOtherStateFiles:
// The existing test asserts the steady-state post-run shape. This
// regression test adds the bounded-window contract — within 500ms of
// firing notifyCommand (the body each of the six events fires),
// save.requested must exist and sessions.json must NOT exist. 500ms is
// comfortably under the daemon's 1s ticker period, so a sessions.json
// observation inside this window cannot come from a daemon-driven
// commit racing the test; it would only come from a regression that
// over-generalised commit-now's synchronous-write path back onto
// notify.
//
// The six events themselves are not driven directly here — per the
// task instructions, "if real-tmux simulator is impractical for some
// events (e.g., pane-focus-out is hard to drive deterministically),
// assert registration-level invariant + unit-level invariant that
// notifyCommand itself does no sessions.json work — that combination
// suffices." The registration-level invariant lives in
// internal/tmux/hooks_register_six_event_routing_test.go; this file
// pins the firing-level invariant on the shared notifyCommand body. The
// composition of the two assertions establishes the six-event eventual-
// consistency contract.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sixEventFiringWindow is the bounded "no sessions.json write" window
// the test asserts after firing notifyCommand. 500ms is comfortably
// under the daemon's 1s ticker period (see cmd/state_daemon.go
// TickerPeriod = 1 * time.Second), so any sessions.json observed
// inside this window is necessarily from the notify path itself, not
// a concurrent daemon tick. A regression that over-generalised commit-
// now's synchronous-write path back onto notify would surface here as
// a sessions.json that exists immediately after notify exits.
const sixEventFiringWindow = 500 * time.Millisecond

// TestNotifyCommand_TouchesSaveRequestedAndWritesNoSessionsJSON is the
// firing-side regression gate for spec § Acceptance Criteria items 9
// and 13. It exercises the notifyCommand body each of the six non-
// session-closed save-trigger events fires, and asserts:
//
//  1. save.requested exists immediately after the notify subprocess
//     exits — the dirty flag is the entire purpose of notify, and a
//     regression that dropped this touch would silently break the
//     daemon's eventual-consistency contract.
//  2. sessions.json does NOT exist within sixEventFiringWindow of
//     notify's exit — notify must perform zero tmux work and zero
//     sessions.json writes, leaving the structural commit to the
//     daemon's next tick. The 500ms window is under the daemon's 1s
//     ticker period, so an observed sessions.json could only come
//     from notify itself.
//
// The sub-test name pins the task's "Named assertions" deliverable
// verbatim so a regression failure surfaces with a self-documenting
// CI failure message.
func TestNotifyCommand_TouchesSaveRequestedAndWritesNoSessionsJSON(t *testing.T) {
	t.Run("firing notifyCommand touches save.requested and writes nothing to sessions.json", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)

		// runStateNotify is defined in state_notify_test.go in the
		// same package and resets cobra state across invocations.
		// Exit status zero is the documented happy-path contract.
		if _, _, err := runStateNotify(t); err != nil {
			t.Fatalf("notify subprocess failed: %v", err)
		}

		// Assertion 1: save.requested must exist. The dirty flag is
		// notify's entire purpose — its absence is a regression.
		saveRequestedPath := filepath.Join(dir, "save.requested")
		if _, err := os.Stat(saveRequestedPath); err != nil {
			t.Fatalf(
				"notify did not touch save.requested (the dirty-flag invariant "+
					"the six events rely on): stat err = %v\n"+
					"--- state dir listing ---\n%s",
				err, dumpStateDirForNotifyTest(dir),
			)
		}

		// Assertion 2: sessions.json must NOT exist within the bounded
		// window. The window is 500ms, comfortably under the daemon's
		// 1s ticker period, so an observed sessions.json could only
		// come from notify itself. The test polls every 25ms inside
		// the window — if any read at any sample point observes a
		// sessions.json, that is a regression.
		sessionsJSONPath := filepath.Join(dir, "sessions.json")
		deadline := time.Now().Add(sixEventFiringWindow)
		const pollInterval = 25 * time.Millisecond
		for time.Now().Before(deadline) {
			if _, err := os.Stat(sessionsJSONPath); err == nil {
				// File exists — regression. Surface the file's contents
				// in the failure message so the post-mortem shows
				// exactly what notify wrote.
				blob, _ := os.ReadFile(sessionsJSONPath)
				t.Fatalf(
					"notify wrote sessions.json within %s of exit "+
						"(spec § Acceptance Criteria items 9 and 13 "+
						"require zero sessions.json writes from notify):\n"+
						"--- sessions.json contents ---\n%s\n"+
						"--- state dir listing ---\n%s",
					sixEventFiringWindow, string(blob), dumpStateDirForNotifyTest(dir),
				)
			} else if !os.IsNotExist(err) {
				t.Fatalf("stat sessions.json during bounded window: %v", err)
			}
			time.Sleep(pollInterval)
		}

		// Final post-window assertion: sessions.json is still absent.
		// This is belt-and-braces over the loop above — the loop
		// returns early on file appearance, so reaching here means
		// no sample observed the file. Re-stating the assertion as a
		// final shape check pins the steady-state contract.
		if _, err := os.Stat(sessionsJSONPath); err == nil {
			blob, _ := os.ReadFile(sessionsJSONPath)
			t.Fatalf(
				"sessions.json exists at end of %s window (notify must not write it):\n"+
					"--- sessions.json contents ---\n%s",
				sixEventFiringWindow, string(blob),
			)
		} else if !os.IsNotExist(err) {
			t.Fatalf("final stat sessions.json: %v", err)
		}
	})
}

// dumpStateDirForNotifyTest returns a newline-joined listing of every
// entry under the state directory at the time of failure. The shape is
// "<name> (size=<bytes>, mode=<mode>)" per entry so a missing
// save.requested (notify dropped its dirty-flag touch) is visually
// distinct from an unexpected sessions.json (notify wrote structural
// state) in the failure log.
func dumpStateDirForNotifyTest(stateDir string) string {
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return fmt.Sprintf("(readdir %s: %v)", stateDir, err)
	}
	var lines []string
	for _, e := range entries {
		info, ierr := e.Info()
		if ierr != nil {
			lines = append(lines, fmt.Sprintf("%s (stat err: %v)", e.Name(), ierr))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s (size=%d, mode=%s)", e.Name(), info.Size(), info.Mode()))
	}
	return strings.Join(lines, "\n")
}
