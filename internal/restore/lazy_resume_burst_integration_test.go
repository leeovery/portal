//go:build integration

package restore_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const (
	// Every line a shell could run from the paste appends here.
	burstSentinel = "burst-ran"
	burstLine     = "echo 0123456789 >> " + burstSentinel + "\n"

	burstDropReport = "can't clear pending input"

	burstReplacementCommand = "echo 'replaced' | tee replacement-fired"
)

// multiLinePaste is a paste of exactly size bytes opening with the discard key
// and ending in an unterminated "yes", with no other d or y between them.
func multiLinePaste(t *testing.T, size int) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("d\n")
	for b.Len()+len(burstLine)+len("yes") <= size {
		b.WriteString(burstLine)
	}
	if pad := size - b.Len() - len("yes"); pad > 0 {
		b.WriteString(strings.Repeat("x", pad-1) + "\n")
	}
	b.WriteString("yes")
	if b.Len() != size {
		t.Fatalf("composed a %d-byte paste, want %d", b.Len(), size)
	}
	return b.String()
}

func singleLineBurst(size int) string {
	return "d" + strings.Repeat("x", size-2) + "y"
}

func TestLazyResumeDiscard_BurstNeverConfirms(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	fx := setupLazyResumeDiscard(t)
	subject := discardSubjectTarget()
	fx.awaitScreenContains(t, subject, panelTitle)

	cases := []struct {
		name  string
		paste func(t *testing.T) string
	}{
		{"a 2,565-byte multi-line paste", func(t *testing.T) string { return multiLinePaste(t, 2565) }},
		{"a 4,005-byte multi-line paste", func(t *testing.T) string { return multiLinePaste(t, 4005) }},
		{"a 4,004-byte single-line burst", func(*testing.T) string { return singleLineBurst(4004) }},
	}
	for _, tc := range cases {
		t.Run(tc.name+" answers nothing", func(t *testing.T) {
			fx.awaitWaiterOnPanel(t)

			fx.pasteIntoSubject(t, tc.paste(t))

			fx.assertPasteAnsweredNothing(t, panelTitle)
		})
	}

	t.Run("a y leaves a registration rewritten while the pane waited", func(t *testing.T) {
		fx.awaitWaiterOnPanel(t)
		store := hooks.NewStore(fx.hooksPath)
		if err := store.Set(discardSubjectToken, hooks.EventOnResume,
			hooks.Registration{Command: burstReplacementCommand}, hooks.ViaCLI); err != nil {
			t.Fatalf("re-register the subject: %v", err)
		}
		rewritten := fx.readHooks(t)

		fx.sendKey(t, "d")
		fx.awaitScreenContains(t, subject, discardConfirmTitle)
		fx.assertScreenHas(t, subject, discardSubjectCommand)
		fx.sendKey(t, "y")

		cleared := harnesstest.PollUntil(t, restoretest.HydrateBudget, restoretest.HydrateTick, func() bool {
			value, err := fx.client.ReadPaneOption(subject, state.ResumePendingOption)
			return err == nil && !state.ResumePendingSet(value)
		})
		if !cleared {
			t.Fatalf("%s still set on the subject pane after y", state.ResumePendingOption)
		}
		fx.awaitScreen(t, subject, "the pane's own transcript with neither screen", func(screen string) bool {
			for _, piece := range discardScreenCopy {
				if strings.Contains(screen, piece) {
					return false
				}
			}
			return strings.Contains(screen, discardSubjectPreRebootLine)
		})

		if got := fx.readHooks(t); string(got) != string(rewritten) {
			t.Fatalf("hooks.json changed under a discard of a command it no longer held\nrewritten:\n%s\nnow:\n%s",
				rewritten, got)
		}
		portalLog := portaltest.ReadPortalLogSafe(fx.stateDir)
		if !strings.Contains(portalLog, "hydrate:") {
			t.Fatalf("portal.log carries no line from the chain, so it cannot show a discard's absence:\n%s", portalLog)
		}
		if strings.Contains(portalLog, "op=discard") {
			t.Fatalf("portal.log records a discard of a command the store no longer held:\n%s", portalLog)
		}
	})
}

func TestLazyResumePaste_NeverAnswersAWaitingPane(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	fx := setupLazyResumeDiscard(t)
	subject := discardSubjectTarget()
	fx.awaitScreenContains(t, subject, panelTitle)

	t.Run("a paste carrying line breaks leaves the panel waiting", func(t *testing.T) {
		fx.awaitWaiterOnPanel(t)

		fx.pasteIntoSubject(t, burstLine+burstLine)

		fx.assertPasteAnsweredNothing(t, panelTitle)
		if screen := fx.paneScreen(t, subject); strings.Contains(screen, burstDropReport) {
			t.Fatalf("the panel reports a drop no discard asked for; capture-pane -p:\n%s", screen)
		}
	})

	t.Run("a paste carrying y leaves the confirmation standing", func(t *testing.T) {
		fx.awaitWaiterOnPanel(t)
		fx.sendKey(t, "d")
		fx.awaitWaiterOn(t, "the confirmation", func(screen string) bool {
			return strings.Contains(screen, discardConfirmTitle)
		})

		fx.pasteIntoSubject(t, "yes\nyes\n")

		time.Sleep(restoretest.PaneReactionBudget)
		fx.assertScreenHas(t, subject, discardConfirmTitle)
		fx.assertUnanswered(t)

		fx.sendKey(t, "Escape")
		fx.awaitWaiterOnPanel(t)
	})

	t.Run("the rest of a paste the drain gave up on answers nothing", func(t *testing.T) {
		fx.awaitWaiterOnPanel(t)
		waiter := fx.waiterPID(t)

		fx.sendKey(t, "d")
		fx.awaitProcessRuns(t, waiter, resumeDrawArgv)
		fx.streamIntoSubject(t, strings.Repeat(burstLine, 4), streamedPasteSpan)

		fx.awaitScreenContains(t, subject, burstDropReport)
		fx.assertPasteAnsweredNothing(t, burstDropReport)
	})

	t.Run("an Enter typed after the pastes resumes the pane", func(t *testing.T) {
		fx.sendKey(t, "Enter")

		cleared := harnesstest.PollUntil(t, restoretest.HydrateBudget, restoretest.HydrateTick, func() bool {
			value, err := fx.client.ReadPaneOption(subject, state.ResumePendingOption)
			return err == nil && !state.ResumePendingSet(value)
		})
		if !cleared {
			t.Fatalf("%s still set on the subject pane after Enter", state.ResumePendingOption)
		}
		fired := harnesstest.PollUntil(t, restoretest.HydrateBudget, restoretest.HydrateTick, func() bool {
			_, err := os.Stat(filepath.Join(fx.workDir, discardSubjectFired))
			return err == nil
		})
		if !fired {
			t.Fatalf("the subject's command never ran after Enter")
		}
		fx.awaitScreen(t, subject, "the hook's output with no panel", func(screen string) bool {
			return strings.Contains(screen, "gone") && !strings.Contains(screen, panelTitle)
		})
		if transcript := fx.paneTranscript(t, subject); !strings.Contains(transcript, discardSubjectPreRebootLine) {
			t.Fatalf("the hook did not start over the pane's transcript; capture-pane -S -:\n%s", transcript)
		}
	})

	if err := os.Remove(filepath.Join(fx.workDir, discardSubjectFired)); err != nil {
		t.Fatalf("clear the subject's sentinel before the next reboot: %v", err)
	}
	fx.captureRound(t)
	fx.rebootRestoreHydrate(t)

	t.Run("a d and a y typed after a paste discard the registration", func(t *testing.T) {
		fx.awaitWaiterOnPanel(t)
		fx.pasteIntoSubject(t, burstLine+burstLine)
		fx.assertPasteAnsweredNothing(t, panelTitle)

		fx.sendKey(t, "d")
		fx.awaitWaiterOn(t, "the confirmation", func(screen string) bool {
			return strings.Contains(screen, discardConfirmTitle)
		})
		fx.sendKey(t, "y")

		cleared := harnesstest.PollUntil(t, restoretest.HydrateBudget, restoretest.HydrateTick, func() bool {
			value, err := fx.client.ReadPaneOption(subject, state.ResumePendingOption)
			return err == nil && !state.ResumePendingSet(value)
		})
		if !cleared {
			t.Fatalf("%s still set on the subject pane after y", state.ResumePendingOption)
		}
		if raw, present := decodeHookEntries(t, fx.readHooks(t))[discardSubjectToken]; present {
			t.Fatalf("hooks.json still carries the subject %q after the discard: %s", discardSubjectToken, raw)
		}
		time.Sleep(restoretest.PaneReactionBudget)
		if _, err := os.Stat(filepath.Join(fx.workDir, discardSubjectFired)); err == nil {
			t.Fatalf("the subject's registered command ran; a discard never runs it")
		}
	})
}

// streamedPasteSpan outlasts the discard's one-second input drain by a margin
// that leaves the drain still flushing when it gives up.
const streamedPasteSpan = 2500 * time.Millisecond

// assertPasteAnsweredNothing waits out the time a wrongly answered paste would
// take to act, then requires the subject still on its waiting panel, carrying
// the row named, with nothing the panel guards having moved. The row is the
// report where the panel carries one: input the tty echoed before the waiter
// took the pane can scroll the panel's title away.
func (fx *lazyDiscardFixture) assertPasteAnsweredNothing(t *testing.T, row string) {
	t.Helper()
	time.Sleep(restoretest.PaneReactionBudget)

	screen := fx.paneScreen(t, discardSubjectTarget())
	if !strings.Contains(screen, row) || strings.Contains(screen, discardConfirmTitle) {
		t.Fatalf("the pane left the waiting panel after the paste; capture-pane -p:\n%s", screen)
	}
	fx.assertUnanswered(t)
}

// assertUnanswered requires everything an answer would move to stand as the
// fixture seeded it: the store, the freeze, the subject's own command unrun, no
// pasted line run by a shell, and the waiter still holding the pane.
func (fx *lazyDiscardFixture) assertUnanswered(t *testing.T) {
	t.Helper()
	subject := discardSubjectTarget()
	if got := fx.readHooks(t); string(got) != string(fx.seededHooks) {
		t.Fatalf("hooks.json changed under the paste\nseeded:\n%s\nnow:\n%s", fx.seededHooks, got)
	}
	fx.assertPending(t, subject)
	if _, err := os.Stat(filepath.Join(fx.workDir, discardSubjectFired)); err == nil {
		t.Fatalf("the subject's registered command ran under the paste")
	}
	if _, err := os.Stat(filepath.Join(fx.workDir, burstSentinel)); err == nil {
		t.Fatalf("a shell ran a line of the paste")
	}
	tree := readProcessTree(t, fx.panePID(t, subject))
	if !treeRuns(tree, resumeWaitArgv) {
		t.Fatalf("the subject is no longer held by its waiter\n%s", formatTree(tree))
	}
}

// waiterPID is the process holding the subject between screens. Each screen's
// draw and wait replace its image in turn, so the pid names the chain's current
// step for as long as the pane waits.
func (fx *lazyDiscardFixture) waiterPID(t *testing.T) int {
	t.Helper()
	tree := readProcessTree(t, fx.panePID(t, discardSubjectTarget()))
	for _, p := range tree[1:] {
		if strings.Contains(p.command, resumeWaitArgv) {
			return p.pid
		}
	}
	t.Fatalf("the subject carries no waiter\n%s", formatTree(tree))
	return 0
}

// awaitProcessRuns polls one process at a tick far inside the draw's own life,
// which a discard's input drain holds open for at least its quiet window.
func (fx *lazyDiscardFixture) awaitProcessRuns(t *testing.T, pid int, argv string) {
	t.Helper()
	var seen []paneProcess
	ran := harnesstest.PollUntil(t, restoretest.HydrateBudget, time.Millisecond, func() bool {
		seen = describeProcesses(t, []int{pid})
		return len(seen) == 1 && strings.Contains(seen[0].command, argv)
	})
	if !ran {
		t.Fatalf("process %d never ran %s\n%s", pid, argv, formatTree(seen))
	}
}

// streamIntoSubject pastes chunk into the subject again and again for span,
// each paste landing well inside the drain's quiet window of the last, so the
// input keeps arriving for all of span.
func (fx *lazyDiscardFixture) streamIntoSubject(t *testing.T, chunk string, span time.Duration) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stream")
	if err := os.WriteFile(path, []byte(chunk), 0o600); err != nil {
		t.Fatalf("write the stream chunk: %v", err)
	}
	fx.ts.Run(t, "load-buffer", "-b", "stream", path)
	for deadline := time.Now().Add(span); time.Now().Before(deadline); {
		fx.ts.Run(t, "paste-buffer", "-b", "stream", "-t", string(discardSubjectTarget()))
	}
	fx.ts.Run(t, "delete-buffer", "-b", "stream")
}

// awaitWaiterOnPanel waits until the subject shows the waiting panel and the
// draw has handed the pane to its waiter.
func (fx *lazyDiscardFixture) awaitWaiterOnPanel(t *testing.T) {
	t.Helper()
	fx.awaitWaiterOn(t, "the waiting panel", func(screen string) bool {
		return strings.Contains(screen, panelTitle) && !strings.Contains(screen, discardConfirmTitle)
	})
}

// pasteIntoSubject delivers text the way a terminal paste reaches the pane:
// through a tmux buffer, whose line feeds tmux sends as carriage returns.
func (fx *lazyDiscardFixture) pasteIntoSubject(t *testing.T, text string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "paste")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("write the paste: %v", err)
	}
	fx.ts.Run(t, "load-buffer", "-b", "burst", path)
	fx.ts.Run(t, "paste-buffer", "-d", "-b", "burst", "-t", string(discardSubjectTarget()))
}

// awaitWaiterOn waits until the subject shows screen and the draw has handed
// the pane to its waiter.
func (fx *lazyDiscardFixture) awaitWaiterOn(t *testing.T, screen string, shows func(string) bool) {
	t.Helper()
	subject := discardSubjectTarget()
	fx.awaitScreen(t, subject, screen, shows)
	rootPID := fx.panePID(t, subject)
	var tree []paneProcess
	settled := harnesstest.PollUntil(t, restoretest.HydrateBudget, restoretest.HydrateTick, func() bool {
		tree = readProcessTree(t, rootPID)
		return treeRuns(tree, resumeWaitArgv) && !treeRuns(tree, resumeDrawArgv)
	})
	if !settled {
		t.Fatalf("the subject's waiter never settled on %s\n%s", screen, formatTree(tree))
	}
}
