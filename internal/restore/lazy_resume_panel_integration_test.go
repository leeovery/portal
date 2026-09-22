//go:build integration

package restore_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const (
	lazySubjectSession  = "lazysubject"
	eagerControlSession = "eagercontrol"

	// Both are the width and charset the pane-token mint produces, so the code
	// that recognises a token — and the refile that only moves a recognised
	// one's bytes — treats them as a real registration's token.
	lazySubjectToken  = "lazysb"
	eagerControlToken = "eagerc"

	// The subject's command carries a space and an embedded single quote: it is
	// both what the panel has to show back and what crosses the shell the helper
	// composed. It is short enough to render inside the card's wrapped rows.
	lazySubjectCommand = "echo 'resumed ok' | tee subject-fired"
	lazySubjectOutput  = "resumed ok"
	lazySubjectFired   = "subject-fired"

	eagerControlCommand = "echo eager-ok | tee control-fired"
	eagerControlFired   = "control-fired"

	subjectPreRebootLine = "subject-before-reboot"
	siblingPreRebootLine = "sibling-before-reboot"
	controlPreRebootLine = "control-before-reboot"
	siblingLiveLine      = "sibling-still-live"

	panelTitle       = "Resume session"
	panelResumeHint  = "⏎ resume"
	panelDiscardHint = "d discard"

	// The ceiling the daemon was measured at, in the kilobytes ps reports.
	restingTreeCeilingKB = 22 * 1024
)

func subjectPaneTarget() tmux.Target { return tmux.PaneTargetExact(lazySubjectSession, 0, 0) }
func siblingPaneTarget() tmux.Target { return tmux.PaneTargetExact(lazySubjectSession, 0, 1) }
func controlPaneTarget() tmux.Target { return tmux.PaneTargetExact(eagerControlSession, 0, 0) }
func subjectPaneKey() string         { return state.SanitizePaneKey(lazySubjectSession, 0, 0) }
func siblingPaneKey() string         { return state.SanitizePaneKey(lazySubjectSession, 0, 1) }
func controlPaneKey() string         { return state.SanitizePaneKey(eagerControlSession, 0, 0) }

func TestLazyResumePanel_RestoredPaneHoldsThePanel(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	fx := setupLazyResumePanel(t)
	firstScrollback := fx.readScrollbackAt(t, fx.subjectScrollbackRel)

	t.Run("it reports a lazy pane as pending and an eager one as not", func(t *testing.T) {
		pending := fx.pendingKeys(t)
		if _, waiting := pending[subjectPaneKey()]; !waiting {
			t.Fatalf("capture reported pending panes %v; want the lazy subject %q among them",
				restoretest.SortedKeySet(pending), subjectPaneKey())
		}
		if _, waiting := pending[controlPaneKey()]; waiting {
			t.Fatalf("capture reported the eager control %q as pending; an eager pane never waits",
				controlPaneKey())
		}
	})

	t.Run("it shows the panel over the pane's own transcript", func(t *testing.T) {
		fx.awaitScreenContains(t, subjectPaneTarget(), panelTitle)
		screen := fx.paneScreen(t, subjectPaneTarget())
		for _, want := range []string{panelResumeHint, panelDiscardHint} {
			if !strings.Contains(screen, want) {
				t.Fatalf("subject pane screen missing key hint %q; capture-pane -p:\n%s", want, screen)
			}
		}
		if strings.Contains(screen, subjectPreRebootLine) {
			t.Fatalf("subject pane screen still shows %q; the panel must cover the transcript, "+
				"capture-pane -p:\n%s", subjectPreRebootLine, screen)
		}
		history := fx.paneHistory(t, subjectPaneTarget())
		if !strings.Contains(history, subjectPreRebootLine) {
			t.Fatalf("subject pane history missing the pre-reboot line %q; capture-pane -a -p:\n%s",
				subjectPreRebootLine, history)
		}
	})

	t.Run("it shows the stored command on the panel through the shell the helper composed", func(t *testing.T) {
		screen := fx.paneScreen(t, subjectPaneTarget())
		if !strings.Contains(screen, lazySubjectCommand) {
			t.Fatalf("subject pane screen missing the stored command %q as stored; capture-pane -p:\n%s",
				lazySubjectCommand, screen)
		}
	})

	t.Run("it fires an eager registration in the same run", func(t *testing.T) {
		restoretest.WaitForFileExists(t, filepath.Join(fx.workDir, eagerControlFired),
			restoretest.PaneReactionBudget, restoretest.PaneReactionTick)

		transcript := fx.paneTranscript(t, controlPaneTarget())
		if !strings.Contains(transcript, controlPreRebootLine) {
			t.Fatalf("control pane missing its transcript line %q; capture-pane -p -S -:\n%s",
				controlPreRebootLine, transcript)
		}
		screen := fx.paneScreen(t, controlPaneTarget())
		if strings.Contains(screen, panelTitle) {
			t.Fatalf("control pane screen shows the resume panel; an eager pane fires instead. "+
				"capture-pane -p:\n%s", screen)
		}
		fx.assertNotPending(t, controlPaneTarget())
	})

	t.Run("it carries one shell parent and one waiter", func(t *testing.T) {
		tree := fx.restingTree(t)
		assertRestingTreeShape(t, tree)
	})

	t.Run("it holds the pane below the resident ceiling the daemon was measured at", func(t *testing.T) {
		tree := fx.restingTree(t)
		total := 0
		for _, p := range tree {
			total += p.rssKB
		}
		t.Logf("waiting pane resting tree: %d KB combined resident over %d processes\n%s",
			total, len(tree), formatTree(tree))
		if total >= restingTreeCeilingKB {
			t.Fatalf("resting tree carries %d KB resident; want below the daemon's %d KB ceiling\n%s",
				total, restingTreeCeilingKB, formatTree(tree))
		}
	})

	t.Run("it leaves the pane beside it live", func(t *testing.T) {
		fx.ts.SendKeys(t, siblingPaneTarget(), "echo "+siblingLiveLine)
		fx.awaitScreenContains(t, siblingPaneTarget(), siblingLiveLine)

		screen := fx.paneScreen(t, subjectPaneTarget())
		if !strings.Contains(screen, panelTitle) {
			t.Fatalf("subject pane no longer shows the panel after its sibling was typed into; "+
				"capture-pane -p:\n%s", screen)
		}
		fx.assertPending(t, subjectPaneTarget())
	})

	second := fx.captureRound(t)

	t.Run("it goes on capturing the pane beside it", func(t *testing.T) {
		if _, waiting := second.pending[siblingPaneKey()]; waiting {
			t.Fatalf("capture reported the unregistered sibling %q as pending", siblingPaneKey())
		}
		fx.assertNotPending(t, siblingPaneTarget())
		if screen := fx.paneScreen(t, siblingPaneTarget()); strings.Contains(screen, panelTitle) {
			t.Fatalf("sibling pane shows the resume panel; capture-pane -p:\n%s", screen)
		}
		if written, ok := second.written[siblingPaneKey()]; !ok || !written {
			t.Fatalf("sibling scrollback written=%v present=%v; a live pane beside a waiting one "+
				"goes on being captured", written, ok)
		}
		if _, captured := second.written[subjectPaneKey()]; captured {
			t.Fatalf("waiting subject %q was capture-paned; a frozen pane's scrollback must not be "+
				"rewritten", subjectPaneKey())
		}
	})

	t.Run("it keeps the waiting pane's transcript through a capture it was never answered through", func(t *testing.T) {
		record := fx.subjectRecord(t, second.idx)
		wantPath := state.PendingScrollbackFile(lazySubjectToken)
		if record.ScrollbackFile != wantPath {
			t.Fatalf("subject record names scrollback %q; want %q, the pane's own token, which is "+
				"where a frozen pane's bytes are carried to", record.ScrollbackFile, wantPath)
		}
		if got := fx.readScrollbackAt(t, record.ScrollbackFile); string(got) != string(firstScrollback) {
			t.Fatalf("subject scrollback changed while the pane waited\nbefore:\n%s\nafter:\n%s",
				firstScrollback, got)
		}
		if !strings.Contains(string(firstScrollback), subjectPreRebootLine) {
			t.Fatalf("the subject's saved scrollback never held %q; the fixture proves nothing:\n%s",
				subjectPreRebootLine, firstScrollback)
		}
	})

	t.Run("it offers the panel again on the reboot after the one that drew it", func(t *testing.T) {
		fx.rebootRestoreHydrate(t)

		fx.awaitScreenContains(t, subjectPaneTarget(), panelTitle)
		history := fx.paneHistory(t, subjectPaneTarget())
		if !strings.Contains(history, subjectPreRebootLine) {
			t.Fatalf("subject pane history lost %q across the second reboot; capture-pane -a -p:\n%s",
				subjectPreRebootLine, history)
		}
		fx.assertPending(t, subjectPaneTarget())
		if _, waiting := fx.pendingKeys(t)[subjectPaneKey()]; !waiting {
			t.Fatalf("subject %q absent from the fresh capture's pending set after the second restore",
				subjectPaneKey())
		}
	})

	t.Run("it clears the marker and runs the command on Enter", func(t *testing.T) {
		sentinel := filepath.Join(fx.workDir, lazySubjectFired)
		if _, err := os.Stat(sentinel); err == nil {
			t.Fatalf("subject sentinel %s exists before the panel was answered; a lazy registration "+
				"must not have fired", sentinel)
		}

		fx.ts.Run(t, "send-keys", "-t", string(subjectPaneTarget()), "Enter")

		cleared := harnesstest.PollUntil(t, restoretest.HydrateBudget, restoretest.HydrateTick, func() bool {
			value, err := fx.client.ReadPaneOption(subjectPaneTarget(), state.ResumePendingOption)
			return err == nil && !state.ResumePendingSet(value)
		})
		if !cleared {
			t.Fatalf("%s still set on the subject pane after Enter", state.ResumePendingOption)
		}

		restoretest.WaitForFileExists(t, sentinel, restoretest.PaneReactionBudget, restoretest.PaneReactionTick)
	})

	t.Run("it reveals the transcript that was underneath the panel", func(t *testing.T) {
		fx.awaitScreenContains(t, subjectPaneTarget(), lazySubjectOutput)
		transcript := fx.paneTranscript(t, subjectPaneTarget())
		if !strings.Contains(transcript, subjectPreRebootLine) {
			t.Fatalf("subject pane missing the transcript line %q the panel was covering; "+
				"capture-pane -p -S -:\n%s", subjectPreRebootLine, transcript)
		}
		if screen := fx.paneScreen(t, subjectPaneTarget()); strings.Contains(screen, panelTitle) {
			t.Fatalf("subject pane screen still shows the panel after it was answered; "+
				"capture-pane -p:\n%s", screen)
		}
	})

	t.Run("it closes the pane on the first exit after the resume", func(t *testing.T) {
		paneID := fx.paneID(t, subjectPaneTarget())

		fx.ts.SendKeys(t, subjectPaneTarget(), "exit")

		closed := harnesstest.PollUntil(t, exitClosesPaneBudget, restoretest.PaneReactionTick, func() bool {
			return !slices.Contains(fx.livePaneIDs(t, lazySubjectSession), paneID)
		})
		if !closed {
			t.Fatalf("pane %s survived %s after `exit`; a resumed pane closes on the first one. "+
				"live panes=%v", paneID, exitClosesPaneBudget, fx.livePaneIDs(t, lazySubjectSession))
		}
	})
}

// lazyPanelFixture is the two-session install the suite reboots: a lazy subject
// beside the plain pane a user works in, and an eager control proving both modes
// ship live together.
type lazyPanelFixture struct {
	ts       *tmuxtest.Socket
	client   *tmux.Client
	stateDir string
	binDir   string
	workDir  string

	hashes               state.HashMap
	prev                 state.Index
	subjectScrollbackRel string
}

func setupLazyResumePanel(t *testing.T) *lazyPanelFixture {
	t.Helper()

	fx := &lazyPanelFixture{
		binDir: restoretest.BuildPortalBinaryDir(t),
		hashes: state.HashMap{},
	}

	_, fx.stateDir = portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", fx.stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	configDir := t.TempDir()
	hooksPath := filepath.Join(configDir, "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)
	// Named but never written: the subject inherits the shipped lazy default
	// from an install that has set no resume mode at all.
	t.Setenv("PORTAL_PREFS_FILE", filepath.Join(configDir, "prefs.json"))
	seedLazyPanelHooks(t, hooksPath)

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, fx.stateDir)

	fx.ts = tmuxtest.New(t, "ptl-lazy-")
	fx.client = fx.ts.Client()
	fx.workDir = t.TempDir()

	fx.ts.Run(t, "new-session", "-d", "-s", lazySubjectSession, "-c", fx.workDir)
	fx.ts.WaitForSession(t, lazySubjectSession, 2*time.Second)
	fx.ts.Run(t, "split-window", "-t", string(tmux.PaneTarget(lazySubjectSession, 0, 0)), "-c", fx.workDir)
	fx.ts.Run(t, "new-session", "-d", "-s", eagerControlSession, "-c", fx.workDir)
	fx.ts.WaitForSession(t, eagerControlSession, 2*time.Second)

	fx.ts.StampPaneToken(t, subjectPaneTarget(), lazySubjectToken)
	fx.ts.StampPaneToken(t, controlPaneTarget(), eagerControlToken)

	fx.printLine(t, subjectPaneTarget(), subjectPreRebootLine)
	fx.printLine(t, siblingPaneTarget(), siblingPreRebootLine)
	fx.printLine(t, controlPaneTarget(), controlPreRebootLine)

	first := fx.captureRound(t)
	fx.subjectScrollbackRel = fx.subjectRecord(t, first.idx).ScrollbackFile

	fx.rebootRestoreHydrate(t)
	return fx
}

func seedLazyPanelHooks(t *testing.T, hooksPath string) {
	t.Helper()
	store := hooks.NewStore(hooksPath)
	// No mode of its own: the subject waits only because the install-wide
	// default is lazy.
	if err := store.Set(lazySubjectToken, "on-resume",
		hooks.Registration{Command: lazySubjectCommand}, hooks.ViaCLI); err != nil {
		t.Fatalf("hooks.Set subject: %v", err)
	}
	if err := store.Set(eagerControlToken, "on-resume",
		hooks.Registration{Command: eagerControlCommand, Resume: resumemode.Eager}, hooks.ViaCLI); err != nil {
		t.Fatalf("hooks.Set control: %v", err)
	}
}

// printLine leaves one recognisable line at the foot of a pane's transcript,
// over padding that fills the screen above it, and drops the history behind it.
// What a restored pane shows is the tail of what was replayed into it — each
// replayed line scrolls the screen once — so a line saved with blank rows under
// it is off the top of the restored screen before anything can read it, and the
// pane a panel covers has no history to reach it in.
func (fx *lazyPanelFixture) printLine(t *testing.T, target tmux.Target, line string) {
	t.Helper()
	fx.ts.SendKeys(t, target, "clear; for i in 1 2 3 4 5 6 7 8; do echo pad-$i; done; echo "+line)
	ran := harnesstest.PollUntil(t, restoretest.PaneReactionBudget, restoretest.PaneReactionTick, func() bool {
		return strings.Contains(fx.paneScreen(t, target), line)
	})
	if !ran {
		t.Fatalf("pane %s never printed %q; capture-pane -p:\n%s", target, line, fx.paneScreen(t, target))
	}
	fx.ts.Run(t, "clear-history", "-t", string(target))
}

// captureRoundResult is what one pass of the saver's own route produced: the
// index it committed, the panes it reported pending, and which panes it wrote
// scrollback for.
type captureRoundResult struct {
	idx     state.Index
	pending map[string]struct{}
	written map[string]bool
}

// captureRound takes a capture the way the daemon takes one, in its order:
// structure, the re-filing of every frozen pane's scrollback onto its own token,
// the per-pane scrollback dump, then the commit that reclaims whatever the
// committed index no longer names. A capture landing while a pane waits reaches
// the state an install reaches only if all four run.
func (fx *lazyPanelFixture) captureRound(t *testing.T) captureRoundResult {
	t.Helper()

	skipSet, err := state.ListSkeletonMarkers(fx.client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers: %v", err)
	}
	prev := fx.prev
	idx, pending, err := state.CaptureStructure(fx.client, skipSet, &prev, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}

	state.RefilePendingScrollback(fx.stateDir, &idx, pending, fx.hashes, nil)

	written := map[string]bool{}
	anyWritten := false
	for _, sess := range idx.Sessions {
		for _, win := range sess.Windows {
			for _, pane := range win.Panes {
				key := state.SanitizePaneKey(sess.Name, win.Index, pane.Index)
				if _, skipped := skipSet[key]; skipped {
					continue
				}
				if _, waiting := pending[key]; waiting {
					continue
				}
				target := tmux.PaneTargetExact(sess.Name, win.Index, pane.Index)
				data, hash, err := state.CaptureAndHashPane(fx.client, target)
				if err != nil {
					t.Fatalf("CaptureAndHashPane %s: %v", target, err)
				}
				wrote, err := state.WriteScrollbackIfChanged(fx.stateDir, key, data, hash, fx.hashes)
				if err != nil {
					t.Fatalf("WriteScrollbackIfChanged %s: %v", key, err)
				}
				written[key] = wrote
				anyWritten = anyWritten || wrote
			}
		}
	}

	if err := state.Commit(fx.stateDir, idx, anyWritten, nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	fx.prev = idx
	return captureRoundResult{idx: idx, pending: pending, written: written}
}

// rebootRestoreHydrate runs the whole recovery a user's reboot runs: the server
// is killed, the saved index restored onto a fresh one with the built binary,
// and every restored pane driven through its hydrate helper.
func (fx *lazyPanelFixture) rebootRestoreHydrate(t *testing.T) {
	t.Helper()
	restoretest.RebootServer(t, fx.ts, fx.client)
	if err := restoretest.RestoreFromState(t, fx.client, fx.stateDir, fx.binDir); err != nil {
		t.Fatalf("RestoreFromState: %v", err)
	}
	restoretest.DriveSignalHydrate(t, fx.client, fx.stateDir,
		[]string{lazySubjectSession, eagerControlSession})
	restoretest.WaitForSkeletonMarkersCleared(t, fx.client, restoretest.HydrateBudget, restoretest.HydrateTick)
}

func (fx *lazyPanelFixture) paneScreen(t *testing.T, target tmux.Target) string {
	t.Helper()
	return fx.ts.Run(t, "capture-pane", "-p", "-t", string(target))
}

// paneHistory reads what the pane holds underneath the alternate screen the
// panel is painted into.
func (fx *lazyPanelFixture) paneHistory(t *testing.T, target tmux.Target) string {
	t.Helper()
	return fx.ts.Run(t, "capture-pane", "-a", "-p", "-t", string(target))
}

// paneTranscript reads a pane's whole transcript, screen and scrollback alike,
// for a pane no panel is covering.
func (fx *lazyPanelFixture) paneTranscript(t *testing.T, target tmux.Target) string {
	t.Helper()
	return fx.ts.Run(t, "capture-pane", "-p", "-S", "-", "-t", string(target))
}

func (fx *lazyPanelFixture) awaitScreenContains(t *testing.T, target tmux.Target, want string) {
	t.Helper()
	shown := harnesstest.PollUntil(t, restoretest.HydrateBudget, restoretest.HydrateTick, func() bool {
		return strings.Contains(fx.paneScreen(t, target), want)
	})
	if !shown {
		t.Fatalf("pane %s never showed %q; capture-pane -p:\n%s", target, want, fx.paneScreen(t, target))
	}
}

func (fx *lazyPanelFixture) pendingKeys(t *testing.T) map[string]struct{} {
	t.Helper()
	_, pending, err := state.CaptureStructure(fx.client, nil, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	return pending
}

func (fx *lazyPanelFixture) assertPending(t *testing.T, target tmux.Target) {
	t.Helper()
	set := harnesstest.PollUntil(t, restoretest.HydrateBudget, restoretest.HydrateTick, func() bool {
		value, err := fx.client.ReadPaneOption(target, state.ResumePendingOption)
		return err == nil && state.ResumePendingSet(value)
	})
	if !set {
		t.Fatalf("%s is not set on pane %s; a waiting pane's freeze was never written",
			state.ResumePendingOption, target)
	}
}

func (fx *lazyPanelFixture) assertNotPending(t *testing.T, target tmux.Target) {
	t.Helper()
	value, err := fx.client.ReadPaneOption(target, state.ResumePendingOption)
	if err != nil {
		t.Fatalf("ReadPaneOption %s on %s: %v", state.ResumePendingOption, target, err)
	}
	if state.ResumePendingSet(value) {
		t.Fatalf("%s = %q on pane %s; want it unset", state.ResumePendingOption, value, target)
	}
}

func (fx *lazyPanelFixture) subjectRecord(t *testing.T, idx state.Index) state.Pane {
	t.Helper()
	sess := restoretest.FindCapturedSession(t, idx, lazySubjectSession)
	if len(sess.Windows) != 1 || len(sess.Windows[0].Panes) != 2 {
		t.Fatalf("captured session %q topology = %v; want one window of two panes",
			lazySubjectSession, paneIndices(sess))
	}
	return sess.Windows[0].Panes[0]
}

// readScrollbackAt reads the bytes a saved record names, rather than the bytes
// at an address: a frozen pane's scrollback is re-filed onto its durable token,
// so the path is the record's to state and never the reader's to compose.
func (fx *lazyPanelFixture) readScrollbackAt(t *testing.T, stored string) []byte {
	t.Helper()
	path := filepath.Join(fx.stateDir, stored)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scrollback %s: %v", path, err)
	}
	return data
}

// restingTree returns the subject pane's process tree once the draw has handed
// the pane over: the fixture waits for the hand-off rather than assuming it,
// because the assertions about the tree's shape are the caller's.
func (fx *lazyPanelFixture) restingTree(t *testing.T) []paneProcess {
	t.Helper()
	rootPID := fx.panePID(t, subjectPaneTarget())
	var tree []paneProcess
	settled := harnesstest.PollUntil(t, restoretest.HydrateBudget, restoretest.HydrateTick, func() bool {
		tree = readProcessTree(t, rootPID)
		return len(tree) == 2 && !treeRuns(tree, resumeDrawArgv)
	})
	if !settled {
		t.Fatalf("subject pane %d never settled after the draw handed off:\n%s", rootPID, formatTree(tree))
	}
	return tree
}

// paneID is the pane's own server-unique id. A pane's coordinates are not an
// identity: tmux renumbers the panes left in a window the moment one closes, so
// a coordinate that still answers may be a different pane.
func (fx *lazyPanelFixture) paneID(t *testing.T, target tmux.Target) string {
	t.Helper()
	return strings.TrimSpace(fx.ts.Run(t, "display-message", "-p", "-t", string(target), "#{pane_id}"))
}

func (fx *lazyPanelFixture) livePaneIDs(t *testing.T, session string) []string {
	t.Helper()
	out := fx.ts.Run(t, "list-panes", "-s", "-t", string(tmux.CoordTargetExact(session)), "-F", "#{pane_id}")
	return strings.Fields(out)
}

func (fx *lazyPanelFixture) panePID(t *testing.T, target tmux.Target) int {
	t.Helper()
	raw := strings.TrimSpace(fx.ts.Run(t, "display-message", "-p", "-t", string(target), "#{pane_pid}"))
	pid, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("pane_pid for %s = %q: %v", target, raw, err)
	}
	return pid
}

// The chain's subcommands as ps renders them. The pane's own process is the
// shell the chain was handed to, which quotes each word of the argv it holds,
// so only its subcommand names are matched on.
const (
	resumeDrawArgv    = "resume-draw"
	resumeWaitArgv    = "resume-wait"
	resumeRecoverArgv = "resume-recover"
)

// paneProcess is one row of the read the suite takes outside tmux. It signals
// nothing and enumerates nothing it did not cause to exist: the root is the pid
// tmux reports for the fixture's own pane.
type paneProcess struct {
	pid     int
	ppid    int
	rssKB   int
	command string
}

func assertRestingTreeShape(t *testing.T, tree []paneProcess) {
	t.Helper()
	if len(tree) != 2 {
		t.Fatalf("waiting pane carries %d processes; want one shell parent over one waiter\n%s",
			len(tree), formatTree(tree))
	}
	parent, child := tree[0], tree[1]
	if child.ppid != parent.pid {
		t.Fatalf("second process is not a child of the pane's own process\n%s", formatTree(tree))
	}
	if !strings.Contains(parent.command, "sh -c") || !strings.Contains(parent.command, resumeRecoverArgv) {
		t.Fatalf("the pane's own process is not the shell parking the chain's tail\n%s", formatTree(tree))
	}
	if !strings.Contains(child.command, resumeWaitArgv) {
		t.Fatalf("the parked shell's child is not the waiter\n%s", formatTree(tree))
	}
	if strings.Contains(child.command, resumeDrawArgv) {
		t.Fatalf("the draw is still resident under the parked shell\n%s", formatTree(tree))
	}
}

// treeRuns ignores the pane's own process, the parked shell whose argv names
// every command of the chain it was handed.
func treeRuns(tree []paneProcess, argv string) bool {
	if len(tree) == 0 {
		return false
	}
	for _, p := range tree[1:] {
		if strings.Contains(p.command, argv) {
			return true
		}
	}
	return false
}

func formatTree(tree []paneProcess) string {
	var b strings.Builder
	for _, p := range tree {
		fmt.Fprintf(&b, "  pid=%d ppid=%d rss=%dKB %s\n", p.pid, p.ppid, p.rssKB, p.command)
	}
	return b.String()
}

func readProcessTree(t *testing.T, rootPID int) []paneProcess {
	t.Helper()
	return describeProcesses(t, collectDescendants(t, rootPID))
}

func collectDescendants(t *testing.T, rootPID int) []int {
	t.Helper()
	pids := []int{rootPID}
	for i := 0; i < len(pids); i++ {
		pids = append(pids, childPIDs(t, pids[i])...)
	}
	return pids
}

// childPIDs reads one process's children and nothing else, so the enumeration
// can never widen past the fixture's own pane.
func childPIDs(t *testing.T, pid int) []int {
	t.Helper()
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(pid)).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil
		}
		t.Fatalf("pgrep -P %d: %v", pid, err)
	}
	return parsePIDs(t, string(out))
}

func parsePIDs(t *testing.T, raw string) []int {
	t.Helper()
	var pids []int
	for field := range strings.FieldsSeq(raw) {
		pid, err := strconv.Atoi(field)
		if err != nil {
			t.Fatalf("pgrep returned %q: %v", field, err)
		}
		pids = append(pids, pid)
	}
	return pids
}

func describeProcesses(t *testing.T, pids []int) []paneProcess {
	t.Helper()
	if len(pids) == 0 {
		return nil
	}
	list := make([]string, 0, len(pids))
	for _, pid := range pids {
		list = append(list, strconv.Itoa(pid))
	}
	out, err := exec.Command("ps", "-o", "pid=,ppid=,rss=,command=", "-p", strings.Join(list, ",")).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil
		}
		t.Fatalf("ps -p %v: %v", list, err)
	}

	rows := map[int]paneProcess{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		row := parseProcessRow(t, line)
		rows[row.pid] = row
	}

	// ps orders by pid; the caller reads the pane's own process off the front,
	// so the rows come back in the order they were asked for.
	tree := make([]paneProcess, 0, len(rows))
	for _, pid := range pids {
		if row, ok := rows[pid]; ok {
			tree = append(tree, row)
		}
	}
	return tree
}

func parseProcessRow(t *testing.T, line string) paneProcess {
	t.Helper()
	fields := strings.Fields(line)
	if len(fields) < 4 {
		t.Fatalf("ps row %q has %d fields; want pid, ppid, rss and a command", line, len(fields))
	}
	numbers := make([]int, 3)
	for i := range numbers {
		n, err := strconv.Atoi(fields[i])
		if err != nil {
			t.Fatalf("ps row %q field %d = %q: %v", line, i, fields[i], err)
		}
		numbers[i] = n
	}
	return paneProcess{
		pid:     numbers[0],
		ppid:    numbers[1],
		rssKB:   numbers[2],
		command: strings.Join(fields[3:], " "),
	}
}
