//go:build integration

package restore_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	discardSubjectSession   = "discardsubject"
	discardBystanderSession = "discardbystander"

	// The width and charset the pane-token mint produces.
	discardSubjectToken   = "dscsub"
	discardBystanderToken = "dscbys"

	// The subject's command must never run: it leaves a sentinel if it does.
	discardSubjectCommand   = "echo 'gone' | tee discard-fired"
	discardSubjectFired     = "discard-fired"
	discardBystanderCommand = "echo bystander-ok"

	discardSubjectPreRebootLine   = "discard-subject-before-reboot"
	discardBystanderPreRebootLine = "discard-bystander-before-reboot"

	discardConfirmTitle  = "▲ Discard resume?"
	discardConfirmFooter = "y discard   esc cancel"
	panelCommandLabel    = "ON RESUME"
)

func discardSubjectTarget() tmux.Target {
	return tmux.PaneTargetExact(discardSubjectSession, 0, 0)
}

func discardBystanderTarget() tmux.Target {
	return tmux.PaneTargetExact(discardBystanderSession, 0, 0)
}

func discardSubjectKey() string { return state.SanitizePaneKey(discardSubjectSession, 0, 0) }

var discardScreenCopy = []string{
	panelTitle, panelCommandLabel, panelResumeHint, panelDiscardHint, discardConfirmTitle,
}

func TestLazyResumeDiscard_RealPaneDiscardsItsResume(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	fx := setupLazyResumeDiscard(t)
	subject := discardSubjectTarget()

	// Taken while the subject waits, so the write that lands after the discard
	// is one this pane was previously refused.
	whileWaiting := fx.captureRound(t)

	t.Run("it shows the confirmation over the pane's own transcript", func(t *testing.T) {
		fx.awaitScreenContains(t, subject, panelTitle)
		fx.assertScreenHas(t, subject, panelResumeHint, panelDiscardHint)
		fx.assertHistoryHasPreRebootLine(t)

		fx.sendKey(t, "d")

		fx.awaitScreenContains(t, subject, discardConfirmTitle)
		fx.assertScreenHas(t, subject, discardSubjectCommand, discardConfirmFooter)
		fx.assertHistoryHasPreRebootLine(t)
	})

	t.Run("it returns the waiting panel on Escape", func(t *testing.T) {
		fx.sendKey(t, "Escape")

		fx.awaitScreen(t, subject, "the waiting panel back in place of the confirmation", func(screen string) bool {
			return strings.Contains(screen, panelTitle) && !strings.Contains(screen, discardConfirmTitle)
		})
		fx.assertScreenHas(t, subject, panelResumeHint, panelDiscardHint)
		fx.assertPending(t, subject)
	})

	t.Run("it removes only the subject's registration", func(t *testing.T) {
		fx.sendKey(t, "d")
		fx.awaitScreenContains(t, subject, discardConfirmTitle)
		fx.sendKey(t, "y")

		cleared := harnesstest.PollUntil(t, restoretest.HydrateBudget, restoretest.HydrateTick, func() bool {
			value, err := fx.client.ReadPaneOption(subject, state.ResumePendingOption)
			return err == nil && !state.ResumePendingSet(value)
		})
		if !cleared {
			t.Fatalf("%s still set on the subject pane after y", state.ResumePendingOption)
		}

		entries := decodeHookEntries(t, fx.readHooks(t))
		if raw, present := entries[discardSubjectToken]; present {
			t.Fatalf("hooks.json still carries the subject %q after the discard: %s", discardSubjectToken, raw)
		}
		seeded := decodeHookEntries(t, fx.seededHooks)
		want, got := seeded[discardBystanderToken], entries[discardBystanderToken]
		if string(got) != string(want) {
			t.Fatalf("bystander entry changed across the discard\nseeded:\n%s\nnow:\n%s", want, got)
		}
		if len(entries) != 1 {
			t.Fatalf("hooks.json holds %d entries after the discard; want only the bystander's:\n%s",
				len(entries), fx.readHooks(t))
		}
	})

	t.Run("it reveals the transcript that was underneath the confirmation", func(t *testing.T) {
		fx.awaitScreen(t, subject, "the pre-reboot line with no trace of either screen", func(screen string) bool {
			if !strings.Contains(screen, discardSubjectPreRebootLine) {
				return false
			}
			for _, piece := range discardScreenCopy {
				if strings.Contains(screen, piece) {
					return false
				}
			}
			return true
		})

		// The hand-off that would wrongly run the command follows the marker's
		// clear; give it every chance to write its sentinel before concluding
		// it never ran.
		time.Sleep(restoretest.PaneReactionBudget)
		if _, err := os.Stat(filepath.Join(fx.workDir, discardSubjectFired)); err == nil {
			t.Fatalf("the subject's registered command ran; a discard never runs it")
		}
	})

	t.Run("it leaves the pane's durable token stamped and its session live", func(t *testing.T) {
		token, err := fx.client.ReadPaneOption(subject, state.PortalPaneIDOption)
		if err != nil {
			t.Fatalf("ReadPaneOption %s: %v", state.PortalPaneIDOption, err)
		}
		if token != discardSubjectToken {
			t.Fatalf("subject %s = %q after the discard; want %q untouched",
				state.PortalPaneIDOption, token, discardSubjectToken)
		}
		live, err := fx.client.HasSessionProbe(discardSubjectSession)
		if err != nil {
			t.Fatalf("HasSessionProbe %s: %v", discardSubjectSession, err)
		}
		if !live {
			t.Fatalf("session %s is gone after the discard; a discard never touches the session",
				discardSubjectSession)
		}
		idx, _, err := state.CaptureStructure(fx.client, nil, nil, nil)
		if err != nil {
			t.Fatalf("CaptureStructure: %v", err)
		}
		restoretest.FindCapturedSession(t, idx, discardSubjectSession)
	})

	t.Run("it clears the pending marker and resumes writing the pane's scrollback", func(t *testing.T) {
		fx.assertNotPending(t, subject)
		if pending := fx.pendingKeys(t); len(pending) != 0 {
			t.Fatalf("capture reports pending panes %v after the discard; want none",
				restoretest.SortedKeySet(pending))
		}
		if _, captured := whileWaiting.written[discardSubjectKey()]; captured {
			t.Fatalf("waiting subject %q was capture-paned before the discard; the fixture proves nothing",
				discardSubjectKey())
		}

		after := fx.captureRound(t)
		if _, waiting := after.pending[discardSubjectKey()]; waiting {
			t.Fatalf("capture still reports the subject %q as pending after the discard", discardSubjectKey())
		}
		if written, ok := after.written[discardSubjectKey()]; !ok || !written {
			t.Fatalf("subject scrollback written=%v captured=%v after the discard; a pane that no "+
				"longer waits is captured again", written, ok)
		}
		// The answered pane's record goes back to its positional file, which
		// holds the capture just written; a record left on the token-named
		// transcript would let housekeeping delete that capture and restore the
		// pre-answer transcript on the next reboot.
		sess := restoretest.FindCapturedSession(t, after.idx, discardSubjectSession)
		if len(sess.Windows) == 0 || len(sess.Windows[0].Panes) == 0 {
			t.Fatalf("captured session %q holds no pane 0.0", discardSubjectSession)
		}
		record := sess.Windows[0].Panes[0]
		positional := state.ScrollbackFile(fx.stateDir, discardSubjectKey())
		if got := filepath.Join(fx.stateDir, record.ScrollbackFile); got != positional {
			t.Fatalf("subject record names %q after the discard; want its positional file %q",
				record.ScrollbackFile, positional)
		}
		if got := fx.readScrollbackAt(t, record.ScrollbackFile); string(got) != string(after.captured[discardSubjectKey()]) {
			t.Fatalf("subject positional file holds %q; want the capture the round wrote %q",
				got, after.captured[discardSubjectKey()])
		}
	})

	t.Run("it closes the pane on the first exit", func(t *testing.T) {
		// The save the next reboot restores from, taken while the pane is alive.
		fx.captureRound(t)

		fx.ts.SendKeys(t, subject, "exit")

		awaitPaneGone(t, fx.ts, discardSubjectSession, 0, 0, exitClosesPaneBudget)
	})

	t.Run("it restores the pane with no panel on the next reboot", func(t *testing.T) {
		fx.rebootRestoreHydrate(t)

		fx.awaitScreenContains(t, subject, discardSubjectPreRebootLine)
		// A bare shell execs at once; give a panel every chance to appear
		// before concluding none will.
		time.Sleep(restoretest.PaneReactionBudget)
		screen := fx.paneScreen(t, subject)
		for _, piece := range discardScreenCopy {
			if strings.Contains(screen, piece) {
				t.Fatalf("restored subject shows %q; a discarded resume is never offered again. "+
					"capture-pane -p:\n%s", piece, screen)
			}
		}
		fx.assertNotPending(t, subject)

		tree := readProcessTree(t, fx.panePID(t, subject))
		for _, p := range tree {
			if strings.Contains(p.command, resumeWaitArgv) || strings.Contains(p.command, resumeDrawArgv) {
				t.Fatalf("restored subject still carries the resume chain\n%s", formatTree(tree))
			}
		}
	})
}

// lazyDiscardFixture is the two-session install the suite reboots: a lazy
// subject whose resume is discarded, and an eager bystander whose registration
// must come through the discard's rewrite of the store byte for byte.
type lazyDiscardFixture struct {
	*lazyPanelFixture

	hooksPath   string
	seededHooks []byte
}

func setupLazyResumeDiscard(t *testing.T) *lazyDiscardFixture {
	t.Helper()

	fx := &lazyDiscardFixture{lazyPanelFixture: &lazyPanelFixture{
		binDir: restoretest.BuildPortalBinaryDir(t),
		hashes: state.HashMap{},
	}}

	_, fx.stateDir = portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", fx.stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	configDir := t.TempDir()
	fx.hooksPath = filepath.Join(configDir, "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", fx.hooksPath)
	// Named but never written: the subject inherits the shipped lazy default.
	t.Setenv("PORTAL_PREFS_FILE", filepath.Join(configDir, "prefs.json"))
	fx.seedHooks(t)

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, fx.stateDir)

	fx.ts = tmuxtest.New(t, "ptl-discard-")
	fx.client = fx.ts.Client()
	fx.workDir = t.TempDir()

	fx.ts.Run(t, "new-session", "-d", "-s", discardSubjectSession, "-c", fx.workDir)
	fx.ts.WaitForSession(t, discardSubjectSession, 2*time.Second)
	fx.ts.Run(t, "new-session", "-d", "-s", discardBystanderSession, "-c", fx.workDir)
	fx.ts.WaitForSession(t, discardBystanderSession, 2*time.Second)

	fx.ts.StampPaneToken(t, discardSubjectTarget(), discardSubjectToken)
	fx.ts.StampPaneToken(t, discardBystanderTarget(), discardBystanderToken)

	fx.printLine(t, discardSubjectTarget(), discardSubjectPreRebootLine)
	fx.printLine(t, discardBystanderTarget(), discardBystanderPreRebootLine)

	fx.captureRound(t)
	fx.rebootRestoreHydrate(t)
	return fx
}

// seedHooks writes the subject in the string form, so it waits only because
// the install-wide default is lazy, and the bystander in the object form
// carrying its own mode — the shape a rewrite could re-marshal differently.
func (fx *lazyDiscardFixture) seedHooks(t *testing.T) {
	t.Helper()
	store := hooks.NewStore(fx.hooksPath)
	if err := store.Set(discardSubjectToken, "on-resume",
		hooks.Registration{Command: discardSubjectCommand}, hooks.ViaCLI); err != nil {
		t.Fatalf("hooks.Set subject: %v", err)
	}
	if err := store.Set(discardBystanderToken, "on-resume",
		hooks.Registration{Command: discardBystanderCommand, Resume: resumemode.Eager}, hooks.ViaCLI); err != nil {
		t.Fatalf("hooks.Set bystander: %v", err)
	}
	fx.seededHooks = fx.readHooks(t)

	bystander := string(decodeHookEntries(t, fx.seededHooks)[discardBystanderToken])
	if !strings.Contains(bystander, `"resume"`) || !strings.Contains(bystander, `"eager"`) {
		t.Fatalf("seeded bystander is not the object form carrying its resume mode:\n%s", bystander)
	}
}

func (fx *lazyDiscardFixture) readHooks(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(fx.hooksPath)
	if err != nil {
		t.Fatalf("read %s: %v", fx.hooksPath, err)
	}
	return data
}

// decodeHookEntries splits hooks.json into each key's entry as the bytes the
// file holds for it, so an entry is compared exactly as it was written.
func decodeHookEntries(t *testing.T, data []byte) map[string]json.RawMessage {
	t.Helper()
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("decode hooks.json: %v\n%s", err, data)
	}
	return entries
}

func (fx *lazyDiscardFixture) rebootRestoreHydrate(t *testing.T) {
	t.Helper()
	restoretest.RebootServer(t, fx.ts, fx.client)
	if err := restoretest.RestoreWithMarker(t, fx.client,
		restoretest.NewRestoreOrchestrator(t, fx.client, fx.stateDir, fx.binDir)); err != nil {
		t.Fatalf("RestoreWithMarker: %v", err)
	}
	restoretest.DriveSignalHydrate(t, fx.client, fx.stateDir,
		[]string{discardSubjectSession, discardBystanderSession})
	restoretest.WaitForSkeletonMarkersCleared(t, fx.client, restoretest.HydrateBudget, restoretest.HydrateTick)
}

// sendKey delivers one key to the subject with no Enter behind it: each screen
// answers single keys, and a stray Enter would answer the waiting panel.
func (fx *lazyDiscardFixture) sendKey(t *testing.T, key string) {
	t.Helper()
	fx.ts.Run(t, "send-keys", "-t", string(discardSubjectTarget()), key)
}

func (fx *lazyDiscardFixture) awaitScreen(t *testing.T, target tmux.Target, what string, ok func(string) bool) {
	t.Helper()
	shown := harnesstest.PollUntil(t, restoretest.HydrateBudget, restoretest.HydrateTick, func() bool {
		return ok(fx.paneScreen(t, target))
	})
	if !shown {
		t.Fatalf("pane %s never showed %s; capture-pane -p:\n%s", target, what, fx.paneScreen(t, target))
	}
}

func (fx *lazyDiscardFixture) assertScreenHas(t *testing.T, target tmux.Target, wants ...string) {
	t.Helper()
	screen := fx.paneScreen(t, target)
	for _, want := range wants {
		if !strings.Contains(screen, want) {
			t.Fatalf("pane %s screen missing %q; capture-pane -p:\n%s", target, want, screen)
		}
	}
}

// assertHistoryHasPreRebootLine reads underneath the alternate screen either
// screen is painted into.
func (fx *lazyDiscardFixture) assertHistoryHasPreRebootLine(t *testing.T) {
	t.Helper()
	history := fx.paneHistory(t, discardSubjectTarget())
	if !strings.Contains(history, discardSubjectPreRebootLine) {
		t.Fatalf("subject history missing the pre-reboot line %q; capture-pane -a -p:\n%s",
			discardSubjectPreRebootLine, history)
	}
}
