package state_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// The colon-free session the capture must still return in full, so a run that
// surfaced nothing at all cannot pass as a fix.
const plainSession = "plain"

func TestCaptureStructureRealTmuxUnaddressableSessionName(t *testing.T) {
	t.Run("it surfaces an unaddressable session in a real-tmux capture rather than dropping it", func(t *testing.T) {
		for _, unaddressable := range []string{colonSession, dollarSession} {
			t.Run(unaddressable, func(t *testing.T) {
				tmuxtest.SkipIfNoTmux(t)

				ts := tmuxtest.New(t, "ptl-unaddrsess-")
				client := ts.Client()
				if _, err := client.EnsureServer(); err != nil {
					t.Fatalf("EnsureServer: %v", err)
				}
				dir := t.TempDir()
				for _, name := range []string{unaddressable, plainSession} {
					ts.Run(t, "new-session", "-d", "-s", name, "-c", dir)
				}
				// Not WaitForSession: has-session composes the same target the
				// capture does and can never resolve this name. Enumeration
				// settles both names.
				waitForListedSessions(t, ts, unaddressable, plainSession)

				logger, sink := openTestLogger(t, t.TempDir())
				idx, _, err := state.CaptureStructure(client, nil, nil, logger)
				if err != nil {
					t.Fatalf("CaptureStructure: %v", err)
				}

				if !capturedSession(idx, plainSession) {
					t.Fatalf("precondition: %q must be captured; Sessions = %+v", plainSession, idx.Sessions)
				}

				log := sink.Body()
				// A tmux that can address the name captures it, which is the
				// better outcome and leaves nothing to classify.
				if capturedSession(idx, unaddressable) {
					return
				}
				if !strings.Contains(log, "capture anomalous session error") {
					t.Errorf("%q is absent from the capture and was not reported as anomalous; log:\n%s", unaddressable, log)
				}
				if strings.Contains(log, "vanished") {
					t.Errorf("a live %q must never be counted as natural churn; log:\n%s", unaddressable, log)
				}
			})
		}
	})
}

func waitForListedSessions(t *testing.T, ts *tmuxtest.Socket, names ...string) {
	t.Helper()
	listed := func() bool {
		out, err := ts.TryRun("list-sessions", "-F", "#{session_name}")
		if err != nil {
			return false
		}
		for _, name := range names {
			if !slices.Contains(strings.Split(strings.TrimSpace(out), "\n"), name) {
				return false
			}
		}
		return true
	}
	if !harnesstest.PollUntil(t, 2*time.Second, 20*time.Millisecond, listed) {
		t.Fatalf("sessions %v did not all appear within 2s", names)
	}
}

func capturedSession(idx state.Index, name string) bool {
	for _, s := range idx.Sessions {
		if s.Name == name {
			return true
		}
	}
	return false
}
