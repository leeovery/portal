package cmd

import (
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

const (
	absentIndexWarn     = "sessions.json absent; committing without the saved index"
	unreadableIndexWarn = "read sessions.json failed; committing without the saved index"
)

func TestDaemonTick_ReportsASessionsJSONItCouldNotReadAndReplacesIt(t *testing.T) {
	cases := []struct {
		name        string
		stage       func(t *testing.T, dir string)
		want, other string
	}{
		{name: "absent", stage: func(*testing.T, string) {}, want: absentIndexWarn, other: unreadableIndexWarn},
		{name: "hand-broken", stage: func(t *testing.T, dir string) {
			if err := os.WriteFile(state.SessionsJSON(dir), []byte("{not valid json"), 0o600); err != nil {
				t.Fatalf("seed undecodable sessions.json: %v", err)
			}
		}, want: unreadableIndexWarn, other: absentIndexWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PORTAL_STATE_DIR", dir)
			sink := logtest.Install(t)
			sess, panes := oneSession()
			deps := makeDeps(t, dir, &daemonFakeCommander{sessionsOut: sess, panesOut: panes})
			deps.Logger = daemonLogger
			deps.PrevIndex = sentinelIndex("remembered")
			tc.stage(t, dir)

			if err := captureAndCommit(t.Context(), deps); err != nil {
				t.Fatalf("captureAndCommit: %v", err)
			}

			rec := sink.Records().Matching("daemon", tc.want).AtExactLevel(slog.LevelWarn).Only(t, tc.want)
			if tc.want == unreadableIndexWarn {
				if cause := rec.ErrorAttr(t, "error"); !errors.Is(cause, state.ErrCorruptIndex) {
					t.Errorf("error = %v, want the decode failure", cause)
				}
			}
			if n := len(sink.Records().WithMessage(tc.other)); n != 0 {
				t.Errorf("%q logged %d times, want 0", tc.other, n)
			}
			committed, _, err := state.ReadIndex(dir)
			if err != nil {
				t.Fatalf("tick did not replace sessions.json: %v", err)
			}
			if len(committed.Sessions) != 1 || committed.Sessions[0].Name != "work" {
				t.Errorf("committed sessions = %+v, want the captured [work]", committed.Sessions)
			}
		})
	}
}

func TestDaemonTick_OverAReadableSessionsJSONReportsNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := logtest.Install(t)
	sess, panes := oneSession()
	deps := makeDeps(t, dir, &daemonFakeCommander{sessionsOut: sess, panesOut: panes})
	deps.Logger = daemonLogger
	if err := state.Commit(dir, state.Index{Version: state.SchemaVersion}, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}

	if err := captureAndCommit(t.Context(), deps); err != nil {
		t.Fatalf("captureAndCommit: %v", err)
	}

	for _, msg := range []string{absentIndexWarn, unreadableIndexWarn} {
		if n := len(sink.Records().WithMessage(msg)); n != 0 {
			t.Errorf("%q logged %d times, want 0", msg, n)
		}
	}
}
