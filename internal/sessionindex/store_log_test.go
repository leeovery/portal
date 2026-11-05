package sessionindex_test

import (
	"errors"
	"log/slog"
	"maps"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
)

// ofSession is the records naming session, as one op's records for several
// sessions are told apart.
func ofSession(t *testing.T, recs logtest.Records, session string) logtest.Records {
	t.Helper()
	var named logtest.Records
	for _, rec := range recs {
		if rec.AttrString(t, "session") == session {
			named = append(named, rec)
		}
	}
	return named
}

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
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","label":"Old"},{"name":"b"},{"name":"e","created_at":"2026-11-02T08:00:00Z"},{"name":"f","label":"F"},{"name":"g"}]}`)
		sink := logtest.Install(t)

		_, err := sessionindex.Update(dir, "tui", func(idx *sessionindex.Index) error {
			_ = idx.SetLabel("c", "Gamma")
			_ = idx.SetLabel("a", "")
			idx.Remove("b")
			claimFor(t, idx, "d", "/d", liveCreation, 0)
			claimFor(t, idx, "f", "/f", liveCreation, 4242)
			idx.Restored(sessionindex.Entry{Name: "e", CreatedAt: time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)}, time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC), 4242)
			idx.SetScrollback("e", "scrollback/e.bin")
			idx.RecordCwd("e", "/work/e")
			idx.Owe("g")
			return nil
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		for _, want := range []struct {
			op, session, value string
			level              slog.Level
		}{
			{"set-label", "c", "Gamma", slog.LevelInfo},
			{"clear-label", "a", "", slog.LevelInfo},
			{"rm", "b", "", slog.LevelInfo},
			{"claim", "d", "/d", slog.LevelInfo},
			{"adopt", "f", "2026-11-04T10:00:00Z", slog.LevelInfo},
			{"restored", "e", "2026-11-04T09:00:00Z", slog.LevelInfo},
			{"set-scrollback", "e", "scrollback/e.bin", slog.LevelInfo},
			{"record-cwd", "e", "/work/e", slog.LevelDebug},
			{"set-owed", "g", "", slog.LevelInfo},
		} {
			rec := ofSession(t, sink.Records().Matching("sessions", want.op), want.session).Only(t, want.op)
			logtest.AssertRecord(t, rec, logtest.RecordWant{Level: want.level, Msg: want.op, Component: "sessions", Op: want.op, Via: "tui"})
			if got := rec.AttrString(t, "session"); got != want.session {
				t.Errorf("%s session = %q, want %q", want.op, got, want.session)
			}
			if got := rec.AttrOrEmpty("value"); got != want.value {
				t.Errorf("%s value = %q, want %q", want.op, got, want.value)
			}
		}
	})

	t.Run("an owed mark cleared logs at INFO, saying why", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","created_at":"2026-11-04T10:00:00Z","pid":4242,"owed":true},{"name":"b","created_at":"2026-11-04T09:00:00Z","pid":1717,"owed":true}]}`)
		sink := logtest.Install(t)

		if _, err := sessionindex.Update(dir, "internal", func(idx *sessionindex.Index) error {
			claimFor(t, idx, "a", "", liveCreation, 4242)
			idx.Hydrated("b", time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC), 1717)
			return nil
		}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		cleared := sink.Records().Matching("sessions", "clear-owed")
		why := map[string]string{}
		for _, rec := range cleared {
			logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelInfo, Msg: "clear-owed", Component: "sessions", Op: "clear-owed", Via: "internal"})
			why[rec.AttrString(t, "session")] = rec.AttrString(t, "detail")
		}
		if want := map[string]string{"a": "its session is live", "b": "its helper finished its restore"}; !maps.Equal(why, want) {
			t.Errorf("clear-owed records = %v, want %v", why, want)
		}
	})

	t.Run("a restore names the pid and the creation it took the place of", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","label":"Alpha","created_at":"2026-11-02T08:00:00Z","pid":1717}]}`)
		sink := logtest.Install(t)

		if _, err := sessionindex.Update(dir, "internal", func(idx *sessionindex.Index) error {
			idx.Restored(sessionindex.Entry{Name: "a", CreatedAt: time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC), PID: 1717}, time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC), 4242)
			return nil
		}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		detail := sink.Records().Matching("sessions", "restored").Only(t, "restored record").AttrString(t, "detail")
		for _, want := range []string{"pid 4242", "2026-11-02T08:00:00Z"} {
			if !strings.Contains(detail, want) {
				t.Errorf("detail = %q, want it to name %s", detail, want)
			}
		}
	})

	for _, tc := range []struct {
		name, held string
		want       []string
	}{
		{"saved before this boot", `"created_at":"2026-11-03T06:00:00Z","pid":1717`, []string{"it was saved before this boot", "pid 4242", "the creation of 2026-11-03T06:00:00Z"}},
		{"whose creation was never recorded", `"label":"Alpha"`, []string{"its creation was never recorded", "pid 4242", "no recorded creation"}},
	} {
		t.Run("an adopt of an entry "+tc.name+" says why, naming the pid and the creation it took the place of", func(t *testing.T) {
			dir := t.TempDir()
			writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a",`+tc.held+`}]}`)
			sink := logtest.Install(t)

			if _, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
				claimFor(t, idx, "a", "/a", liveCreation, 4242)
				return nil
			}); err != nil {
				t.Fatalf("Update: %v", err)
			}
			detail := sink.Records().Matching("sessions", "adopt").Only(t, "adopt record").AttrString(t, "detail")
			for _, want := range tc.want {
				if !strings.Contains(detail, want) {
					t.Errorf("detail = %q, want it to name %s", detail, want)
				}
			}
		})
	}

	t.Run("a change-free update logs one DEBUG noop", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","label":"Alpha"}]}`)
		sink := logtest.Install(t)

		if _, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error { return idx.SetLabel("a", "Alpha") }); err != nil {
			t.Fatalf("Update: %v", err)
		}
		rec := sink.Records().Only(t, "noop record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelDebug, Msg: "update-noop", Component: "sessions", Op: "update-noop", Via: "cli"})
	})

	for _, tc := range []struct {
		name    string
		body    string
		created time.Time
		refused error
	}{
		{
			name:    "the same creation, its dir recorded",
			body:    `{"version":2,"sessions":[{"name":"a","label":"Alpha","dir":"/kept","created_at":"2026-11-03T09:30:00Z"}]}`,
			created: time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC),
		},
		{
			name:    "no creation time, and nothing to drop",
			body:    `{"version":2,"sessions":[{"name":"a","dir":"/a"}]}`,
			refused: sessionindex.ErrNoCreation,
		},
		{
			name:    "no creation time over an entry of this boot",
			body:    `{"version":2,"sessions":[{"name":"a","label":"Alpha","created_at":"2026-11-04T09:00:00Z","pid":1717}]}`,
			refused: sessionindex.ErrNoCreation,
		},
	} {
		t.Run("a claim of "+tc.name+" changes nothing and logs one DEBUG noop", func(t *testing.T) {
			dir := t.TempDir()
			writeIndex(t, dir, tc.body)
			sink := logtest.Install(t)

			if _, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
				if _, err := idx.Claim("a", "/a", tc.created, 0, ruleRestoring()); !errors.Is(err, tc.refused) {
					t.Errorf("Claim = %v, want %v", err, tc.refused)
				}
				return nil
			}); err != nil {
				t.Fatalf("Update: %v", err)
			}
			rec := sink.Records().Only(t, "noop record")
			logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelDebug, Msg: "update-noop", Component: "sessions", Op: "update-noop", Via: "cli"})
			if got := string(readFile(t, dir)); got != tc.body {
				t.Errorf("sessions.json = %s, want it untouched", got)
			}
		})
	}

	t.Run("a claim replacing an earlier creation's entry logs replace, naming the label and scrollback it dropped", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","label":"Old work","dir":"/old","created_at":"2026-11-04T09:00:00Z","scrollback_file":"scrollback/a.bin"}]}`)
		sink := logtest.Install(t)

		if _, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
			claimFor(t, idx, "a", "/a", liveCreation, 0)
			return nil
		}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		rec := sink.Records().Matching("sessions", "replace").Only(t, "replace record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelInfo, Msg: "replace", Component: "sessions", Op: "replace", Via: "cli"})
		detail := rec.AttrString(t, "detail")
		for _, want := range []string{"earlier creation", "2026-11-04T09:00:00Z", `"Old work"`, `"scrollback/a.bin"`} {
			if !strings.Contains(detail, want) {
				t.Errorf("detail = %q, want it to name %s", detail, want)
			}
		}
		if n := len(sink.Records().Matching("sessions", "claim")); n != 0 {
			t.Errorf("claim records = %d, want the replacement logged as replace alone", n)
		}
	})

	t.Run("a change fn rolls back logs nothing", func(t *testing.T) {
		dir := t.TempDir()
		sink := logtest.Install(t)

		_, _ = sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
			_ = idx.SetLabel("a", "Alpha")
			return errors.New("boom")
		})
		if n := len(sink.Records()); n != 0 {
			t.Errorf("records = %d, want none", n)
		}
	})

	t.Run("a write whose directory sync fails has landed: its changes log as written, then one WARN unsynced", func(t *testing.T) {
		dir := t.TempDir()
		seedIndex(t, dir, sessionindex.Entry{Name: "gone"})
		fileutil.FailDirSyncForTest(t, syscall.EIO)
		sink := logtest.Install(t)

		outcome, err := sessionindex.Update(dir, "tui", func(idx *sessionindex.Index) error {
			idx.Remove("gone")
			return idx.SetLabel("a", "Alpha")
		})
		if err != nil {
			t.Fatalf("Update = %v, want the landed write to succeed", err)
		}

		if !sameEntries(outcome.Removed, []sessionindex.Entry{{Name: "gone"}}) {
			t.Errorf("removed = %+v, want gone, as a landed write reports", outcome.Removed)
		}
		for _, op := range []string{"rm", "set-label"} {
			sink.Records().Matching("sessions", op).AtExactLevel(slog.LevelInfo).Only(t, op+" as written")
		}
		rec := sink.Records().Matching("sessions", "unsynced").Only(t, "the unsynced warning")
		logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelWarn, Msg: "unsynced", Component: "sessions", Op: "unsynced", Via: "tui"})
		logtest.AssertWriteFailure(t, rec, "write-failed-fsync", fileutil.ErrWriteFsync)
		if idx, _ := sessionindex.Read(dir); idx.Len() != 1 {
			t.Errorf("sessions = %+v, want a alone", idx.Entries())
		}
	})

	t.Run("a failed write logs each pending change at WARN with the error", func(t *testing.T) {
		dir := unwritableStateDir(t)
		sink := logtest.Install(t)

		_, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
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
