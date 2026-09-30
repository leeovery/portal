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

			fx.awaitScreen(t, subject, "the confirmation or the waiting panel reporting the drop", func(screen string) bool {
				return strings.Contains(screen, discardConfirmTitle) || strings.Contains(screen, burstDropReport)
			})
			// Anything the paste wrongly answered lands well inside this.
			time.Sleep(restoretest.PaneReactionBudget)

			screen := fx.paneScreen(t, subject)
			if !strings.Contains(screen, discardConfirmTitle) && !strings.Contains(screen, burstDropReport) {
				t.Fatalf("the pane left both screens after the paste; capture-pane -p:\n%s", screen)
			}
			if got := fx.readHooks(t); string(got) != string(fx.seededHooks) {
				t.Fatalf("hooks.json changed under the paste\nseeded:\n%s\nnow:\n%s", fx.seededHooks, got)
			}
			fx.assertPending(t, subject)
			if _, err := os.Stat(filepath.Join(fx.workDir, burstSentinel)); err == nil {
				t.Fatalf("a shell ran a line of the paste")
			}
			tree := readProcessTree(t, fx.panePID(t, subject))
			if !treeRuns(tree, resumeWaitArgv) {
				t.Fatalf("the subject is no longer held by its waiter\n%s", formatTree(tree))
			}

			if strings.Contains(screen, discardConfirmTitle) {
				fx.sendKey(t, "Escape")
			}
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

// awaitWaiterOnPanel waits until the subject shows the waiting panel and the
// draw has handed the pane to its waiter.
func (fx *lazyDiscardFixture) awaitWaiterOnPanel(t *testing.T) {
	t.Helper()
	subject := discardSubjectTarget()
	fx.awaitScreen(t, subject, "the waiting panel", func(screen string) bool {
		return strings.Contains(screen, panelTitle) && !strings.Contains(screen, discardConfirmTitle)
	})
	rootPID := fx.panePID(t, subject)
	var tree []paneProcess
	settled := harnesstest.PollUntil(t, restoretest.HydrateBudget, restoretest.HydrateTick, func() bool {
		tree = readProcessTree(t, rootPID)
		return treeRuns(tree, resumeWaitArgv) && !treeRuns(tree, resumeDrawArgv)
	})
	if !settled {
		t.Fatalf("the subject's waiter never settled on the panel\n%s", formatTree(tree))
	}
}
