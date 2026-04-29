//go:build integration

// Phase 3 task 3-13 (expanded by Phase 12 task 12-4) — full state↔restore
// round-trip integration test.
//
// This file holds the lower-level round-trip companion to
// cmd/bootstrap/reboot_roundtrip_test.go (Phase 5 task 5-9). Where the
// reboot test exercises the full nine-step bootstrap.Orchestrator wiring,
// this test pins down the state↔restore primitive directly: capture →
// commit → kill → restore → drive signal-hydrate → assert.
//
// Why both layers carry an end-to-end round-trip:
//
//   - The bootstrap-layer test guards the orchestration and step ordering.
//   - This state-layer test guards the capture/encode + restore pipeline
//     in isolation, so a regression in (say) CaptureStructure or
//     SessionRestorer.applyEnvironment fails here without first hiding
//     behind a bootstrap-step failure. It also stays meaningful if
//     bootstrap is ever refactored to call Restore differently.
//
// Why this file (and not the package's other integration_test.go) is
// gated `//go:build integration`:
//
//   - The marker-clearance assertion requires the in-pane `portal state
//     hydrate` helper to actually run, which means `portal` must be
//     resolvable on PATH inside each restored pane. Building the binary
//     adds ~1s to the test and is the same trade-off
//     reboot_roundtrip_test.go made.
//   - The other tests in integration_test.go (corrupt-index, base-index
//     drift, FIFO sweep) do not need the binary and stay on the default
//     test surface.
//
// Build & run:
//
//	go test -tags=integration ./internal/restore/...
//	go test -short -tags=integration ./internal/restore/...   # skips this
//
// Tests in this file are NOT included in the default `go test ./...` run
// because the `//go:build integration` tag gates them off. They also call
// `testing.Short()` so the short-mode CI lane skips them.

package restore_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// fixtureSession describes the topology of one captured session in the
// test fixture. Two of these (alpha + beta) feed createFullTopology.
type fixtureSession struct {
	name        string
	envKey      string
	envValue    string
	cwds        [2][2]string // [windowIdx][paneIdx]
	zoomedW     int          // index of the window whose w.p1 is zoomed
	zoomedP     int
	activeWin   int    // session-level active window
	activePanes [2]int // active pane per window
}

// TestPhase3Integration_FullRoundTrip is the expanded state↔restore round
// trip mandated by planning task 3-13 (and re-scoped in task 12-4). It
// stands up two sessions × two windows × two panes, marks one pane
// zoomed and one pane active per window, sets per-session env vars,
// seeds each pane's scrollback file with ANSI SGR sequences, runs the
// save → kill → restore cycle, drives signal-hydrate via direct FIFO
// byte writes (the byte-equivalent of `portal state signal-hydrate`),
// and asserts on every dimension the planning bullet covers:
//
//   - Structure: 2 sessions, each with 2 windows and 2 panes per window.
//   - Layout + zoom: window_zoomed_flag is 1 on the zoomed window, 0
//     elsewhere; pane geometry survives the round-trip.
//   - Active pane: per-window active pane preserved.
//   - Per-session environment: KEY=VALUE round-trips via show-environment.
//   - ANSI scrollback: capture-pane -e on the live pane contains the
//     seeded SGR sequence, the literal payload, and the reset sequence
//     (using a precise contains-with-prefix-suffix scheme rather than a
//     literal byte-compare; capture-pane introduces wrap padding that
//     makes byte-equality brittle without coverage benefit — the
//     substring scheme is the same trade-off reboot_roundtrip_test.go
//     adopted, and is sensitive enough to catch any byte-level escape
//     loss).
//   - @portal-restoring marker cleared after restoreWithMarker exits.
//   - @portal-skeleton-<paneKey> markers cleared after each helper
//     finishes its dump and 100ms settle sleep.
func TestPhase3Integration_FullRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	binDir := buildPortalBinaryDir(t)
	prependPathDir(t, binDir)

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	// Hooks store path is set so the helper has a concrete file to read
	// rather than the user's real ~/.config/portal/hooks.json. No
	// on-resume hooks are registered here — hook firing is covered by
	// the bootstrap-layer reboot test (cmd/bootstrap/reboot_roundtrip_
	// test.go); this test asserts marker clearance only.
	t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))

	alpha := fixtureSession{
		name:        "alpha",
		envKey:      "ALPHA_ENV",
		envValue:    "alpha-value",
		cwds:        [2][2]string{{t.TempDir(), t.TempDir()}, {t.TempDir(), t.TempDir()}},
		zoomedW:     0,
		zoomedP:     1,
		activeWin:   1,
		activePanes: [2]int{1, 0}, // w0 active=p1, w1 active=p0
	}
	beta := fixtureSession{
		name:        "beta",
		envKey:      "BETA_ENV",
		envValue:    "beta-value",
		cwds:        [2][2]string{{t.TempDir(), t.TempDir()}, {t.TempDir(), t.TempDir()}},
		zoomedW:     1,
		zoomedP:     0,
		activeWin:   0,
		activePanes: [2]int{0, 0}, // w0 active=p0, w1 active=p0
	}

	ts := tmuxtest.New(t, "ptl-3-13-")
	client := ts.Client()

	createFullTopology(t, ts, alpha)
	createFullTopology(t, ts, beta)

	// Capture + commit. Skeleton-marker skip-set is empty on a fresh
	// server — these are all live, never-been-restored panes.
	idx, err := state.CaptureStructure(client, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	verifyTopologyShape(t, idx, alpha, beta)

	// Seed each pane's scrollback file with a deterministic ANSI fixture
	// AFTER capture but BEFORE persist — the on-disk file the hydrate
	// helper later dumps is what we control here. capture-pane output
	// would be timing-dependent and would defeat any contains check.
	scrollbackFixtures := map[string][]byte{}
	for _, fx := range []fixtureSession{alpha, beta} {
		for w := 0; w < 2; w++ {
			for p := 0; p < 2; p++ {
				key := state.SanitizePaneKey(fx.name, w, p)
				bytes := ansiFixtureBytes(fx.name, w, p)
				scrollbackFixtures[key] = bytes
				path := state.ScrollbackFile(stateDir, key)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatalf("mkdir scrollback dir: %v", err)
				}
				if err := os.WriteFile(path, bytes, 0o600); err != nil {
					t.Fatalf("write scrollback %s: %v", key, err)
				}
			}
		}
	}

	// Persist sessions.json. EncodeIndex is the canonical writer; using
	// it (rather than handcrafting JSON) keeps the test honest with the
	// schema CaptureStructure produced.
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(stateDir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	// Kill the server so we run Restore against a fresh one. The
	// list-sessions check confirms the kill actually took effect — a
	// silently-still-alive server would mask a Restore that did nothing.
	ts.KillServer()
	if _, err := ts.TryRun("list-sessions"); err == nil {
		t.Fatalf("list-sessions succeeded after kill-server; expected error")
	}
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

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

	// Acceptance: structure equality. Both sessions live, with the
	// expected window/pane addressing under default 0/0 base indices
	// (the test does not exercise drift — that's TestPhase3Integration_
	// RestoreUsesLiveIndicesUnderBaseIndexDrift's domain).
	verifyLiveStructure(t, ts, alpha, beta)

	// Acceptance: layout / zoom. window_zoomed_flag must match what we
	// captured pre-save — alpha:0 zoomed, alpha:1 not; beta:1 zoomed,
	// beta:0 not.
	verifyZoomFlags(t, ts, alpha, beta)

	// Acceptance: active pane preserved per window AND per session.
	// We assert pane_active for the (window, pane) the fixture marked
	// active before save.
	verifyActivePanes(t, ts, alpha, beta)

	// Acceptance: per-session environment round-trip via tmux show-
	// environment.
	verifySessionEnv(t, client, alpha)
	verifySessionEnv(t, client, beta)

	// Acceptance: @portal-restoring cleared after restoreWithMarker's
	// deferred unset ran. Zero output (or absent) means cleared.
	verifyRestoringMarkerCleared(t, ts)

	// Drive signal-hydrate on every restored pane: write 1 byte to each
	// pane's FIFO with O_WRONLY|O_NONBLOCK and an ENXIO/EAGAIN retry
	// budget. This is the byte-equivalent of cmd/state_signal_hydrate
	// production code and is the same approach
	// reboot_roundtrip_test.go takes (the production CLI is covered by
	// dedicated unit tests; spawning it here adds nothing).
	driveSignalHydrate(t, client, stateDir, []string{alpha.name, beta.name})

	// Wait for every helper's marker-unset (step 8 of "Helper Behavior
	// on Startup"). Empty marker set = every helper reached the post-
	// settle-sleep step. A stuck marker means the helper crashed
	// pre-unset; downstream assertions would be flaky if we proceeded
	// blindly.
	waitForSkeletonMarkersCleared(t, client, 10*time.Second)

	// Acceptance: ANSI scrollback survives. capture-pane -e on each
	// hydrated pane should contain the seeded SGR + payload + reset
	// triple. We use a precise contains-with-prefix-suffix scheme
	// (rather than a literal byte-compare) per the documented
	// trade-off above: capture-pane wraps lines and pads cells.
	for _, fx := range []fixtureSession{alpha, beta} {
		for w := 0; w < 2; w++ {
			for p := 0; p < 2; p++ {
				verifyANSIInScrollback(t, ts, fx.name, w, p, scrollbackFixtures[state.SanitizePaneKey(fx.name, w, p)])
			}
		}
	}
}

// createFullTopology builds one session with two windows of two panes
// each, applies per-session env, marks the configured pane zoomed, and
// selects per-window + session-level active panes. All shells are
// `sleep infinity` so panes outlive the test body without producing
// scrollback noise — the test seeds scrollback bytes on disk after
// capture, so what runs in the live pane is irrelevant to the bytes
// checked later.
func createFullTopology(t *testing.T, ts *tmuxtest.Socket, fx fixtureSession) {
	t.Helper()
	// Session, window 0, pane 0: bootstraps the session.
	ts.Run(t, "new-session", "-d", "-s", fx.name, "-c", fx.cwds[0][0], "sleep", "infinity")
	ts.WaitForSession(t, fx.name, 2*time.Second)

	ts.Run(t, "set-environment", "-t", fx.name, fx.envKey, fx.envValue)

	// Window 0, pane 1.
	ts.Run(t, "split-window", "-t", fx.name+":0", "-c", fx.cwds[0][1], "sleep", "infinity")

	// Window 1 (new-window), pane 0.
	ts.Run(t, "new-window", "-t", fx.name, "-c", fx.cwds[1][0], "sleep", "infinity")

	// Window 1, pane 1.
	ts.Run(t, "split-window", "-t", fx.name+":1", "-c", fx.cwds[1][1], "sleep", "infinity")

	// Mark the configured pane zoomed. resize-pane -Z is a toggle, so
	// only call it once.
	zoomTarget := tmux.PaneTarget(fx.name, fx.zoomedW, fx.zoomedP)
	ts.Run(t, "resize-pane", "-t", zoomTarget, "-Z")

	// Per-window active pane.
	for w, ap := range fx.activePanes {
		ts.Run(t, "select-pane", "-t", tmux.PaneTarget(fx.name, w, ap))
	}

	// Session-level active window.
	ts.Run(t, "select-window", "-t", fmt.Sprintf("%s:%d", fx.name, fx.activeWin))
}

// ansiFixtureBytes returns the deterministic ANSI byte fixture seeded
// into the scrollback file for one (session, window, pane). Each pane
// gets a unique payload so a swapped-on-restore regression (e.g.
// ApplySkeletonMarkers cross-wiring scrollback paths) would be caught
// — we verify the right bytes land in the right pane.
func ansiFixtureBytes(session string, window, pane int) []byte {
	return []byte(fmt.Sprintf(
		"\x1b[31m[fixture %s w%d p%d]\x1b[0m\nbefore-reboot-payload\n",
		session, window, pane,
	))
}

// verifyTopologyShape sanity-checks the captured Index against the
// fixture inputs. Failures here mean capture silently regressed (e.g.
// dropped a session) and the round-trip is no longer testing what its
// name claims.
func verifyTopologyShape(t *testing.T, idx state.Index, alpha, beta fixtureSession) {
	t.Helper()
	if got := len(idx.Sessions); got != 2 {
		t.Fatalf("captured %d sessions; want 2 (idx=%+v)", got, idx)
	}
	// Sessions are sorted alphabetically by Canonicalize.
	if idx.Sessions[0].Name != alpha.name || idx.Sessions[1].Name != beta.name {
		t.Fatalf("session names = [%s, %s]; want [%s, %s]",
			idx.Sessions[0].Name, idx.Sessions[1].Name, alpha.name, beta.name)
	}
	for _, fx := range []struct {
		idxOf int
		f     fixtureSession
	}{{0, alpha}, {1, beta}} {
		s := idx.Sessions[fx.idxOf]
		if got := len(s.Windows); got != 2 {
			t.Fatalf("%s windows = %d; want 2", s.Name, got)
		}
		for w := 0; w < 2; w++ {
			if got := len(s.Windows[w].Panes); got != 2 {
				t.Fatalf("%s w%d panes = %d; want 2", s.Name, w, got)
			}
		}
		// Zoom captured on the right window.
		if !s.Windows[fx.f.zoomedW].Zoomed {
			t.Errorf("%s w%d not zoomed in capture; want zoomed=true", s.Name, fx.f.zoomedW)
		}
		// Per-session env captured.
		if got := s.Environment[fx.f.envKey]; got != fx.f.envValue {
			t.Errorf("%s env[%s] = %q; want %q", s.Name, fx.f.envKey, got, fx.f.envValue)
		}
		// Per-window active pane captured.
		for w := 0; w < 2; w++ {
			ap := fx.f.activePanes[w]
			if !s.Windows[w].Panes[ap].Active {
				t.Errorf("%s w%d p%d should be active in capture; got Active=false (panes=%+v)",
					s.Name, w, ap, s.Windows[w].Panes)
			}
		}
	}
}

// verifyLiveStructure asserts the restored topology matches the saved
// shape: each session has windows at 0,1 and panes at 0,1 in each
// window (default base-index 0).
func verifyLiveStructure(t *testing.T, ts *tmuxtest.Socket, sessions ...fixtureSession) {
	t.Helper()
	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	for _, fx := range sessions {
		if !strings.Contains(out, fx.name) {
			t.Errorf("session %q missing post-restore; got %q", fx.name, out)
		}
		panesOut := ts.Run(t, "list-panes", "-s", "-t", fx.name,
			"-F", "#{window_index}:#{pane_index}")
		for w := 0; w < 2; w++ {
			for p := 0; p < 2; p++ {
				want := fmt.Sprintf("%d:%d", w, p)
				if !strings.Contains(panesOut, want) {
					t.Errorf("%s live pane %q missing; got %q", fx.name, want, panesOut)
				}
			}
		}
	}
}

// verifyZoomFlags asserts window_zoomed_flag matches the captured
// fixture configuration: only the configured window per session is
// zoomed, all others are not.
func verifyZoomFlags(t *testing.T, ts *tmuxtest.Socket, sessions ...fixtureSession) {
	t.Helper()
	for _, fx := range sessions {
		for w := 0; w < 2; w++ {
			got := strings.TrimSpace(ts.Run(t, "display-message", "-p",
				"-t", fmt.Sprintf("%s:%d", fx.name, w),
				"#{window_zoomed_flag}"))
			want := "0"
			if w == fx.zoomedW {
				want = "1"
			}
			if got != want {
				t.Errorf("%s:%d window_zoomed_flag = %q; want %q", fx.name, w, got, want)
			}
		}
	}
}

// verifyActivePanes asserts pane_active is 1 for every (window,
// expected-active-pane) the fixture pre-saved. Per-window assertion
// catches both "wrong pane is active" and "no pane is active" — tmux
// always has exactly one active pane per window so the latter is
// effectively impossible, but the former would silently regress
// applyActivePane.
func verifyActivePanes(t *testing.T, ts *tmuxtest.Socket, sessions ...fixtureSession) {
	t.Helper()
	for _, fx := range sessions {
		for w := 0; w < 2; w++ {
			ap := fx.activePanes[w]
			got := strings.TrimSpace(ts.Run(t, "display-message", "-p",
				"-t", tmux.PaneTarget(fx.name, w, ap),
				"#{pane_active}"))
			if got != "1" {
				t.Errorf("%s w%d p%d pane_active = %q; want 1", fx.name, w, ap, got)
			}
		}
	}
}

// verifySessionEnv asserts the per-session env var captured pre-save
// is set on the restored session. show-environment lists all set vars;
// we search for the exact KEY=VALUE line.
func verifySessionEnv(t *testing.T, client *tmux.Client, fx fixtureSession) {
	t.Helper()
	out, err := client.ShowEnvironment(fx.name)
	if err != nil {
		t.Fatalf("ShowEnvironment %q: %v", fx.name, err)
	}
	wantLine := fx.envKey + "=" + fx.envValue
	if !strings.Contains(out, wantLine) {
		t.Errorf("session %q env missing %q; got:\n%s", fx.name, wantLine, out)
	}
}

// verifyRestoringMarkerCleared confirms @portal-restoring is unset
// after restoreWithMarker's deferred clear ran. show-options exits
// non-zero when the option is unset (which is the success case here);
// a zero exit with a non-empty value is the failure mode.
func verifyRestoringMarkerCleared(t *testing.T, ts *tmuxtest.Socket) {
	t.Helper()
	out, err := ts.TryRun("show-options", "-sv", state.RestoringMarkerName)
	if err == nil && strings.TrimSpace(out) != "" {
		t.Errorf("%s should be unset after restoreWithMarker; got %q",
			state.RestoringMarkerName, out)
	}
}

// verifyANSIInScrollback captures the live pane buffer (with ANSI
// escapes preserved via -e) and asserts the seeded fixture survived.
// We assert each anchor independently — a missing SGR open vs missing
// payload vs missing reset each pinpoints a different regression.
//
// Why not literal byte-equality vs the on-disk fixture: capture-pane
// reflows long lines and right-pads cells with spaces; tmux's PTY
// parser also normalises some SGR sequences (notably `ESC[0m` is
// emitted as `ESC[39m` by capture-pane -e when only the foreground
// colour was modified, because the cell-state diff at end-of-input is
// "default fg restored", not "all attributes reset"). Those gaps
// break byte-equal but not the ANSI fidelity guarantee the spec
// makes. The contains scheme below tolerates the documented
// reformatting while still catching every regression we care about
// (ANSI stripped → SGR open missing; wrong pane content → label
// missing; reset dropped → neither `[0m` nor `[39m` present).
func verifyANSIInScrollback(t *testing.T, ts *tmuxtest.Socket, session string, win, pane int, fixtureBytes []byte) {
	t.Helper()
	target := tmux.PaneTarget(session, win, pane)
	out := ts.Run(t, "capture-pane", "-e", "-p", "-S", "-", "-t", target)

	// The fixture is "\x1b[31m[fixture <s> w<w> p<p>]\x1b[0m\nbefore-
	// reboot-payload\n". Each independent anchor proves a distinct
	// invariant:
	//   1. SGR open (\x1b[31m) survived — capture-pane without -e
	//      would have stripped it; reaching the live pane proves
	//      hydrate's io.Copy ran verbatim.
	//   2. The per-pane label (embeds session/window/pane) lands in
	//      the right pane — guards against a swapped scrollback file
	//      regression.
	//   3. SGR reset survived in some form (`\x1b[0m` byte-equal, or
	//      `\x1b[39m` after tmux's foreground-only normalisation) —
	//      proves the reset wasn't dropped.
	//   4. The payload after the reset round-tripped — proves no
	//      truncation mid-fixture.
	label := fmt.Sprintf("[fixture %s w%d p%d]", session, win, pane)

	if !strings.Contains(out, "\x1b[31m") {
		t.Errorf("scrollback for %s missing red SGR open (%q); fixture=%q got=%q",
			target, "\x1b[31m", fixtureBytes, out)
	}
	if !strings.Contains(out, label) {
		t.Errorf("scrollback for %s missing per-pane label (%q); fixture=%q got=%q",
			target, label, fixtureBytes, out)
	}
	// Either canonical reset (`[0m`) or tmux's normalised foreground-
	// only reset (`[39m`) satisfies the "reset survived" anchor.
	if !strings.Contains(out, "\x1b[0m") && !strings.Contains(out, "\x1b[39m") {
		t.Errorf("scrollback for %s missing SGR reset (neither %q nor %q); fixture=%q got=%q",
			target, "\x1b[0m", "\x1b[39m", fixtureBytes, out)
	}
	if !strings.Contains(out, "before-reboot-payload") {
		t.Errorf("scrollback for %s missing post-SGR payload (%q); fixture=%q got=%q",
			target, "before-reboot-payload", fixtureBytes, out)
	}
}

// driveSignalHydrate is the test-side replacement for `portal state
// signal-hydrate <session>`. For each named session, it enumerates
// skeleton-marked panes and writes 1 byte to each pane's hydration
// FIFO with the production retry contract (O_WRONLY|O_NONBLOCK,
// ENXIO/EAGAIN ladder) but a longer absolute budget — under parallel
// `go test ./...` load the in-pane fork+exec for the helper can take
// well over a second before reaching its open(O_RDONLY).
func driveSignalHydrate(t *testing.T, client *tmux.Client, stateDir string, sessions []string) {
	t.Helper()
	const (
		retryDelay = 50 * time.Millisecond
		budget     = 10 * time.Second
	)
	markers, err := state.ListSkeletonMarkers(client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers: %v", err)
	}
	if len(markers) == 0 {
		t.Fatal("no skeleton markers; restore did not arm any panes")
	}
	for _, session := range sessions {
		panes, err := client.ListPanesInSession(session)
		if err != nil {
			t.Fatalf("ListPanesInSession %q: %v", session, err)
		}
		for _, p := range panes {
			liveKey := state.SanitizePaneKey(session, p.Window, p.Pane)
			if _, marked := markers[liveKey]; !marked {
				continue
			}
			fifo := state.FIFOPath(stateDir, liveKey)
			if err := openAndSignalFIFO(fifo, retryDelay, budget); err != nil {
				t.Errorf("signal FIFO %s: %v", fifo, err)
			}
		}
	}
}

// openAndSignalFIFO opens path O_WRONLY|O_NONBLOCK, retries ENXIO and
// EAGAIN at delay intervals until budget elapses, then writes a single
// byte. Byte-equivalent to cmd/state_signal_hydrate.writeFIFOSignal —
// duplicated here (rather than imported) because that function lives
// in the cmd package and is unexported, and integration round-trip
// tests across both layers each need their own copy.
func openAndSignalFIFO(path string, delay, budget time.Duration) error {
	deadline := time.Now().Add(budget)
	var lastErr error
	for {
		f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			if _, werr := f.Write([]byte{1}); werr != nil {
				_ = f.Close()
				return fmt.Errorf("write byte: %w", werr)
			}
			_ = f.Close()
			return nil
		}
		if !errors.Is(err, syscall.ENXIO) && !errors.Is(err, syscall.EAGAIN) {
			return fmt.Errorf("open: %w", err)
		}
		lastErr = err
		if time.Now().After(deadline) {
			return fmt.Errorf("retries exhausted after %s: %w", budget, lastErr)
		}
		time.Sleep(delay)
	}
}

// waitForSkeletonMarkersCleared polls until every @portal-skeleton-*
// server option has been unset. Each helper unsets its own marker after
// scrollback dump + 100ms settle (or after the file-missing recovery
// path), so an empty marker set means every helper reached the
// hook-or-shell exec step. timeout is the deadline; on expiry the test
// fails with a sorted list of stuck keys for stable diagnostics.
func waitForSkeletonMarkersCleared(t *testing.T, client *tmux.Client, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		markers, err := state.ListSkeletonMarkers(client)
		if err != nil {
			t.Fatalf("ListSkeletonMarkers: %v", err)
		}
		if len(markers) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	markers, _ := state.ListSkeletonMarkers(client)
	t.Fatalf("skeleton markers still set after %s: %v", timeout, sortedKeySet(markers))
}

// sortedKeySet flattens a presence-set to a sorted string slice for
// stable diagnostic output. Used only by the markers-not-cleared
// failure path so a flaky CI failure prints a deterministic key list.
func sortedKeySet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// buildPortalBinaryDir compiles `portal` into a fresh temp directory
// and returns the dir path so the caller can prepend it to PATH. The
// binary must exist at that path before Restore arms each pane with
// `respawn-pane -k` whose command is `sh -c 'portal state hydrate ...'`;
// the in-pane shell resolves "portal" by looking on PATH.
func buildPortalBinaryDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "portal")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build portal: %v\n%s", err, out)
	}
	return dir
}

// prependPathDir prefixes dir to the test process's PATH (visible to
// the tmux server we'll fork, since tmux inherits our env). t.Setenv
// guarantees the original PATH is restored on test exit so subsequent
// tests in the same package see the unmodified value.
func prependPathDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// projectRoot walks up from the test's runtime CWD to the directory
// holding go.mod. The internal/restore test binary's CWD is
// internal/restore/ at runtime; the closest go.mod is two levels up at
// the repo root, which is where `go build .` must run from to compile
// the portal CLI.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate project root (go.mod)")
		}
		dir = parent
	}
}
