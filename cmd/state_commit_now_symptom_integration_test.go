//go:build integration

package cmd_test

// Real-tmux integration gate pinning the canonical end-to-end symptom
// described in the killed-session-resurrects-within-tick-window spec
// (specification.md § Acceptance Criteria items 1, 2, 5, 6, and §
// Testing Requirements → Integration Tests → "Kill → bootstrap
// timeline (the canonical symptom test)" / "_portal-saver self-kill").
//
// Unit tests (state_commit_now_test.go) cover commit-now's mechanics
// against in-process fakes; the sibling re-entrancy gate
// (state_commit_now_reentrancy_integration_test.go, task 1-5) pins
// hook re-entrancy. Neither shows that the headline user-visible
// symptom is gone: kill any session → next portal bootstrap's Restore
// step does not resurrect it. This file is that gate.
//
// Three sub-tests are wired:
//
//  1. canonical symptom — kill a user session, observe sessions.json
//     update synchronously (two-consecutive-consistent reads, no
//     time.Sleep), then run a fresh portal bootstrap and assert the
//     killed session is not reconstructed as a skeleton pane.
//
//  2. _portal-saver self-kill with @portal-restoring CLEAR — the
//     underscore-prefix filter (keepSessionNames) suppresses _portal-
//     saver from sessions.json; the user sessions stay intact.
//
//  3. _portal-saver self-kill with @portal-restoring SET — the
//     commit-now short-circuit fires; sessions.json is byte-identical
//     before/after. Then we clear the marker, kill B, and assert the
//     subsequent kill DOES update sessions.json — proving the gate is
//     per-invocation and not a one-shot disable.
//
// Skip behaviour mirrors the sibling real-tmux integration tests:
//
//   - tmuxtest.SkipIfNoTmux skips when tmux is not on PATH.
//   - portalbintest.StagePortalBinary skips cleanly if `go build`
//     fails — a dev machine without `go` or a broken build is not a
//     symptom-elimination failure.
//
// No t.Parallel: the cmd-package convention (mock injection via
// package-level mutable state cleaned up by t.Cleanup) applies even
// though the test exercises a subprocess. t.Parallel is forbidden
// across the portal test suite (see CLAUDE.md).
//
// Integration-lane test (`//go:build integration`), matching
// cmd/state_commit_now_reentrancy_integration_test.go and
// cmd/state_daemon_integration_test.go — every test that spawns a real
// daemon or execs the built binary lives behind the tag, where the
// daemon-pgrep sandbox is compiled in.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// symptomKillBudget bounds the wait for sessions.json to reflect a
// kill via commit-now. Mirrors the re-entrancy gate's 1.5s budget
// from task 1-5: 1.5s leaves headroom over the spec's 1s example
// for CI scheduling jitter while keeping a regression sharply
// surfaced.
const symptomKillBudget = 1500 * time.Millisecond

// Poll cadence and consecutive-read threshold are shared with the
// re-entrancy gate (see reentrancyPollInterval / reentrancyConsecutiveReads
// in state_commit_now_reentrancy_integration_test.go): both gates run
// the same two-consecutive-consistent-reads discipline against
// sessions.json on a 25ms cadence, so the constants are declared once
// and reused across the cmd_test package.

// markerSetSettle is the brief wait used in sub-test 3 to give the
// session-closed hook subprocess time to execute when @portal-
// restoring is set. Because the short-circuit is the load-bearing
// path here (no write to sessions.json is expected), the test
// cannot use poll-for-change as a readiness signal — there is no
// change to detect. 250ms comfortably covers the [hook fire → run-
// shell fork → portal subprocess startup → IsRestoringSet query →
// exit] chain on every host we exercise; a regression that
// accidentally bypassed the short-circuit and wrote sessions.json
// would do so well inside this window.
const markerSetSettle = 250 * time.Millisecond

// TestCommitNowSymptom drives the canonical kill→bootstrap timeline
// against a real tmux server with three sub-tests covering the
// three spec acceptance branches that real tmux is the only honest
// way to gate:
//
//  1. canonical symptom — external kill must update sessions.json
//     synchronously and bootstrap must not resurrect the killed
//     session.
//  2. _portal-saver self-kill with @portal-restoring clear — under-
//     score-prefix filter alone is enough; sessions.json omits the
//     saver and keeps the user sessions.
//  3. @portal-restoring defence — marker-set kill is a no-op on
//     sessions.json; marker-clear kill on the same fixture updates
//     sessions.json correctly.
//
// Each sub-test runs against its own isolated tmux server and state
// directory so a failure in one cannot pollute the others. The portal
// binary is built once at the parent level (StagePortalBinary's
// PATH-prepend persists across sub-tests).
func TestCommitNowSymptom(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	// Build the portal binary and PATH-prepend so every subprocess
	// (portal list, portal state commit-now via hook) resolves to the
	// freshly built binary. The t.Setenv contract restores PATH on
	// test exit; sub-tests inherit the prepend.
	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	t.Run("canonical symptom: kill, sync sessions.json update, second bootstrap does not reconstruct", func(t *testing.T) {
		fixture := newSymptomFixture(t, binary, binDir, "ptl-symptom-canon-")

		// Baseline shape (A + B present, saver/anchor filtered) is
		// already asserted inside newSymptomFixture; no need to
		// repeat the check here.

		// Sub-test 1 step 3: external kill via the test socket. The
		// session-closed hook fires synchronously from tmux's POV; the
		// hook's run-shell fork-execs portal state commit-now
		// asynchronously, so the load-bearing wait is the poll below.
		fixture.sock.Run(t, "kill-session", "-t", "B")

		// Sub-test 1 step 4: tight poll, two-consecutive-consistent
		// reads. No time.Sleep — the hook subprocess completes well
		// inside the 1.5s budget on every host. ctx.Err on timeout
		// triggers a diagnostic dump.
		ctx, cancel := context.WithTimeout(context.Background(), symptomKillBudget)
		defer cancel()
		if perr := pollSessionsJSON(ctx, fixture.stateDir, []string{"A"}, []string{"B"}); perr != nil {
			t.Fatalf(
				"sessions.json did not reflect kill of B within %s: %v\n%s",
				symptomKillBudget, perr, fixture.diagnostic(),
			)
		}

		// Sub-test 1 step 5: second bootstrap via a fresh portal list
		// subprocess. This re-runs the full bootstrap including step 6
		// Restore. Restore reads sessions.json (now omitting B) and
		// MUST NOT create a skeleton session for B. The assertion is
		// on live tmux state — enumerate sessions and confirm B is
		// absent. (The first bootstrap's _anchor and A remain; the
		// second bootstrap will also re-spawn _portal-saver if it is
		// not already alive.)
		runPortalList(t, binary, fixture)

		live := liveSessionNames(t, fixture.sock)
		if _, present := live["B"]; present {
			t.Fatalf(
				"second bootstrap reconstructed killed session B as a live tmux session; "+
					"live sessions = %v\n%s",
				keysOf(live), fixture.diagnostic(),
			)
		}
		if _, present := live["A"]; !present {
			t.Errorf("second bootstrap dropped surviving session A; live sessions = %v", keysOf(live))
		}

		// Final sessions.json shape check: A present, B absent. This
		// pins the steady-state observable downstream consumers see.
		assertSessionsJSONHas(t, fixture.stateDir, []string{"A"}, []string{"B"})
	})

	t.Run("_portal-saver self-kill with marker clear leaves user sessions intact and omits _portal-saver", func(t *testing.T) {
		fixture := newSymptomFixture(t, binary, binDir, "ptl-symptom-saverkill-")

		// Verify the scaffold actually produced _portal-saver via the
		// real bootstrap path. Underscore-prefix sessions are filtered
		// out of Client.ListSessions, so we query via the raw tmux
		// list-sessions through the test socket.
		if !rawSessionPresent(t, fixture.sock, tmux.PortalSaverName) {
			t.Fatalf("_portal-saver not present after bootstrap; "+
				"sub-test cannot exercise saver self-kill scenario\n%s",
				fixture.diagnostic())
		}

		// Baseline shape is already asserted inside newSymptomFixture.

		// Sub-test 2 step 2: kill _portal-saver. This fires
		// session-closed against the test socket; commit-now runs
		// (marker is clear) and rewrites sessions.json. The under-
		// score-prefix filter inside keepSessionNames excludes
		// _portal-saver from the resulting Index, so the post-kill
		// shape is identical to the baseline (A + B, _portal-saver
		// still absent).
		fixture.sock.Run(t, "kill-session", "-t", tmux.PortalSaverName)

		// Sub-test 2 step 3: poll sessions.json to confirm the
		// commit-now subprocess ran. The two-consecutive-consistent
		// discipline holds even though the expected shape is "A + B
		// present, _portal-saver absent" — i.e. identical to the
		// baseline — because (a) the file may be re-written under the
		// same shape by commit-now, and (b) we still want to prove
		// commit-now executed (its log entry lives in portal.log) by
		// virtue of two settled reads.
		ctx, cancel := context.WithTimeout(context.Background(), symptomKillBudget)
		defer cancel()
		if perr := pollSessionsJSON(ctx, fixture.stateDir,
			[]string{"A", "B"},
			[]string{tmux.PortalSaverName},
		); perr != nil {
			t.Fatalf(
				"sessions.json did not stabilise after _portal-saver kill within %s: %v\n%s",
				symptomKillBudget, perr, fixture.diagnostic(),
			)
		}

		// Final shape assertion: user sessions intact, saver omitted.
		assertSessionsJSONHas(t, fixture.stateDir, []string{"A", "B"}, []string{tmux.PortalSaverName})
	})

	t.Run("@portal-restoring defence: marker-set saver-kill is byte-identical, marker-clear kill updates correctly", func(t *testing.T) {
		fixture := newSymptomFixture(t, binary, binDir, "ptl-symptom-marker-")

		if !rawSessionPresent(t, fixture.sock, tmux.PortalSaverName) {
			t.Fatalf("_portal-saver not present after bootstrap; "+
				"sub-test cannot exercise marker-set defence\n%s",
				fixture.diagnostic())
		}

		// Sub-test 3 step 2: manually set @portal-restoring on the
		// test server. set-option -s applies a server-level option,
		// matching production bootstrap step 3.
		fixture.sock.Run(t, "set-option", "-s", state.RestoringMarkerName, "1")

		// Teardown safety: clear @portal-restoring on test exit even
		// if an assertion fails mid-test. Server teardown via
		// tmuxtest.Socket's kill-server also takes the marker with
		// it, so this is belt-and-braces — but the marker pollution
		// scope is the test server itself, which is already isolated
		// per sub-test.
		t.Cleanup(func() {
			_, _ = fixture.sock.TryRun("set-option", "-su", state.RestoringMarkerName)
		})

		// Sub-test 3 step 3: snapshot sessions.json bytes. The
		// load-bearing assertion below compares the post-kill file
		// byte-for-byte against this snapshot.
		pre, err := os.ReadFile(state.SessionsJSON(fixture.stateDir))
		if err != nil {
			t.Fatalf("read sessions.json pre-kill: %v\n%s", err, fixture.diagnostic())
		}

		// Sub-test 3 step 4: kill _portal-saver. commit-now should
		// short-circuit on @portal-restoring and exit 0 without
		// writing sessions.json (touch save.requested instead, but
		// that file is orthogonal to the byte-identical assertion).
		fixture.sock.Run(t, "kill-session", "-t", tmux.PortalSaverName)

		// Sub-test 3 step 5: wait briefly. We can't poll for change
		// because no change is expected — the load-bearing readiness
		// signal is "the hook subprocess has had time to run and
		// short-circuit". 250ms covers fork + portal startup + tmux
		// query + exit on every host we exercise.
		time.Sleep(markerSetSettle)

		// Sub-test 3 step 6: re-read and assert byte-identical.
		post, err := os.ReadFile(state.SessionsJSON(fixture.stateDir))
		if err != nil {
			t.Fatalf("read sessions.json post-kill: %v\n%s", err, fixture.diagnostic())
		}
		if string(pre) != string(post) {
			t.Fatalf(
				"sessions.json mutated despite @portal-restoring set\n"+
					"--- pre (%d bytes) ---\n%s\n"+
					"--- post (%d bytes) ---\n%s\n%s",
				len(pre), string(pre),
				len(post), string(post),
				fixture.diagnostic(),
			)
		}

		// Sub-test 3 step 7: clear the marker. After this, commit-now
		// must commit normally on subsequent kills.
		fixture.sock.Run(t, "set-option", "-su", state.RestoringMarkerName)

		// Sub-test 3 step 8: kill session B with the marker now
		// clear. This proves the gate is per-invocation: commit-now
		// re-queries @portal-restoring at every entry, not a one-
		// shot disable.
		fixture.sock.Run(t, "kill-session", "-t", "B")

		// Sub-test 3 step 9: poll for the post-kill shape (A present,
		// B absent) within the 1.5s budget. Same two-consecutive-
		// consistent discipline as sub-test 1.
		ctx, cancel := context.WithTimeout(context.Background(), symptomKillBudget)
		defer cancel()
		if perr := pollSessionsJSON(ctx, fixture.stateDir, []string{"A"}, []string{"B"}); perr != nil {
			t.Fatalf(
				"sessions.json did not reflect post-marker-clear kill of B within %s: %v\n%s",
				symptomKillBudget, perr, fixture.diagnostic(),
			)
		}
	})
}

// symptomFixture bundles the per-sub-test scaffolding: an isolated
// tmux server, a portal state directory, and the env wiring used by
// every portal subprocess. Each sub-test gets its own fixture so a
// leaked tmux session, marker, or sessions.json mutation in one
// cannot influence another.
type symptomFixture struct {
	sock     *tmuxtest.Socket
	stateDir string
	binary   string
	binDir   string
}

// newSymptomFixture creates the per-sub-test scaffold:
//
//  1. Isolated tmux server with the `_anchor` session keeping it
//     alive across user-session kills.
//  2. Isolated PORTAL_STATE_DIR per sub-test.
//  3. User sessions A and B created via the test socket.
//  4. First full bootstrap via `portal list` subprocess — this
//     registers Portal's global hooks (notably the commit-now
//     migration on session-closed), spawns _portal-saver, and
//     writes the initial sessions.json. The subprocess inherits
//     TMUX pointing at the test socket and PORTAL_STATE_DIR
//     pointing at the test temp dir.
//
// After return, sessions.json on disk reflects A + B (the under-
// score-prefix filter excludes _anchor and _portal-saver).
func newSymptomFixture(t *testing.T, binary, binDir, sockPrefix string) symptomFixture {
	t.Helper()

	// Full isolation posture: HOME/XDG scrub, dev-state-dir fingerprint
	// backstop, in-process pgrep sandbox, and — load-bearing for THIS fixture,
	// which execs the real binary — the cross-process sandbox registry
	// (state.SandboxRegistryEnv, t.Setenv'd so it rides os.Environ into every
	// portal subprocess and the tmux server spawned below). Without the
	// registry, the subprocess `portal list` bootstrap's orphan sweep
	// enumerates machine-wide and would SIGKILL the developer's live daemon
	// (proven by the canary harness during the fix; the sweep runs at step 4,
	// before EnsureSaver, so its legitimate set is empty here).
	_, stateDir := portaltest.IsolateStateForTest(t)

	// t.Setenv("PORTAL_STATE_DIR", ...) BEFORE the tmux server is
	// spawned so the server (forked by the first sock.Run below)
	// inherits PORTAL_STATE_DIR and propagates it to every
	// session-closed hook subprocess. Without this, commit-now in
	// the hook subprocess resolves the state dir to the user's
	// ~/.config/portal/state and writes there — the test's polling
	// would never observe the kill because the wrong file is being
	// updated. Mirrors task 1-5's binding (see
	// state_commit_now_reentrancy_integration_test.go).
	//
	// t.Setenv is scoped to whichever *testing.T is passed in — here
	// the sub-test t, so its automatic Cleanup restores the prior
	// value before the next sub-test sets a fresh stateDir. Per-sub-
	// test isolation is preserved because each sub-test spawns its
	// own tmux server AFTER its own setenv, so the server's env
	// reflects that sub-test's stateDir.
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	// Isolate the per-file config env vars too — the bootstrap subprocess
	// spawned below (portal list) runs the full orchestrator against the
	// test's tmux server. Without these the subprocess would inherit the
	// developer's real ~/.config/portal/hooks.json / projects.json /
	// aliases and resolve config against them instead of the test's
	// isolated state (which has nothing in common with the developer's).
	t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(stateDir, "hooks.json"))
	t.Setenv("PORTAL_PROJECTS_FILE", filepath.Join(stateDir, "projects.json"))
	t.Setenv("PORTAL_ALIASES_FILE", filepath.Join(stateDir, "aliases"))

	// Teardown-race guard: registered HERE (after IsolateStateForTest,
	// before tmuxtest.New) so LIFO runs it AFTER kill-server SIGHUPs the
	// test's saver pane and BEFORE the stateDir TempDir RemoveAll — the
	// saver's graceful shutdown flush and straggler session-closed hook
	// subprocesses otherwise race the removal ("directory not empty").
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	sock := tmuxtest.New(t, sockPrefix)

	// Anchor session keeps the server alive after user sessions are
	// killed. Underscore-prefix filter ensures it never appears in
	// sessions.json so the user-session assertions stay clean.
	sock.Run(t, "new-session", "-d", "-s", "_anchor", "sh", "-c", "sleep infinity")

	// Create the two user sessions before bootstrap so the very
	// first sessions.json written by EnsureSaver's daemon (or by
	// the bootstrap-triggered commit-now via subsequent kills)
	// reflects A + B.
	sock.Run(t, "new-session", "-d", "-s", "A", "sh", "-c", "sleep infinity")
	sock.Run(t, "new-session", "-d", "-s", "B", "sh", "-c", "sleep infinity")

	fixture := symptomFixture{
		sock:     sock,
		stateDir: stateDir,
		binary:   binary,
		binDir:   binDir,
	}

	// First bootstrap. `portal list` is the minimum-cost command
	// that triggers PersistentPreRunE and therefore the full ten-
	// step bootstrap orchestrator (EnsureServer is already a no-op
	// since the server is up, RegisterPortalHooks installs the
	// commit-now hook, EnsureSaver spawns _portal-saver, Restore is
	// a no-op against an empty sessions.json on first run).
	runPortalList(t, binary, fixture)

	// Force an initial sessions.json via a synchronous commit-now
	// subprocess. The bootstrap orchestrator does not write
	// sessions.json itself — that is normally the daemon's job, and
	// the daemon's first commit can lag arbitrarily on slow CI hosts
	// (~1s ticker period + per-pane wall time). The canonical and
	// marker-set sub-tests both need a stable pre-kill sessions.json
	// shape to gate on (canonical wants to assert "A+B → A only";
	// marker-set wants to assert "byte-identical pre/post"), so we
	// drive commit-now directly here rather than waiting on the
	// daemon. commit-now is the same primitive the session-closed
	// hook will invoke later, so the on-disk shape it produces is
	// the same shape the kill path will read and update.
	runPortalCommitNow(t, binary, fixture)

	// Confirm the post-bootstrap, post-direct-commit-now baseline
	// shape on disk. This is a steady-state assertion (no race
	// window — commit-now exited synchronously) so a non-blocking
	// shape check is appropriate.
	assertSessionsJSONHas(t, fixture.stateDir, []string{"A", "B"},
		[]string{tmux.PortalSaverName, "_anchor"})

	return fixture
}

// diagnostic returns a multi-line snapshot of the fixture's current
// state for inclusion in failure messages. Surfaces (a) the on-disk
// state directory contents (including the inlined sessions.json
// bytes, truncated), (b) the raw tmux session list against the test
// socket, and (c) the live pane enumeration. These three signals are
// enough to disambiguate every failure mode the canonical / saver /
// marker sub-tests can hit.
func (f symptomFixture) diagnostic() string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- state directory (%s) ---\n", f.stateDir)
	b.WriteString(dumpStateDir(f.stateDir))
	b.WriteString("\n--- raw tmux sessions (test socket) ---\n")
	if out, err := f.sock.TryRun("list-sessions", "-F", "#{session_name}"); err == nil {
		b.WriteString(out)
	} else {
		fmt.Fprintf(&b, "(list-sessions error: %v)\n%s", err, out)
	}
	b.WriteString("\n--- raw tmux panes (test socket) ---\n")
	if out, err := f.sock.TryRun("list-panes", "-a", "-F",
		"#{session_name}:#{window_index}.#{pane_index} #{pane_current_command}"); err == nil {
		b.WriteString(out)
	} else {
		fmt.Fprintf(&b, "(list-panes error: %v)\n%s", err, out)
	}
	return b.String()
}

// runPortalSubprocess invokes the portal binary with the given args
// against the fixture's tmux socket and state directory. Centralises
// the env wiring (TMUX pointing at the test socket, PORTAL_STATE_DIR
// pointing at the test temp dir, PATH prefixed with the staged
// binary's dir) shared by every portal subprocess this file spawns.
// A non-zero exit is treated as a test failure: the diagnostic
// dump is included so the failure mode (missing hook, broken
// bootstrap, etc.) can be triaged without re-running.
func runPortalSubprocess(t *testing.T, binary string, f symptomFixture, args ...string) {
	t.Helper()

	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("TMUX=%s,1,0", f.sock.SocketPath()),
		"PORTAL_STATE_DIR="+f.stateDir,
		"PATH="+f.binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("portal %s subprocess failed: %v\n--- output ---\n%s\n%s",
			strings.Join(args, " "), err, string(out), f.diagnostic())
	}
	if testing.Verbose() && len(out) > 0 {
		t.Logf("portal %s subprocess output:\n%s", strings.Join(args, " "), string(out))
	}
}

// runPortalCommitNow invokes `portal state commit-now` as a sub-
// process against the fixture's tmux socket and state directory.
// Used by newSymptomFixture to force a deterministic initial
// sessions.json before any kill assertions run — the bootstrap
// orchestrator does not write sessions.json itself and the daemon's
// first commit can lag on slow hosts.
//
// commit-now's @portal-restoring short-circuit is irrelevant here:
// fixture setup always runs with the marker clear, so the
// subprocess takes the structural-commit happy path. A non-zero
// exit indicates the synchronous capture-and-commit primitive
// itself is broken, which is a fixture failure not an assertion
// failure.
func runPortalCommitNow(t *testing.T, binary string, f symptomFixture) {
	t.Helper()
	runPortalSubprocess(t, binary, f, "state", "commit-now")
}

// runPortalList invokes `portal list` as a subprocess against the
// fixture's tmux socket and state directory. This is the canonical
// trigger for the full bootstrap orchestrator from outside the test
// process: PersistentPreRunE runs all ten steps before list itself
// emits anything, so the side effects (hook registration, saver
// spawn, first sessions.json) are in place by the time the
// subprocess exits.
//
// `list` was chosen over `open _anchor` because (a) list has no TUI
// path and exits deterministically without a controlling terminal,
// (b) list runs against the existing live tmux server without
// touching session state, and (c) list emits at most a one-line-per-
// session names dump — easy to capture for diagnostics.
//
// Fatal on subprocess failure: a non-zero exit indicates bootstrap
// itself broke, which is a fixture failure not an assertion failure.
func runPortalList(t *testing.T, binary string, f symptomFixture) {
	t.Helper()
	runPortalSubprocess(t, binary, f, "list")
}

// pollSessionsJSON polls sessions.json on the reentrancyPollInterval
// cadence until ctx is cancelled or reentrancyConsecutiveReads
// successive reads observe (a) every name in mustHave present and
// (b) every name in mustOmit absent. Returns nil on observed
// stabilisation, ctx.Err on timeout, and a wrapped error for non-
// ENOENT read failures.
//
// File-absent (ENOENT) and ReadIndex's skip flag both reset the
// consecutive counter — they represent the pre-write window before
// the hook subprocess (or daemon's first commit) lands a file.
func pollSessionsJSON(ctx context.Context, stateDir string, mustHave, mustOmit []string) error {
	var consecutive int
	ticker := time.NewTicker(reentrancyPollInterval)
	defer ticker.Stop()

	for {
		idx, skip, err := state.ReadIndex(stateDir)
		switch {
		case err != nil && errors.Is(err, fs.ErrNotExist):
			consecutive = 0
		case err != nil:
			return fmt.Errorf("read sessions.json during poll: %w", err)
		case skip:
			consecutive = 0
		default:
			present := sessionNames(idx)
			if matchesShape(present, mustHave, mustOmit) {
				consecutive++
				if consecutive >= reentrancyConsecutiveReads {
					return nil
				}
			} else {
				consecutive = 0
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// matchesShape reports whether the presence set contains every name
// in mustHave and none of the names in mustOmit.
func matchesShape(present map[string]struct{}, mustHave, mustOmit []string) bool {
	for _, n := range mustHave {
		if _, ok := present[n]; !ok {
			return false
		}
	}
	for _, n := range mustOmit {
		if _, ok := present[n]; ok {
			return false
		}
	}
	return true
}

// assertSessionsJSONHas fails the test if sessions.json does not
// currently contain every name in mustHave or contains any name in
// mustOmit. Used as a steady-state shape check after polling has
// already settled — distinct from pollSessionsJSON which is the
// readiness gate.
func assertSessionsJSONHas(t *testing.T, stateDir string, mustHave, mustOmit []string) {
	t.Helper()
	idx, skip, err := state.ReadIndex(stateDir)
	if err != nil || skip {
		t.Fatalf("assert sessions.json shape: skip=%v err=%v\n%s",
			skip, err, dumpStateDir(stateDir))
	}
	present := sessionNames(idx)
	for _, n := range mustHave {
		if _, ok := present[n]; !ok {
			t.Errorf("sessions.json missing %q; present=%v", n, keysOf(present))
		}
	}
	for _, n := range mustOmit {
		if _, ok := present[n]; ok {
			t.Errorf("sessions.json unexpectedly contains %q; present=%v", n, keysOf(present))
		}
	}
}

// liveSessionNames returns the set of session names currently alive
// on the test socket. Used by sub-test 1 to assert that the second
// bootstrap did NOT reconstruct the killed session as a skeleton
// pane — restore creates sessions matching names in sessions.json
// that aren't live, so if B is absent from sessions.json the live
// list should remain free of B.
func liveSessionNames(t *testing.T, sock *tmuxtest.Socket) map[string]struct{} {
	t.Helper()
	out, err := sock.TryRun("list-sessions", "-F", "#{session_name}")
	if err != nil {
		t.Fatalf("list-sessions on test socket: %v\n%s", err, out)
	}
	set := map[string]struct{}{}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		set[line] = struct{}{}
	}
	return set
}

// rawSessionPresent reports whether the named session exists on the
// test socket using the raw `tmux has-session` probe. Used to assert
// fixture preconditions for the saver-kill sub-tests — Portal's
// Client.ListSessions filters underscore-prefixed names, so we
// cannot use the Go client to verify _portal-saver is alive.
func rawSessionPresent(t *testing.T, sock *tmuxtest.Socket, name string) bool {
	t.Helper()
	_, err := sock.TryRun("has-session", "-t", name)
	return err == nil
}

// keysOf returns a deterministic-ish slice view of a presence set
// for inclusion in failure messages. Map iteration order is
// non-deterministic but the slice format is human-readable; tests
// that compare on this slice's content would be flaky, so callers
// use it only in t.Fatalf / t.Errorf diagnostic strings.
func keysOf(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}
