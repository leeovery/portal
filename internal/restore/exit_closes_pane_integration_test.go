//go:build integration

// Phase 3 task 3-3 — real-tmux integration test gating spec § AC5
// (exit typed once in a restored pane closes the pane) and the
// "no parked sh wrapper" side effect of Fix 3 (wrapper drop in
// internal/restore/session.go's buildHydrateCommand).
//
// AC5: `exit` typed in a restored pane closes the pane on the first
// invocation. No orphan `sh` parent process under tmux for any
// restored pane. (See specification.md § Acceptance Criteria → AC5
// and § Manual Verification Protocol → "Additional post-fix checks".)
//
// What this test exercises end-to-end:
//
//   - Builds the portal binary so each restored pane's `portal state
//     hydrate` helper resolves on PATH (the helper is spawned via
//     respawn-pane -k by restore step 5 and must execute to dump
//     scrollback + unset its marker + exec $SHELL).
//   - Captures a single saved session with one window and one pane,
//     persists sessions.json, kills the server, restores against a
//     fresh server.
//   - Drives signal-hydrate via direct FIFO byte writes (the
//     byte-equivalent of `portal state signal-hydrate`).
//   - Polls state.ListSkeletonMarkers with a 2-second deadline; pass
//     condition is empty marker set within the window. This proves
//     the helper reached its post-dump exec-shell step.
//   - Sends "exit" via tmux send-keys to the live pane. Polls
//     list-panes for pane-gone within 2 seconds — the AC5 contract.
//   - The third sub-test additionally `pgrep`s for any parked `sh -c`
//     wrapper around `portal state hydrate`, which under the pre-fix
//     wrapper-shell shape would have appeared on every restored pane.
//
// Why this gates AC5 and existing tests don't:
//
//   - Existing reboot round-trip tests (cmd/bootstrap/reboot_roundtrip_
//     test.go) assert structure, layout, hooks, and ANSI scrollback
//     fidelity post-restore. They do NOT type `exit` in a restored
//     pane and assert the pane closes. A regression that re-introduces
//     the `sh -c '<helper>; exec $SHELL'` wrapper around
//     buildHydrateCommand would leave those tests green (the helper
//     still runs and side-effects fire) but break the user's "exit
//     closes pane on first invocation" experience — surfaced here.
//
//   - The third sub-test makes the "no parked sh parent" side-effect
//     observable: under the pre-fix wrapper, `pgrep` would find
//     `sh -c 'portal state hydrate ...; exec $SHELL'` on every
//     restored pane. Post-fix, the bare `portal state hydrate` is
//     pane's initial process and `syscall.Exec`s its replacement
//     directly — no parked parent.
//
// Build & run:
//
//	go test -tags=integration ./internal/restore/...
//	go test -short -tags=integration ./internal/restore/...   # skips this
//
// Tests in this file are NOT included in the default `go test ./...`
// run because the `//go:build integration` tag gates them off. They
// also call `testing.Short()` so the short-mode CI lane skips them.
//
// Portability notes:
//
//   - `pgrep -fl <pattern>`: -f matches the full command line, -l
//     prints the matched line. Both flags are present on macOS and
//     Linux. The exit code is 0 when at least one match is found and
//     1 when no match is found; the test treats exit-1-with-empty-
//     output as the pass condition rather than a failure.
//   - `tmux send-keys ... "exit" Enter`: client.SendKeys appends the
//     "Enter" key automatically, so the test passes the literal
//     string "exit" (no trailing newline).
//   - tmux pane-gone semantics: when the LAST pane in a session
//     closes, the SESSION goes away too. After `exit`, the session
//     lookup may return a non-zero exit code from `list-panes -t
//     <session>` because the session is gone. The polling helper
//     accepts both "list-panes succeeded but pane index missing"
//     and "list-panes failed (session gone)" as pane-gone signals.

package restore_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// exitClosesPaneBudget is the spec-mandated upper bound for both the
// post-restore marker-clearance poll and the post-exit pane-gone
// poll. Surface as a named constant so a change to the spec contract
// is a one-site edit and the diagnostic messages can self-reference
// the budget without drift. Spec § AC5: pane closes on first `exit`;
// the 2s budget mirrors the AC1 marker-clearance contract for
// consistency across the resurrection acceptance suite.
const exitClosesPaneBudget = 2 * time.Second

// TestExitClosesRestoredPane_NoHook is the bare-no-hook variant. The
// helper has no on-resume hook to run, so after scrollback dump +
// settle + marker-unset it `syscall.Exec`s the bare $SHELL directly
// under tmux (no inner `sh -c` envelope from the hook path either).
// `exit` typed at the shell terminates the only process in the pane,
// and tmux closes the pane.
//
// Pre-fix wrapper-shape regression: with `sh -c '<helper>; exec
// $SHELL'` re-introduced, typing `exit` once would terminate the
// inner $SHELL but return control to the outer sh, which would then
// `exec $SHELL` again — the user would have to type `exit` twice to
// close the pane. The post-exit pane-gone poll would expire and the
// test would fail with a "pane survived after exit" diagnostic.
func TestExitClosesRestoredPane_NoHook(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	stateDir, ts, sessionName := setupExitClosesPane(t, "")

	driveAndAwaitMarkerClear(t, ts.Client(), stateDir, sessionName)

	target := tmux.PaneTarget(sessionName, 0, 0)
	if err := ts.Client().SendKeys(target, "exit"); err != nil {
		t.Fatalf("SendKeys exit: %v", err)
	}

	awaitPaneGone(t, ts, sessionName, 0, 0, exitClosesPaneBudget)
}

// TestExitClosesRestoredPane_WithHook is the with-on-resume-hook
// variant. The helper exec's the user's hook command via the inner
// `sh -c '<HOOK>; exec $SHELL'` envelope (this inner wrapper is
// independent of the outer wrapper that Fix 3 dropped — see spec
// § Fix 3 → "Inner Hook-Firing Wrapper Is Untouched"). The hook
// command writes a sentinel file the test asserts on, proving the
// hook fired.
//
// Critical AC5 anchor: the inner `sh -c '<HOOK>; exec $SHELL'`
// envelope's `<HOOK>` runs once and exits; `exec $SHELL` then
// replaces the outer sh with $SHELL in-place. By the time the user
// types `exit`, the sh process has already been replaced — there is
// no parked parent to fall back to. So even with a hook registered,
// `exit` typed once at the shell terminates the only live process in
// the pane and tmux closes the pane.
func TestExitClosesRestoredPane_WithHook(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	// Sentinel path is unique per test so concurrent test runs (e.g.
	// `go test -count=N`) don't race on a shared filename. t.TempDir
	// is per-test and cleaned up automatically; the sentinel file
	// inherits that lifecycle.
	sentinel := filepath.Join(t.TempDir(), "hook-fired")
	hookCmd := fmt.Sprintf("echo hook-fired > %s", sentinel)

	stateDir, ts, sessionName := setupExitClosesPane(t, hookCmd)

	driveAndAwaitMarkerClear(t, ts.Client(), stateDir, sessionName)

	// Hook firing happens inside the helper's exec chain after the
	// marker is unset (spec § Resume Hooks → "fire from inside the
	// helper's exec chain ... at the end of successful hydration").
	// Marker clearance is therefore the upstream barrier; once the
	// barrier passes the hook command has been kicked off via
	// `sh -c '<HOOK>; exec $SHELL'`, but the sentinel write may take
	// a brief settle to hit disk. Poll up to the same 2s budget.
	// A missing sentinel after the budget means the hook did NOT fire
	// — that is the original Symptom B (AC2) shape, but it also blocks
	// AC5 because if the inner `sh -c '<HOOK>; exec $SHELL'` never
	// reached `exec $SHELL`, the pane is stuck running the hook
	// forever and `exit` would not close it.
	restoretest.WaitForFileExists(t, sentinel, exitClosesPaneBudget, 50*time.Millisecond)

	target := tmux.PaneTarget(sessionName, 0, 0)
	if err := ts.Client().SendKeys(target, "exit"); err != nil {
		t.Fatalf("SendKeys exit: %v", err)
	}

	awaitPaneGone(t, ts, sessionName, 0, 0, exitClosesPaneBudget)
}

// TestNoParkedShWrapperPostRestore is the "no parked sh parent"
// regression guard for Fix 3 → Side Effects. After restore + signal-
// drive, no `sh -c '...portal state hydrate...'` process should be
// running anywhere on the system (we use a session-name suffix in
// the pgrep pattern to avoid false-positives from concurrent test
// runs that might also have hydrate processes alive — the suffix
// matches only this test's session name).
//
// Pre-fix wrapper-shape regression: the previous outer wrapper
// `sh -c 'portal state hydrate --fifo X --file Y --hook-key Z;
// exec $SHELL'` left a parked sh parent under tmux for the lifetime
// of the pane. After this fix, the bare `portal state hydrate`
// invocation is the pane's initial process and `syscall.Exec`s its
// replacement directly — no parked parent.
//
// We tolerate up to a 2s settle window before the pgrep — the
// helper's 100 ms post-dump sleep + tmux command latency means the
// `portal state hydrate` process is still alive briefly after the
// FIFO write returns, and we don't want to false-positive on the
// helper itself. The pattern specifically matches `sh -c.*portal
// state hydrate` so the bare-helper hit (no `sh -c` prefix) is
// excluded by construction.
func TestNoParkedShWrapperPostRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	stateDir, ts, sessionName := setupExitClosesPane(t, "")

	driveAndAwaitMarkerClear(t, ts.Client(), stateDir, sessionName)

	// Settle window: give the helper 2s to complete its post-dump
	// settle sleep + syscall.Exec the bare $SHELL. Without this, a
	// fast pgrep could observe the still-running `portal state
	// hydrate` process — but our pattern excludes the bare-helper
	// shape, so settling is defence-in-depth, not correctness-
	// critical. Surfaced as a named constant so the spec's 2s budget
	// stays the canonical reference.
	time.Sleep(exitClosesPaneBudget)

	// Suffix the pattern with the session-unique paneKey so a
	// concurrent test run (e.g. `go test -count=3`) cannot
	// false-positive on a sibling test's still-running helper.
	// state.SanitizePaneKey produces the same paneKey suffix the
	// hydrate command embeds via --hook-key / --fifo / --file
	// arguments; pgrep -f matches against the full command line so
	// any sh -c wrapper around `portal state hydrate ... <suffix>`
	// would be caught.
	paneSuffix := state.SanitizePaneKey(sessionName, 0, 0)
	pattern := "sh -c.*portal state hydrate.*" + paneSuffix

	cmd := exec.Command("pgrep", "-fl", pattern)
	out, err := cmd.CombinedOutput()
	// pgrep exit code semantics (POSIX-style, consistent across
	// macOS and Linux):
	//   0 — at least one match (FAILURE for AC5 — wrapper present)
	//   1 — no match (PASS for AC5 — no parked wrapper)
	//   2 — invalid syntax (test bug, fail loudly)
	//   3 — internal error (treat as test bug, fail loudly)
	if err == nil {
		t.Fatalf("AC5 violation: parked sh -c wrapper found around "+
			"portal state hydrate (Fix 3 → Side Effects: orphan sh "+
			"parent must be eliminated). pattern=%q\npgrep output:\n%s",
			pattern, out)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		switch code := exitErr.ExitCode(); code {
		case 1:
			// No match — the pass condition.
		default:
			t.Fatalf("pgrep -fl %q: unexpected exit code %d "+
				"(want 0=match[fail] or 1=no-match[pass]); err=%v\n"+
				"output:\n%s", pattern, code, err, out)
		}
	} else {
		t.Fatalf("pgrep -fl %q: non-exec.ExitError failure (pgrep "+
			"binary missing or unrunnable?): %v\noutput:\n%s",
			pattern, err, out)
	}
}

// setupExitClosesPane stands up the shared test scaffold for all
// three sub-tests:
//
//   - PORTAL_STATE_DIR + PORTAL_HOOKS_FILE pinned to t.TempDir paths
//     so the hydrate helper resolves the test's isolated config
//     locations rather than the user's real ~/.config/portal/.
//   - portal binary built and PATH-prepended so the helper resolves
//     on PATH inside each restored pane.
//   - One saved session with one window and one pane is captured
//     pre-kill, then the server is killed and a fresh one brought
//     up. Restore reconstructs the skeleton + arms the FIFO.
//   - If hookCmd is non-empty, an on-resume hook is registered
//     against the saved structural identifier (session:0.0) before
//     the kill-and-restore cycle. The hooks store path is the
//     PORTAL_HOOKS_FILE pinned above.
//
// Returns (stateDir, ts, sessionName) so callers can drive the FIFO
// signal, send keys, and assert via the same socket.
func setupExitClosesPane(t *testing.T, hookCmd string) (string, *tmuxtest.Socket, string) {
	t.Helper()

	binDir := restoretest.BuildPortalBinaryDir(t)
	restoretest.PrependPATH(t, binDir)

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	// Session names are namespaced per-test via t.Name() to avoid any
	// chance of a cross-test collision under `go test -count=N` or
	// parallel test packages — although tmuxtest.Socket already
	// isolates per-test via its own UNIX socket, descriptive session
	// names also make pgrep diagnostics easier to attribute.
	//
	// state.SanitizePaneKey is filesystem-safe-but-tmux-tolerant:
	// "/" maps to "_" and a leading "." is escaped, so even a
	// pathological t.Name() like "Test/Sub" yields a usable session
	// name. PaneTarget uses the session name verbatim, so we pre-
	// sanitise here for safety against tmux's own session-name rules
	// (no colons, no periods leading the name).
	sessionName := sanitizeSessionForTmux(t.Name())

	ts := tmuxtest.New(t, "ptl-exit-")
	client := ts.Client()

	// Stand up the saved session: one window, one pane. `sleep
	// infinity` keeps the pane alive across the capture without
	// producing scrollback noise — the in-pane content is irrelevant
	// to AC5 (the helper doesn't replay anything meaningful for an
	// empty scrollback file, but the marker clearance + exec $SHELL
	// chain still runs).
	ts.Run(t, "new-session", "-d", "-s", sessionName, "sleep", "infinity")
	ts.WaitForSession(t, sessionName, 2*time.Second)

	// CAPTURE.
	idx, err := state.CaptureStructure(client, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	if len(idx.Sessions) != 1 || idx.Sessions[0].Name != sessionName {
		t.Fatalf("expected one captured session named %q; got %+v",
			sessionName, idx.Sessions)
	}

	// Register the on-resume hook BEFORE persisting sessions.json.
	// The hook is keyed by the saved structural identifier
	// (session:0.0) — same shape buildHydrateCommand passes via
	// --hook-key, so the helper's hooks.LookupOnResume succeeds.
	if hookCmd != "" {
		store := hooks.NewStore(hooksPath)
		hookKey := tmux.PaneTarget(sessionName, 0, 0)
		if err := store.Set(hookKey, "on-resume", hookCmd); err != nil {
			t.Fatalf("hooks.Set: %v", err)
		}
	}

	// PERSIST sessions.json via EncodeIndex (the canonical writer)
	// so the on-disk schema matches what production code produces.
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(stateDir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	// KILL the server. The list-sessions check confirms the kill
	// actually took effect — a silently-still-alive server would
	// mask a Restore that did nothing.
	ts.KillServer()
	if _, err := ts.TryRun("list-sessions"); err == nil {
		t.Fatalf("list-sessions succeeded after kill-server; expected error")
	}
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// RESTORE against the freshly-started server.
	logger, err := state.OpenLogger(filepath.Join(stateDir, "portal.log"), false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	o := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}
	if err := restoreWithMarker(t, client, o); err != nil {
		t.Fatalf("restoreWithMarker: %v", err)
	}

	return stateDir, ts, sessionName
}

// driveAndAwaitMarkerClear is the post-restore "kick the helper"
// sequence shared by all three sub-tests: write a single byte to
// the pane's hydration FIFO (mirrors what `portal state signal-
// hydrate` does in production), then poll until the
// @portal-skeleton-* marker for that pane is unset. Marker
// clearance is the upstream barrier for "helper has reached its
// post-dump exec-shell step" — the AC5 contract assumes the helper
// is past that point before `exit` is typed.
//
// The 10s budget mirrors restoretest.WaitForSkeletonMarkersCleared
// (used by the canonical reboot round-trip). It is deliberately
// looser than AC5's 2s pane-close budget because the in-pane
// fork+exec for the helper can take >1s under parallel CI load —
// see internal/restoretest's documented retry posture.
func driveAndAwaitMarkerClear(t *testing.T, client *tmux.Client, stateDir, sessionName string) {
	t.Helper()
	restoretest.DriveSignalHydrate(t, client, stateDir, []string{sessionName})
	restoretest.WaitForSkeletonMarkersCleared(t, client, 10*time.Second)
}

// awaitPaneGone polls list-panes for the target session until the
// pane at (window, pane) is no longer present, or the budget
// elapses.
//
// tmux pane-gone semantics: when the LAST pane in a session closes,
// the SESSION itself goes away too. After `exit` in a single-pane
// session, `tmux list-panes -t <session>` may return a non-zero
// exit code (because the session is gone). Either of the following
// is a pass:
//
//   - list-panes succeeds but the pane index is absent from output
//     (the session has other panes that survived).
//   - list-panes returns an error (the session is gone — implies
//     the pane is gone too).
//
// On budget expiry the test fails with the most-recent observed
// output for diagnostic value.
func awaitPaneGone(t *testing.T, ts *tmuxtest.Socket, sessionName string, window, pane int, budget time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	wantPane := fmt.Sprintf("%d:%d", window, pane)
	var lastOut string
	var lastErr error
	for time.Now().Before(deadline) {
		out, err := ts.TryRun("list-panes", "-s", "-t", sessionName,
			"-F", "#{window_index}:#{pane_index}")
		lastOut, lastErr = out, err
		if err != nil {
			// list-panes failed — session likely gone, which
			// implies our pane is gone too. AC5 pass.
			return
		}
		if !strings.Contains(out, wantPane) {
			// list-panes succeeded but the pane is missing from
			// its output. AC5 pass.
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("AC5 violation: pane %s:%s survived %s after `exit` "+
		"(spec AC5: pane closes on first invocation). last "+
		"list-panes output=%q err=%v",
		sessionName, wantPane, budget, lastOut, lastErr)
}

// sanitizeSessionForTmux strips characters tmux's session-name
// parser rejects (notably ":" which tmux uses as the
// session:window.pane separator) from a free-form input and falls
// back to a static name if the result is empty.
//
// t.Name() can include slashes (sub-test names) and other tokens
// that are filesystem-safe but tmux-hostile. The single canonical
// sanitiser lives here so all three sub-tests produce sessions that
// new-session -s and tmux.PaneTarget accept verbatim.
func sanitizeSessionForTmux(name string) string {
	// Replace forbidden tmux session-name characters with '-'.
	// tmux rejects ':' (separator), '.' (window-pane separator
	// when leading), and whitespace. Slashes are accepted by tmux
	// but we replace them too for log-readability.
	replacer := strings.NewReplacer(
		":", "-",
		".", "-",
		"/", "-",
		" ", "-",
		"\t", "-",
	)
	out := replacer.Replace(name)
	if out == "" {
		out = "exit-test"
	}
	return out
}
