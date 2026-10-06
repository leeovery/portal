package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

const emptyCaptureRefused = "empty capture not confirmed; saved transcript kept"

// daemonDumper runs one cycle that dumps scrollback against fc, logging through
// logger.
type daemonDumper struct {
	name string
	run  func(t *testing.T, saved savedStateFixture, fc *daemonFakeCommander, logger *slog.Logger)
}

var daemonDumpers = []daemonDumper{
	{"the daemon's tick", func(t *testing.T, saved savedStateFixture, fc *daemonFakeCommander, logger *slog.Logger) {
		deps := makeDeps(t, saved.dir, fc)
		deps.PrevIndex = &saved.index
		deps.Logger = logger
		tick(t.Context(), deps)
	}},
	{"the daemon's shutdown flush", func(t *testing.T, saved savedStateFixture, fc *daemonFakeCommander, logger *slog.Logger) {
		deps := makeDeps(t, saved.dir, fc)
		deps.PrevIndex = &saved.index
		deps.Logger = logger
		if err := defaultShutdownFlush(deps); err != nil {
			t.Fatalf("defaultShutdownFlush: %v", err)
		}
	}},
}

// emptyCaptureThen answers the cycle's capture with "work" alone live and its
// pane captured empty, then applies answerAfter to every read sent after that
// capture.
func emptyCaptureThen(answerAfter func(fc *daemonFakeCommander)) *daemonFakeCommander {
	fc := workOnlyCommander(nil)
	fc.dispatchHook = func(args []string) {
		if args[0] == "capture-pane" {
			answerAfter(fc)
		}
	}
	return fc
}

func workPaneKey() string { return state.SanitizePaneKey("work", 0, 0) }

func readWorkTranscript(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(state.ScrollbackFile(dir, workPaneKey()))
	if err != nil {
		t.Fatalf("read work transcript: %v", err)
	}
	return data
}

func assertEmptyCaptureRefusedLine(t *testing.T, sink *logtest.Sink) {
	t.Helper()
	line := sink.Records().Matching("daemon", emptyCaptureRefused).AtOrAboveLevel(slog.LevelInfo).Only(t, "refused empty write line")
	if want := []string{"component", "pane_key", "error"}; !slices.Equal(line.Keys, want) {
		t.Errorf("refused empty write line keys = %v, want %v", line.Keys, want)
	}
	if got := line.AttrString(t, "pane_key"); got != workPaneKey() {
		t.Errorf("refused empty write pane_key = %q, want %q", got, workPaneKey())
	}
}

func TestDaemonDumpKeepsASavedTranscriptOverAnUnconfirmedEmptyCapture(t *testing.T) {
	unconfirmed := []struct {
		name        string
		answerAfter func(fc *daemonFakeCommander)
	}{
		{"the read after the capture is refused", func(fc *daemonFakeCommander) { fc.confirmErr = refusedConfirmation() }},
		{"the read after the capture is answered by another server on the socket", func(fc *daemonFakeCommander) { fc.answeringPID = fakeOwnServerPID + 1 }},
		{"the read after the capture is answered naming no server", func(fc *daemonFakeCommander) { fc.silentConfirm = true }},
	}
	for _, d := range daemonDumpers {
		for _, u := range unconfirmed {
			t.Run(d.name+"/"+u.name, func(t *testing.T) {
				saved := seedSavedState(t)
				before := readWorkTranscript(t, saved.dir)
				logger, sink := newCaptureLoggerForComponent(t, "daemon")

				d.run(t, saved, emptyCaptureThen(u.answerAfter), logger)

				if got := readWorkTranscript(t, saved.dir); string(got) != string(before) {
					t.Errorf("work transcript = %q, want unchanged %q", got, before)
				}
				assertEmptyCaptureRefusedLine(t, sink)
			})
		}
	}
}

func TestDaemonDumpWritesAnEmptyCaptureItsOwnServerConfirms(t *testing.T) {
	for _, d := range daemonDumpers {
		t.Run(d.name, func(t *testing.T) {
			saved := seedSavedState(t)
			logger, sink := newCaptureLoggerForComponent(t, "daemon")
			fc := emptyCaptureThen(func(*daemonFakeCommander) {})

			d.run(t, saved, fc, logger)

			if got := readWorkTranscript(t, saved.dir); len(got) != 0 {
				t.Errorf("work transcript = %q, want the confirmed empty capture", got)
			}
			if got := sink.Records().WithMessage(emptyCaptureRefused); len(got) != 0 {
				t.Errorf("refused empty write lines = %d, want none", len(got))
			}
			if got := len(fc.callsContaining("display-message")); got != 2 {
				t.Errorf("confirmation reads = %d, want the cycle's and the empty capture's", got)
			}
		})
	}
}

func TestDaemonDumpTalliesARefusedEmptyWriteOnceAsAnomalous(t *testing.T) {
	for _, d := range daemonDumpers {
		t.Run(d.name, func(t *testing.T) {
			saved := seedSavedState(t)
			sink := logtest.Install(t)
			fc := emptyCaptureThen(func(fc *daemonFakeCommander) { fc.answeringPID = fakeOwnServerPID + 1 })

			d.run(t, saved, fc, daemonLogger)

			summary := sink.Records().Matching("capture", "tick complete").Only(t, "tick complete line")
			if got := summary.IntAttr(t, "anomalous"); got != 1 {
				t.Errorf("anomalous = %d, want 1", got)
			}
			sink.Records().Matching("daemon", emptyCaptureRefused).AtExactLevel(slog.LevelWarn).Only(t, "refused empty write line")
			if got := sink.Records().WithMessage("write scrollback failed"); len(got) != 0 {
				t.Errorf("write scrollback failed lines = %d, want none", len(got))
			}
		})
	}
}

func TestDaemonDumpLogsAFailedWriteWithoutTheRefusalLine(t *testing.T) {
	for _, d := range daemonDumpers {
		t.Run(d.name, func(t *testing.T) {
			saved := seedSavedState(t)
			sink := logtest.Install(t)
			fc := workOnlyCommander(nil)
			fc.captureByTarget = map[string]string{"work:0.0": "fresh scrollback"}
			blockWorkTranscript(t, saved.dir)

			d.run(t, saved, fc, daemonLogger)

			warn := sink.Records().Matching("daemon", "write scrollback failed").AtExactLevel(slog.LevelWarn).Only(t, "write scrollback failed line")
			if want := []string{"component", "pane_key", "error"}; !slices.Equal(warn.Keys, want) {
				t.Errorf("write scrollback failed keys = %v, want %v", warn.Keys, want)
			}
			if got := warn.AttrString(t, "pane_key"); got != workPaneKey() {
				t.Errorf("write scrollback failed pane_key = %q, want %q", got, workPaneKey())
			}
			if got := sink.Records().WithMessage(emptyCaptureRefused); len(got) != 0 {
				t.Errorf("refused empty write lines = %d, want none", len(got))
			}
			summary := sink.Records().Matching("capture", "tick complete").Only(t, "tick complete line")
			if got := summary.IntAttr(t, "anomalous"); got != 1 {
				t.Errorf("anomalous = %d, want 1", got)
			}
		})
	}
}

// blockWorkTranscript puts a non-empty directory at the work pane's transcript
// path, so a write's rename over it fails.
func blockWorkTranscript(t *testing.T, dir string) {
	t.Helper()
	path := state.ScrollbackFile(dir, workPaneKey())
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove work transcript: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(path, "occupant"), 0o700); err != nil {
		t.Fatalf("block work transcript: %v", err)
	}
}
