package state_test

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// The names at the heart of the case: tmux accepts each one, Portal's target
// form cannot address it, and tmux reports that failure in the words it uses for
// a session that no longer exists.
const (
	colonSession  = "a:b"
	dollarSession = "$foo"
)

func TestCaptureStructureUnaddressableSessionName(t *testing.T) {
	t.Run("it classifies an unaddressable session name as anomalous rather than natural churn", func(t *testing.T) {
		for _, unaddressable := range []string{colonSession, dollarSession} {
			t.Run(unaddressable, func(t *testing.T) {
				logger, sink := openTestLogger(t, t.TempDir())
				mock := &captureMock{
					listSessions: listSessionsFor(unaddressable, "plain"),
					listPanes: strings.Join([]string{
						paneLine(unaddressable, 0, "m", "L", false, true, 0, "/a", true, "zsh"),
						paneLine("plain", 0, "m", "L", false, true, 0, "/p", true, "zsh"),
					}, "\n"),
					envErrs: map[string]error{
						unaddressable: noSuchSessionErr("=" + unaddressable),
					},
					t: t,
				}
				client := tmux.NewClient(mock.commander())

				idx, _, err := state.CaptureStructure(client, nil, nil, logger)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "plain" {
					t.Fatalf("Sessions = %+v, want only [plain]", idx.Sessions)
				}

				log := sink.Body()
				if !strings.Contains(log, "capture anomalous session error") {
					t.Errorf("expected the anomalous WARN for %q; log:\n%s", unaddressable, log)
				}
				if strings.Contains(log, "vanished") {
					t.Errorf("a live %q must not be reported as vanished; log:\n%s", unaddressable, log)
				}
			})
		}
	})
}
