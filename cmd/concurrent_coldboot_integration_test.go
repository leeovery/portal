//go:build integration

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

const concurrentBootDrainBudget = 15 * time.Second

// setupConcurrentColdBootEnv deliberately does not pre-start the saver: the
// orchestrator's EnsureSaver step is what the cold route exercises.
func setupConcurrentColdBootEnv(t *testing.T) (*tmuxtest.Socket, *tmux.Client, string, []string) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ensurePortalOnPATH(t)

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	ts := tmuxtest.New(t, "ptl-cc-coldboot-")
	client := ts.Client()

	// Registered after tmuxtest.New so LIFO reaps the daemon before kill-server:
	// a live daemon still holding state-dir fds makes the TempDir RemoveAll fail
	// with "directory not empty" on macOS.
	t.Cleanup(func() {
		reapSaverDaemon(t, ts, client, stateDir)
	})

	resetBootstrapOnce(t)

	return ts, client, stateDir, envSlice
}

// reapTmuxServer kills the whole server, not just a session, so every pane
// shell gets SIGHUP, then waits for the server to stop answering.
func reapTmuxServer(t *testing.T, ts *tmuxtest.Socket) {
	t.Helper()
	ts.KillServer()
	portaltest.AwaitTmuxServerGone(t, ts.SocketPath())
}

// reapSaverDaemon additionally waits for the daemon process itself to exit: its
// state-dir fd release trails the pane vanishing, and t.TempDir cleanup needs
// those released. Best-effort.
func reapSaverDaemon(t *testing.T, ts *tmuxtest.Socket, client *tmux.Client, stateDir string) {
	t.Helper()
	// Snapshotted before the kill so its liveness can be polled directly.
	pid, _ := state.ReadPIDFile(stateDir)
	reapTmuxServer(t, ts)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		// Mid-teardown a transient tmux error also means absent.
		_, present, perr := tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName)
		saverGone := perr != nil || !present
		daemonDead := pid <= 0 || !pidIsAlive(pid)
		if saverGone && daemonDead {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

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

// buildConcurrentColdBootOrchestrator wires the production step set minus hook
// registration, which stays NoOp so the test never mutates the host's global
// hook table.
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
		bootstrap.WithRestore(restoretest.StagedRestoreAdapter(t, client, stateDir, logger, ensurePortalOnPATH(t))),
	)
}

type concurrentBootResult struct {
	stepOrder     []int
	sawComplete   bool
	sawFatal      bool
	serverStarted bool
}

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
				// Restore's per-session events also ride Index 6, so only the
				// zero-counter ticks are real steps.
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

func assertDaemonSingletonNoZombie(t *testing.T, client *tmux.Client, stateDir string) int {
	t.Helper()

	// EnsureSaver returns once its readiness barrier observes the daemon, but on a
	// slow host the pane_pid read can trail by a tick, so poll rather than read once.
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

	assertNoExtraDaemons(t, panePID)
	return panePID
}

func assertNoExtraDaemons(t *testing.T, legitPID int) {
	t.Helper()
	pids, err := portaltest.PgrepPortalDaemons()
	if err != nil {
		// No pgrep (minimal container): the saver-pane cross-check above already
		// covers the legitimate daemon.
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

// assertRestoringCleared guards the leak: a marker left set suppresses daemon
// captureAndCommit indefinitely.
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

func TestConcurrentColdBoot_StepOrderingAndDaemonSingleton(t *testing.T) {
	ts, client, stateDir, _ := setupConcurrentColdBootEnv(t)

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

	for _, name := range []string{"cc-ghost-alpha", "cc-ghost-bravo"} {
		if _, err := ts.TryRun("has-session", "-t", name); err != nil {
			t.Errorf("saved session %q not live post-concurrent-boot: %v "+
				"(restore must complete before the terminal event — no empty-previews regression)", name, err)
		}
	}

	t.Logf("concurrent cold boot OK: 10 steps in order, daemon singleton pane PID=%d, restored sessions live", panePID)
}

func TestConcurrentColdBoot_FastEmptyRestore_NoZombie(t *testing.T) {
	_, client, stateDir, _ := setupConcurrentColdBootEnv(t)

	// No sessions.json seeded, so step 6 is a zero-item tick.
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

func TestConcurrentColdBoot_RestoringWindowSetBeforeRestore(t *testing.T) {
	_, client, stateDir, _ := setupConcurrentColdBootEnv(t)
	restoretest.SeedSessionsJSON(t, stateDir, "cc-window-ghost")

	logger := restoretest.OpenTestLogger(t, stateDir)

	var restoringWhenRestoreRan bool
	var probeErr error
	inner := restoretest.StagedRestoreAdapter(t, client, stateDir, logger, ensurePortalOnPATH(t))
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
	assertRestoringCleared(t, client)
}

// Forwards SetProgress so per-session progress still streams while it observes
// @portal-restoring at the instant Restore() runs.
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

func TestConcurrentColdBoot_WarmUnlatchedOpen_TakesConcurrentDeferredRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)
	ensurePortalOnPATH(t)

	_, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	ts := tmuxtest.New(t, "ptl-cc-warm-")
	client := ts.Client()
	// Warm: the server runs before the latch probe, but nothing stamps
	// @portal-bootstrapped, so the latch reads not-satisfied.
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer (pre-warm the server): %v", err)
	}
	if state.BootstrappedLatchSatisfied(client, version) {
		t.Fatal("latch unexpectedly satisfied on a warm-but-unstamped server; the warm-unlatched premise is broken")
	}
	// No saver daemon is spawned on this route (openTUIFunc is stubbed below), so
	// the plain server reap is the whole cleanup.
	t.Cleanup(func() { reapTmuxServer(t, ts) })

	resetBootstrapOnce(t)
	resetBootstrapWarnings(t)

	runner := &recordingRunner{started: true}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

	var deferredSeen bool
	withFuncSeam(t, &openTUIFunc, func(cmd *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		if runner.calls != 0 {
			t.Errorf("orchestrator ran synchronously (%d calls) on the warm-unlatched TUI path; want deferred", runner.calls)
		}
		deferredSeen = deferredBootstrapFromContext(cmd) != nil
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute (warm-unlatched open): %v", err)
	}

	if !deferredSeen {
		t.Error("warm-unlatched open did not stash a deferred bootstrap; the concurrent + loading route is expected (trigger is latch-absent, not server-down)")
	}
	if runner.calls != 0 {
		t.Errorf("warm-unlatched open: orchestrator calls = %d, want 0 (deferred to openTUI's goroutine, not synchronous)", runner.calls)
	}
}
