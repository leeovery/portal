package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

const waitingTranscript = "waiting-transcript"

// seedWaitingPaneSaved commits "work" with a live pane at 0.0 and, at 0.1, a
// pane whose saved record still names its positional transcript.
func seedWaitingPaneSaved(t *testing.T, dir string) state.Index {
	t.Helper()
	if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
		t.Fatalf("create scrollback dir: %v", err)
	}
	for file, body := range map[string]string{"work__0.0.bin": "live-body", "work__0.1.bin": waitingTranscript} {
		if err := os.WriteFile(filepath.Join(state.ScrollbackDir(dir), file), []byte(body), 0o600); err != nil {
			t.Fatalf("seed scrollback %s: %v", file, err)
		}
	}
	idx := state.Index{Version: state.SchemaVersion, Sessions: []state.Session{{
		Name:        "work",
		Environment: map[string]string{},
		Windows: []state.Window{{
			Index: 0, Name: "main", Layout: "layout", Active: true,
			Panes: []state.Pane{
				{Index: 0, CWD: "/tmp", Active: true, CurrentCommand: "zsh", ScrollbackFile: "scrollback/work__0.0.bin"},
				{Index: 1, CWD: "/tmp", CurrentCommand: "zsh", ScrollbackFile: "scrollback/work__0.1.bin", PortalPaneID: waitingToken},
			},
		}},
	}}}
	if err := state.Commit(dir, idx, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	idx.Canonicalize()
	return idx
}

func assertEverySavedScrollbackOnDisk(t *testing.T, dir string) {
	t.Helper()
	for _, s := range readSessionsJSON(t, dir).Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.ScrollbackFile == "" {
					continue
				}
				if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p.ScrollbackFile))); err != nil {
					t.Errorf("sessions.json names %q for %s:%d.%d, which is not on disk: %v", p.ScrollbackFile, s.Name, w.Index, p.Index, err)
				}
			}
		}
	}
}

func TestDaemonTickCancelledAfterTheRefileThenAStoodDownFlushLeavesEverySavedScrollbackOnDisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := logtest.Install(t)
	seed := seedWaitingPaneSaved(t, dir)

	fc := &daemonFakeCommander{
		sessionsOut:     "work|1|0|",
		panesOut:        daemonPaneRow(0, 0, false, "") + "\n" + daemonPaneRow(0, 1, true, waitingToken),
		captureByTarget: map[string]string{"work:0.0": "live-body"},
	}
	deps := makeCaptureDeps(t, dir, fc)
	deps.PrevIndex = &seed
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fc.dispatchHook = func(args []string) {
		if args[0] == "capture-pane" {
			cancel()
		}
	}
	touchSaveRequested(t, dir)

	tick(ctx, deps)

	if got := scrollbackBody(t, dir, waitingRefiled); got != waitingTranscript {
		t.Fatalf("token-named file after the cancelled tick = %q, want %q", got, waitingTranscript)
	}
	if len(fc.callsContaining("capture-pane")) == 0 {
		t.Fatal("the tick dumped nothing; it was not cancelled mid-dump")
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
	assertEverySavedScrollbackOnDisk(t, dir)
	named := recordedScrollback(t, dir, 0, 1)
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(named)))
	if err != nil {
		t.Fatalf("read the waiting pane's transcript at %q: %v", named, err)
	}
	if string(data) != waitingTranscript {
		t.Errorf("the waiting pane's record names %q holding %q, want %q", named, data, waitingTranscript)
	}
}
