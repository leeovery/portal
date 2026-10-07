package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

const movedOccupantToken = "tokbbb"

// seedContestedLayout commits "work" with waiting pane X saved at 2.0, pane Y
// saved at 3.0 carrying occupantToken ("" for none) and an unmoved pane at 5.0,
// each on its positional file.
func seedContestedLayout(t *testing.T, dir, occupantToken string) state.Index {
	t.Helper()
	if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
		t.Fatalf("create scrollback dir: %v", err)
	}
	panes := []struct {
		window int
		token  string
	}{{2, waitingToken}, {3, occupantToken}, {5, ""}}
	idx := state.Index{Version: state.SchemaVersion, Sessions: []state.Session{{Name: "work", Environment: map[string]string{}}}}
	for _, p := range panes {
		key := state.SanitizePaneKey("work", p.window, 0)
		if err := os.WriteFile(state.ScrollbackFile(dir, key), []byte("saved-"+key), 0o600); err != nil {
			t.Fatalf("seed scrollback %s: %v", key, err)
		}
		idx.Sessions[0].Windows = append(idx.Sessions[0].Windows, state.Window{
			Index: p.window, Name: "main", Layout: "layout",
			Panes: []state.Pane{{Index: 0, CWD: "/tmp", CurrentCommand: "zsh", ScrollbackFile: "scrollback/" + key + ".bin", PortalPaneID: p.token}},
		})
	}
	if err := state.Commit(dir, idx, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	idx.Canonicalize()
	return idx
}

func TestDaemonTickCancelledAfterAContestedWriteThenAStoodDownFlushKeepsEveryRecordsTranscript(t *testing.T) {
	occupants := []struct {
		name  string
		token string
		// assertTick checks what the cancelled tick left of Y's capture.
		assertTick func(t *testing.T, dir string)
	}{
		{"Y carries a token", movedOccupantToken, func(t *testing.T, dir string) {
			if got := scrollbackBody(t, dir, "pane-"+movedOccupantToken+".bin"); got != "y-capture" {
				t.Fatalf("Y's token-named transcript after the cancelled tick = %q; want Y's capture written before the cancellation", got)
			}
		}},
		{"Y carries no token", "", func(t *testing.T, dir string) {
			if got, want := scrollbackBody(t, dir, "work__2.0.bin"), "saved-"+state.SanitizePaneKey("work", 2, 0); got != want {
				t.Fatalf("work__2.0.bin after the cancelled tick = %q; want X's saved transcript, Y's capture deferred", got)
			}
		}},
	}
	for _, occ := range occupants {
		t.Run(occ.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PORTAL_STATE_DIR", dir)
			sink := logtest.Install(t)
			seed := seedContestedLayout(t, dir, occ.token)
			before, err := os.ReadFile(state.SessionsJSON(dir))
			if err != nil {
				t.Fatalf("read sessions.json: %v", err)
			}

			fc := &daemonFakeCommander{
				sessionsOut: "work|3|0|",
				panesOut: daemonPaneRow(1, 0, true, waitingToken) + "\n" +
					daemonPaneRow(2, 0, false, occ.token) + "\n" +
					daemonPaneRow(5, 0, false, ""),
				captureByTarget: map[string]string{"work:2.0": "y-capture", "work:5.0": "unmoved-capture"},
			}
			deps := makeCaptureDeps(t, dir, fc)
			deps.PrevIndex = &seed
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fc.dispatchHook = func(args []string) {
				if args[0] == "capture-pane" && len(args) >= 7 && sessionFromExactTarget(args[6]) == "work:2.0" {
					cancel()
				}
			}

			touchSaveRequested(t, dir)

			tick(ctx, deps)

			occ.assertTick(t, dir)
			if n := len(fc.callsContaining("capture-pane")); n != 1 {
				t.Fatalf("capture-pane calls = %d, want 1: the tick was not cancelled at the pane after Y", n)
			}

			fc.dispatchHook = nil
			fc.confirmErr = &tmux.CommandError{Args: []string{"display-message"}, Stderr: "no server running", Err: errors.New("exit status 1")}
			if err := defaultShutdownFlush(deps); err != nil {
				t.Fatalf("defaultShutdownFlush: %v", err)
			}

			rec := sink.Records().Matching("daemon", "shutdown").Only(t, "daemon shutdown record")
			if got := rec.AttrOrEmpty("flush_completed"); got != "false" {
				t.Fatalf("flush_completed = %q, want \"false\" from a flush that stood down", got)
			}
			after, err := os.ReadFile(state.SessionsJSON(dir))
			if err != nil {
				t.Fatalf("read sessions.json: %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Errorf("sessions.json changed by a tick that was cancelled and a flush that stood down")
			}
			for _, window := range []int{2, 3} {
				named := recordedScrollback(t, dir, window, 0)
				data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(named)))
				if err != nil {
					t.Fatalf("read the transcript named for work:%d.0 at %q: %v", window, named, err)
				}
				if want := "saved-" + state.SanitizePaneKey("work", window, 0); string(data) != want {
					t.Errorf("the record for work:%d.0 names %q holding %q, want %q", window, named, data, want)
				}
			}
			assertEverySavedScrollbackOnDisk(t, dir)
		})
	}
}
