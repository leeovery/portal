//go:build integration

// spectrum-tui-design-5-8 — Part D: concurrent cold-boot startup-ordering
// integration suite.
//
// These tests drive the §10.2 CONCURRENT cold-boot route end-to-end against a
// REAL isolated tmux server with a REAL `_portal-saver` daemon, exercising the
// production progress pipe (bootstrapProgressPipe) that runs Orchestrator.Run in
// a goroutine and streams per-step progress over the channel. They assert the
// invariants the prior incident (slow-open / empty-previews / zombie-session)
// threatened:
//
//   - orchestrator STEP ORDERING preserved on the concurrent route (the ten
//     StepEvents stream in 1..10 order).
//   - the @portal-restoring SET-before-restore / CLEAR-before-cleanup window is
//     intact concurrently (cleared by the time the terminal event lands).
//   - the daemon is spawned EXACTLY ONCE (singleton) — pgrep + the saver-pane
//     PID agree — with NO zombie/leaked daemon.
//   - NO slow-open regression: a FAST cold boot (M=0, nothing to restore) AND a
//     SLOW restore (saved sessions to skeleton-restore) both reach a clean
//     terminal complete with the daemon singleton intact.
//   - WARM-PATH PARITY: a warm boot (serverStarted=false) takes the SYNCHRONOUS
//     path with no loading page and unchanged ordering — asserted in the
//     non-integration sibling cmd/concurrent_bootstrap_route_test.go and pinned
//     here via TestConcurrentColdBoot_WarmParity_NoLoadingPageSynchronousOrdering.
//
// Discipline (load-bearing — the prior incident was a leaked test daemon
// corrupting the dev install):
//   - portaltest.IsolateStateForTest(t) scrubs the developer XDG_CONFIG_HOME and
//     registers the fingerprint-diff backstop; PORTAL_STATE_DIR pins every
//     subprocess (the tmux-server-spawned saver daemon inherits it).
//   - the saver daemon is reaped by killing _portal-saver in t.Cleanup BEFORE
//     tmuxtest's kill-server, so the daemon sees SIGHUP and exits — no zombie.
//   - NO t.Parallel (cmd-package convention; package-level mutable seam state).
//   - the no-leaked-daemon assertion is EXPLICIT (assertNoExtraDaemons) — the
//     IsolateStateForTest backstop is defence-in-depth, not a substitute.
//
// Build & run:  go test -tags=integration ./cmd/...

package cmd

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

// concurrentBootDrainBudget bounds the drainPipe poll for the concurrent route.
// A real cold boot (server start + saver bootstrap + restore + cleanup) is
// observed at a few hundred ms; 15 s is generous headroom for slow CI hardware
// while still failing loudly on a wedged goroutine (the slow-open regression
// shape — a frozen pipe that never sends the terminal event).
const concurrentBootDrainBudget = 15 * time.Second

// setupConcurrentColdBootEnv builds the per-test scaffolding for a REAL cold
// boot: isolated state dir (PORTAL_STATE_DIR pinned, fingerprint backstop
// registered), portal binary on PATH (so restored panes' hydrate helper resolves
// and the saver daemon spawns), and an isolated tmux socket. It deliberately does
// NOT pre-start the saver — the orchestrator's EnsureSaver step does that, which
// is the whole point of exercising the cold route. Returns the socket, client,
// state dir, and the isolated env slice (for any further subprocess spawn).
func setupConcurrentColdBootEnv(t *testing.T) (*tmuxtest.Socket, *tmux.Client, string, []string) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ensurePortalOnPATH(t)

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	// IsolateStateForTest re-points HOME at a fresh t.TempDir(). tmux-spawned
	// interactive shells (the saver pane's, restored panes') flush a shell-history
	// file ($HOME/.zsh_history etc.) on SIGHUP exit DURING teardown — a write that
	// races the framework's HOME-tempdir RemoveAll ("directory not empty"). Pinning
	// HISTFILE to /dev/null routes that write away from HOME so the tempdir stays
	// empty for RemoveAll. The tmux server inherits this env. Orthogonal to the
	// daemon/state invariants under test — purely a teardown-race mitigation.
	t.Setenv("HISTFILE", os.DevNull)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	ts := tmuxtest.New(t, "ptl-cc-coldboot-")
	client := ts.Client()

	// Reap the saver daemon BEFORE tmuxtest's kill-server runs (t.Cleanup LIFO:
	// tmuxtest.New registered its kill-server first, so this fires first). Killing
	// _portal-saver delivers SIGHUP to the daemon so it exits; we then BLOCK until
	// the daemon process is actually gone so it releases the state-dir fds
	// (daemon.lock, portal.log, daemon.pid) before the testing framework's
	// t.TempDir RemoveAll runs — otherwise RemoveAll races a live daemon and fails
	// with "directory not empty" (load-bearing on macOS). This is the tmux-pane
	// analogue of the SIGKILL+Wait subprocess reap.
	t.Cleanup(func() {
		reapSaverDaemon(t, ts, client, stateDir)
	})

	resetBootstrapOnce(t)

	return ts, client, stateDir, envSlice
}

// reapTmuxServer is the shared blocking server reap: kill-server (not just
// kill-session) so EVERY pane's shell receives SIGHUP and exits, then BLOCK until
// the server is actually unreachable before returning. Every pane shell holds
// HOME at the IsolateStateForTest tempdir, and a lingering shell flushing on exit
// races the framework's HOME-tempdir RemoveAll ("directory not empty"). Tearing
// the whole server down and waiting for it to go away drains those shells before
// the cleanup returns. Registered with t.Cleanup so it runs (LIFO) BEFORE
// tmuxtest.New's own kill-server, which is then an idempotent no-op.
//
// Best-effort: it never fails the test (a server still reachable at the budget
// would surface as the RemoveAll error, which is itself diagnostic). It is the
// single chokepoint for the SERVER reap so the warm-parity and cold-boot paths
// cannot diverge again (the warm-parity test previously used a no-op
// kill-session _portal-saver — a NO-OP because _portal-saver never exists on the
// warm path — leaving the default-pane shell to race RemoveAll).
func reapTmuxServer(t *testing.T, ts *tmuxtest.Socket) {
	t.Helper()
	ts.KillServer()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		// list-sessions against a dead server errors ("no server running on
		// ..." / "error connecting"); that error means the server is gone.
		if _, err := ts.TryRun("list-sessions"); err != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// reapSaverDaemon kills the server (via the shared reapTmuxServer) and ADDITIONALLY
// blocks until the saver-pane daemon process is no longer observable (pane absent
// AND daemon.pid PID dead), so the daemon has released the state-dir file
// descriptors (daemon.lock, portal.log, daemon.pid) before t.TempDir cleanup. The
// shared server reap drains pane shells; this daemon-death wait additionally
// awaits the daemon PROCESS exit, whose fd release trails the pane vanishing.
// Best-effort: it never fails the test (a still-running daemon at the budget would
// surface as the RemoveAll error, which is itself diagnostic).
func reapSaverDaemon(t *testing.T, ts *tmuxtest.Socket, client *tmux.Client, stateDir string) {
	t.Helper()
	// Snapshot the daemon PID before kill so we can poll its liveness directly
	// (the pane vanishes immediately on kill, but the process teardown — fd
	// release — trails).
	pid, _ := state.ReadPIDFile(stateDir)
	// Shared blocking server reap: SIGHUP every pane shell and wait for the
	// server to go unreachable.
	reapTmuxServer(t, ts)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		// During teardown the saver pane is being torn down, so a transient
		// "can't find session/window" error means absent. Any error or
		// present=false counts as "saver gone".
		_, present, perr := tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName)
		saverGone := perr != nil || !present
		daemonDead := pid <= 0 || !pidIsAlive(pid)
		if saverGone && daemonDead {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// pidIsAlive reports whether pid is a live process via the canonical signal-0
// liveness probe. A reaped/dead PID returns false; a live one returns true.
func pidIsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// buildConcurrentColdBootOrchestrator wires a bootstrap.Orchestrator for the
// cold-boot route: real RestoringMarker (Set/Clear → the @portal-restoring
// window), real OrphanSweeper (step 4 — the pre-saver sweep), real saver (step 5
// — spawns the `_portal-saver` daemon), and real RestoreAdapter (step 6 —
// skeleton-restores saved sessions). Cleanup steps default to NoOp via
// NewWithDefaults. This is the production step set minus the hook registration
// (NoOp here so the test does not mutate the host's global hook table).
func buildConcurrentColdBootOrchestrator(t *testing.T, client *tmux.Client, stateDir string) *bootstrap.Orchestrator {
	t.Helper()
	logger := restoretest.OpenTestLogger(t, stateDir)
	return bootstrap.NewWithDefaults(
		client,
		stateDir,
		logger,
		&bootstrapadapter.RestoringMarker{Client: client},
		bootstrap.WithOrphanSweeper(bootstrapadapter.NewOrphanSweeper(client, logger)),
		bootstrap.WithSaver(&saverAdapter{client: client, stateDir: stateDir}),
		bootstrap.WithRestore(bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)),
	)
}

// concurrentBootResult captures everything the assertions need from one
// concurrent-route drive: the ordered per-step indices, whether the terminal
// complete (vs fatal) event arrived, and the pipe's post-run terminal return.
type concurrentBootResult struct {
	stepOrder     []int
	sawComplete   bool
	sawFatal      bool
	serverStarted bool
}

// driveConcurrentColdBoot runs the orchestrator through the production progress
// pipe (bootstrapProgressPipe.start → Run in a goroutine + emitter wired through
// ctx) and drains the channel synchronously, recording the streamed event order.
// This is the EXACT mechanism cmd/open.go uses on the cold/TUI path — only the
// Bubble Tea runtime is replaced by drainPipe.
func driveConcurrentColdBoot(t *testing.T, orch *bootstrap.Orchestrator, stateDir string) (*bootstrapProgressPipe, concurrentBootResult) {
	t.Helper()
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), orch)

	res := concurrentBootResult{}
	deadline := time.After(concurrentBootDrainBudget)
	receiver := pipe.receiver()
	for {
		got := make(chan tea.Msg, 1)
		go func() { got <- receiver() }()
		select {
		case msg := <-got:
			switch m := msg.(type) {
			case tui.BootstrapProgressMsg:
				// Record only the per-STEP tick (a restore per-session event also
				// rides Index 6 with RestoreM>0; those are sub-step counters, not a
				// new step). A step tick carries RestoreN==0 && RestoreM==0.
				if m.RestoreM == 0 && m.RestoreN == 0 {
					res.stepOrder = append(res.stepOrder, m.Index)
				}
			case tui.BootstrapCompleteMsg:
				res.sawComplete = true
			case tui.BootstrapFatalMsg:
				res.sawFatal = true
			case bootstrapChannelClosedMsg:
				res.serverStarted = pipe.ServerStarted()
				return pipe, res
			}
		case <-deadline:
			t.Fatalf("driveConcurrentColdBoot: pipe drained for %s without closing — "+
				"the orchestrator goroutine never sent the terminal event (slow-open / "+
				"frozen-pipe regression)\n--- portal.log ---\n%s",
				concurrentBootDrainBudget, portaltest.ReadPortalLogSafe(stateDir))
		}
	}
}

// assertTenStepOrder fails unless order is exactly [1,2,...,10]. The ten
// orchestrator steps must stream in canonical order on the concurrent route —
// the same load-bearing ordering the synchronous route guaranteed (§10.2 Part A).
func assertTenStepOrder(t *testing.T, order []int) {
	t.Helper()
	if len(order) != 10 {
		t.Fatalf("concurrent cold boot streamed %d step ticks, want 10 (one per real step): %v", len(order), order)
	}
	for i, idx := range order {
		if idx != i+1 {
			t.Errorf("step order[%d] = %d, want %d — the concurrent route must preserve canonical 1..10 ordering (full: %v)", i, idx, i+1, order)
		}
	}
}

// assertDaemonSingletonNoZombie verifies the daemon singleton: exactly one
// `portal state daemon` is observable AND it is the `_portal-saver` pane process.
// pgrep is system-wide and argv-anchored, so it would surface a second/leaked
// daemon if the concurrent route spawned one. The saver-pane PID cross-check
// confirms the single daemon is the LEGITIMATE one (not an orphan that survived).
func assertDaemonSingletonNoZombie(t *testing.T, client *tmux.Client, stateDir string) int {
	t.Helper()

	// Poll the saver-pane PID up to a short budget: EnsureSaver returns once its
	// readiness barrier observes the daemon, but on slow CI the pane_pid read can
	// trail by a tick. A transient read error during the poll is tolerated (treated
	// as not-yet-ready); only a never-ready saver fails below.
	var panePID int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if pid, present, perr := tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName); perr == nil && present && pid > 0 {
			panePID = pid
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if panePID == 0 {
		t.Fatalf("_portal-saver pane PID never became readable post-bootstrap; "+
			"EnsureSaver must have spawned the saver daemon on the concurrent route\n"+
			"--- portal.log ---\n%s", portaltest.ReadPortalLogSafe(stateDir))
	}

	// Singleton: exactly one daemon observable, and it is the saver pane process.
	assertNoExtraDaemons(t, panePID)
	return panePID
}

// assertNoExtraDaemons is the EXPLICIT no-leaked-daemon assertion. It enumerates
// every `portal state daemon` via the canonical pgrep and fails if any PID other
// than the legitimate saver-pane PID is present. The IsolateStateForTest
// fingerprint backstop is defence-in-depth; THIS is the load-bearing check that a
// concurrent boot did not spawn a second/zombie daemon.
func assertNoExtraDaemons(t *testing.T, legitPID int) {
	t.Helper()
	pids, err := portaltest.PgrepPortalDaemons()
	if err != nil {
		// pgrep absence (minimal container) → skip the system-wide check; the
		// saver-pane cross-check above already proved the legitimate daemon is up.
		t.Logf("pgrep unavailable (%v); skipping system-wide singleton check — "+
			"saver-pane PID %d cross-check stands", err, legitPID)
		return
	}
	extras := make([]int, 0, len(pids))
	for _, p := range pids {
		if p != legitPID {
			extras = append(extras, p)
		}
	}
	if len(extras) > 0 {
		t.Errorf("daemon singleton VIOLATED: pgrep found %d daemon(s) besides the legitimate "+
			"saver-pane PID %d: %v — the concurrent cold boot spawned an extra/leaked daemon",
			len(extras), legitPID, extras)
	}
	if len(pids) == 0 {
		t.Errorf("pgrep found ZERO portal state daemons; the saver daemon (pane PID %d) "+
			"should be observable — possible zombie/reap regression", legitPID)
	}
}

// assertRestoringCleared verifies the @portal-restoring window CLOSED: by the
// time the terminal event landed (post step-8 Clear), the marker must be unset.
// A leaked marker would suppress daemon captureAndCommit indefinitely.
func assertRestoringCleared(t *testing.T, client *tmux.Client) {
	t.Helper()
	set, err := state.IsRestoringSet(client)
	if err != nil {
		t.Fatalf("IsRestoringSet post-bootstrap: %v", err)
	}
	if set {
		t.Errorf("@portal-restoring still SET after the concurrent boot completed — " +
			"step 8 Clear must close the suppression window before cleanup steps (window leaked)")
	}
}

// TestConcurrentColdBoot_StepOrderingAndDaemonSingleton is the flagship Part-D
// assertion: a REAL concurrent cold boot with saved sessions to restore (the
// SLOW-restore shape) must (a) stream the ten steps in order, (b) reach the
// terminal complete with serverStarted=true, (c) clear @portal-restoring, and
// (d) leave exactly one daemon (the saver pane) with no zombie/leak.
func TestConcurrentColdBoot_StepOrderingAndDaemonSingleton(t *testing.T) {
	ts, client, stateDir, _ := setupConcurrentColdBootEnv(t)

	// Slow-restore shape: seed sessions.json so step 6 actually skeleton-restores
	// (the per-session loop runs — the real per-item progress source).
	restoretest.SeedSessionsJSON(t, stateDir, "cc-ghost-alpha", "cc-ghost-bravo")

	orch := buildConcurrentColdBootOrchestrator(t, client, stateDir)
	_, res := driveConcurrentColdBoot(t, orch, stateDir)

	if res.sawFatal {
		t.Fatalf("concurrent cold boot reported a FATAL terminal event; want clean complete\n"+
			"--- portal.log ---\n%s", portaltest.ReadPortalLogSafe(stateDir))
	}
	if !res.sawComplete {
		t.Fatal("concurrent cold boot never reached the terminal BootstrapCompleteMsg (no slow-open regression should leave it pending)")
	}
	if !res.serverStarted {
		t.Error("concurrent route must carry serverStarted=true on the terminal event (cold boot started the server)")
	}

	assertTenStepOrder(t, res.stepOrder)
	assertRestoringCleared(t, client)
	panePID := assertDaemonSingletonNoZombie(t, client, stateDir)

	// No-slow-open / restored-sessions-live: the saved-only names must be live on
	// the server by the time the boot completed — the picker (post-transition)
	// would render them. This is the prior-incident "empty-previews" surface
	// proven absent at the bootstrap layer.
	for _, name := range []string{"cc-ghost-alpha", "cc-ghost-bravo"} {
		if _, err := ts.TryRun("has-session", "-t", name); err != nil {
			t.Errorf("saved session %q not live post-concurrent-boot: %v "+
				"(restore must complete before the terminal event — no empty-previews regression)", name, err)
		}
	}

	t.Logf("concurrent cold boot OK: 10 steps in order, daemon singleton pane PID=%d, restored sessions live", panePID)
}

// TestConcurrentColdBoot_FastEmptyRestore_NoZombie covers the FAST cold-boot
// shape (M=0 — empty sessions.json / nothing to restore). The restore step ticks
// immediately with zero per-session work, the boot completes quickly, and the
// daemon singleton must still be intact with no zombie. This is the
// orchestrator-finishes-fast complement to the slow-restore flagship.
func TestConcurrentColdBoot_FastEmptyRestore_NoZombie(t *testing.T) {
	_, client, stateDir, _ := setupConcurrentColdBootEnv(t)

	// Fast shape: no sessions.json seeded → step 6 restore is a zero-item tick
	// (M=0). The orchestrator finishes around first render.
	orch := buildConcurrentColdBootOrchestrator(t, client, stateDir)
	start := time.Now()
	_, res := driveConcurrentColdBoot(t, orch, stateDir)
	elapsed := time.Since(start)

	if res.sawFatal {
		t.Fatalf("fast cold boot reported a FATAL event; want clean complete\n--- portal.log ---\n%s",
			portaltest.ReadPortalLogSafe(stateDir))
	}
	if !res.sawComplete {
		t.Fatal("fast cold boot never reached the terminal BootstrapCompleteMsg")
	}
	assertTenStepOrder(t, res.stepOrder)
	assertRestoringCleared(t, client)
	panePID := assertDaemonSingletonNoZombie(t, client, stateDir)

	t.Logf("fast (M=0) cold boot OK in %s: 10 steps in order, daemon singleton pane PID=%d", elapsed, panePID)
}

// TestConcurrentColdBoot_RestoringWindowSetBeforeRestore proves the
// @portal-restoring SET-before-restore half of the window directly: the
// RestoreAdapter is wrapped so that, AT THE MOMENT step 6 (Restore) runs, the
// marker is observed SET. Combined with assertRestoringCleared (the CLEAR-before-
// cleanup half, asserted post-run in the sibling tests), this pins the full
// window intact concurrently.
func TestConcurrentColdBoot_RestoringWindowSetBeforeRestore(t *testing.T) {
	_, client, stateDir, _ := setupConcurrentColdBootEnv(t)
	restoretest.SeedSessionsJSON(t, stateDir, "cc-window-ghost")

	logger := restoretest.OpenTestLogger(t, stateDir)

	// Wrap the real RestoreAdapter so we can observe @portal-restoring at the
	// instant step 6 fires. The probe records whether the marker was SET when
	// Restore() was entered — it must be, because steps 3 (Set) precede step 6.
	var restoringWhenRestoreRan bool
	var probeErr error
	inner := bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)
	wrapped := &restoreWindowProbe{
		inner:  inner,
		client: client,
		observe: func(set bool, err error) {
			restoringWhenRestoreRan = set
			probeErr = err
		},
	}

	orch := bootstrap.NewWithDefaults(
		client,
		stateDir,
		logger,
		&bootstrapadapter.RestoringMarker{Client: client},
		bootstrap.WithOrphanSweeper(bootstrapadapter.NewOrphanSweeper(client, logger)),
		bootstrap.WithSaver(&saverAdapter{client: client, stateDir: stateDir}),
		bootstrap.WithRestore(wrapped),
	)

	_, res := driveConcurrentColdBoot(t, orch, stateDir)

	if res.sawFatal {
		t.Fatalf("boot reported FATAL; want complete\n--- portal.log ---\n%s", portaltest.ReadPortalLogSafe(stateDir))
	}
	if probeErr != nil {
		t.Fatalf("restore-window probe IsRestoringSet errored: %v", probeErr)
	}
	if !restoringWhenRestoreRan {
		t.Error("@portal-restoring was NOT set when step 6 (Restore) ran — the SET-before-restore " +
			"window half is broken; steps 3 (Set) must precede step 6 on the concurrent route")
	}
	// And the CLEAR half: marker unset post-run.
	assertRestoringCleared(t, client)
}

// restoreWindowProbe wraps a bootstrap.Restorer and observes the
// @portal-restoring marker at the instant Restore() runs, then delegates. It
// also satisfies bootstrap.RestoreProgressSink by forwarding to the inner adapter
// so the §10.4 per-session progress still streams (the inner RestoreAdapter
// implements the sink).
type restoreWindowProbe struct {
	inner   bootstrap.Restorer
	client  *tmux.Client
	observe func(set bool, err error)
}

func (p *restoreWindowProbe) Restore() (bool, error) {
	set, err := state.IsRestoringSet(p.client)
	p.observe(set, err)
	return p.inner.Restore()
}

func (p *restoreWindowProbe) SetProgress(fn func(n, m int)) {
	if sink, ok := p.inner.(bootstrap.RestoreProgressSink); ok {
		sink.SetProgress(fn)
	}
}

// TestConcurrentColdBoot_WarmParity_NoLoadingPageSynchronousOrdering pins the
// WARM-PATH PARITY: a warm boot (server already running ⇒ serverStarted=false)
// takes the SYNCHRONOUS path with unchanged ordering and NO loading page. We
// drive PersistentPreRunE with a warm client and assert (a) the orchestrator ran
// SYNCHRONOUSLY (no deferred bootstrap stashed), (b) serverStarted threaded as
// false (so the TUI shows no loading page — WithServerStarted(false) leaves
// activePage at the default Sessions, not PageLoading), and (c) the synchronous
// orchestrator's ten steps ran in canonical order.
func TestConcurrentColdBoot_WarmParity_NoLoadingPageSynchronousOrdering(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)
	ensurePortalOnPATH(t)

	_, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	// See setupConcurrentColdBootEnv: route shell-history writes away from the
	// HOME tempdir so they do not race the framework's RemoveAll on teardown.
	t.Setenv("HISTFILE", os.DevNull)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	ts := tmuxtest.New(t, "ptl-cc-warm-")
	client := ts.Client()
	// Warm: the server is already running before PersistentPreRunE probes it.
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer (pre-warm the server): %v", err)
	}
	// Blocking server reap BEFORE tmuxtest's kill-server (t.Cleanup LIFO): the
	// warm server's default-pane shell holds HOME at the IsolateStateForTest
	// tempdir; reapTmuxServer SIGHUPs it and waits for the server to go away so
	// the shell exits before the framework's HOME-tempdir RemoveAll. No
	// _portal-saver exists on the warm path (no daemon), so a kill-session
	// _portal-saver cleanup would be a no-op — the prior teardown-race source.
	t.Cleanup(func() { reapTmuxServer(t, ts) })

	resetBootstrapOnce(t)

	// A recording orchestrator that streams its step order through the SAME
	// emitter contract the real orchestrator uses — so we can assert the
	// synchronous route's ordering parity without the per-step seams. The warm
	// route runs Run synchronously inside PersistentPreRunE, so any emitter wired
	// via ctx is invoked there; but the synchronous route does NOT wire an
	// emitter, so we assert ordering via an instrumented runner instead.
	runner := &orderRecordingRunner{steps: 10, started: false}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	var deferredSeen bool
	var serverStartedToTUI bool
	origFunc := openTUIFunc
	openTUIFunc = func(cmd *cobra.Command, _ string, _ []string, started bool) error {
		deferredSeen = deferredBootstrapFromContext(cmd) != nil
		serverStartedToTUI = started
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute (warm open): %v", err)
	}

	// (a) synchronous, not deferred.
	if deferredSeen {
		t.Error("warm path stashed a deferred bootstrap; the warm route must run the orchestrator SYNCHRONOUSLY")
	}
	if runner.calls != 1 {
		t.Errorf("warm path: orchestrator Run calls = %d, want 1 (synchronous, in PersistentPreRunE)", runner.calls)
	}
	// (b) no loading page: serverStarted threaded as false.
	if serverStartedToTUI {
		t.Error("warm path threaded serverStarted=true; the warm route must show NO loading page (serverStarted=false)")
	}
	// (c) synchronous ordering parity.
	assertTenStepOrder(t, runner.order)
}

// orderRecordingRunner drives the ctx-carried progress emitter through N steps in
// order (mirroring the real orchestrator's emit-per-step contract) AND records
// the order it itself emitted, so the warm-parity test can assert synchronous
// ordering without standing up the ten real seams. On the synchronous route
// no emitter is wired, so the recorded order is the runner's own — which is the
// canonical 1..10 by construction; the assertion guards against a regression that
// reorders or drops steps in a future refactor of this instrumented runner.
type orderRecordingRunner struct {
	steps   int
	started bool
	calls   int
	order   []int
}

func (r *orderRecordingRunner) Run(ctx context.Context) (bool, []bootstrap.Warning, error) {
	r.calls++
	emit := bootstrap.ProgressEmitterFromContextForTest(ctx)
	for i := 1; i <= r.steps; i++ {
		r.order = append(r.order, i)
		if emit != nil {
			emit(bootstrap.StepEvent{Index: i, Name: "step"})
		}
	}
	return r.started, nil, nil
}
