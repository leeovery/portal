package sessionindex_test

import (
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
)

// unwritableStateDir is a state dir whose commit lock opens but in which no
// temp file can be created, so Update fails at the write.
func unwritableStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	f, err := commitlock.Acquire(dir)
	if err != nil {
		t.Fatalf("seed commit lock: %v", err)
	}
	_ = f.Close()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	return dir
}

func TestUpdate_Logging(t *testing.T) {
	t.Run("each landed change logs at INFO under its own op with via", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","label":"Old"},{"name":"b"}]}`)
		sink := logtest.Install(t)

		err := sessionindex.Update(dir, "tui", func(idx *sessionindex.Index) error {
			_ = idx.SetLabel("c", "Gamma")
			_ = idx.SetLabel("a", "")
			idx.Remove("b")
			return nil
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		for _, want := range []struct{ op, session, value string }{
			{"set-label", "c", "Gamma"},
			{"clear-label", "a", ""},
			{"rm", "b", ""},
		} {
			rec := sink.Records().Matching("sessions", want.op).Only(t, want.op)
			logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelInfo, Msg: want.op, Component: "sessions", Op: want.op, Via: "tui"})
			if got := rec.AttrString(t, "session"); got != want.session {
				t.Errorf("%s session = %q, want %q", want.op, got, want.session)
			}
			if got := rec.AttrOrEmpty("value"); got != want.value {
				t.Errorf("%s value = %q, want %q", want.op, got, want.value)
			}
		}
	})

	t.Run("a change-free update logs one DEBUG noop", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","label":"Alpha"}]}`)
		sink := logtest.Install(t)

		if err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error { return idx.SetLabel("a", "Alpha") }); err != nil {
			t.Fatalf("Update: %v", err)
		}
		rec := sink.Records().Only(t, "noop record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelDebug, Msg: "update-noop", Component: "sessions", Op: "update-noop", Via: "cli"})
	})

	t.Run("a change fn rolls back logs nothing", func(t *testing.T) {
		dir := t.TempDir()
		sink := logtest.Install(t)

		_ = sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
			_ = idx.SetLabel("a", "Alpha")
			return errors.New("boom")
		})
		if n := len(sink.Records()); n != 0 {
			t.Errorf("records = %d, want none", n)
		}
	})

	t.Run("a failed write logs each pending change at WARN with the error", func(t *testing.T) {
		dir := unwritableStateDir(t)
		sink := logtest.Install(t)

		err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
			idx.Remove("absent")
			if err := idx.SetLabel("a", "Alpha"); err != nil {
				return err
			}
			return idx.SetLabel("b", "Beta")
		})
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Fatalf("Update = %v, want a temp-create failure", err)
		}
		if n := len(sink.Records().AtOrAboveLevel(slog.LevelInfo)); n != 2 {
			t.Errorf("records = %d, want one per pending change", n)
		}
		for session, label := range map[string]string{"a": "Alpha", "b": "Beta"} {
			var rec logtest.Record
			for _, r := range sink.Records().Matching("sessions", "set-label") {
				if r.AttrOrEmpty("session") == session {
					rec = r
				}
			}
			logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelWarn, Msg: "set-label", Component: "sessions", Op: "set-label", Via: "cli"})
			if got := rec.AttrOrEmpty("value"); got != label {
				t.Errorf("value = %q, want %q", got, label)
			}
			logtest.AssertWriteFailure(t, rec, "write-failed-temp-create", fileutil.ErrWriteTempCreate)
		}
	})
}
