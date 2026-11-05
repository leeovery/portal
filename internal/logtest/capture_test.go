package logtest_test

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
)

func TestNewCaptureLogger_RendersLevelMessageKeyValue(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)

	logger.Info("hello world", "session", "demo", "count", 3)

	if got, want := sink.Body(), "INFO hello world session=demo count=3"; got != want {
		t.Errorf("Body() = %q, want %q", got, want)
	}
}

func TestSink_RecordsLevelMessageAndOrderedKeys(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)

	logger.Warn("geometry complete", "panes", 4, "took", "5ms", "anomalous", false)

	recs := sink.Records()
	if len(recs) != 1 {
		t.Fatalf("Records() len = %d, want 1", len(recs))
	}
	r := recs[0]
	if r.Level != slog.LevelWarn {
		t.Errorf("Level = %v, want WARN", r.Level)
	}
	if r.Msg != "geometry complete" {
		t.Errorf("Msg = %q, want %q", r.Msg, "geometry complete")
	}
	wantKeys := []string{"panes", "took", "anomalous"}
	if len(r.Keys) != len(wantKeys) {
		t.Fatalf("Keys = %v, want %v", r.Keys, wantKeys)
	}
	for i, k := range wantKeys {
		if r.Keys[i] != k {
			t.Errorf("Keys[%d] = %q, want %q", i, r.Keys[i], k)
		}
	}
}

func TestSink_RendersBoundComponentOnEveryLine(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	bound := logger.With("component", "daemon")

	bound.Info("lock acquired", "session", "_portal-saver")

	if got, want := sink.Body(), "INFO lock acquired component=daemon session=_portal-saver"; got != want {
		t.Errorf("Body() = %q, want %q", got, want)
	}
}

func TestSink_Lines_ReturnsCopy(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)

	logger.Info("one")
	snapshot := sink.Lines()
	logger.Info("two")

	if len(snapshot) != 1 {
		t.Fatalf("snapshot len = %d, want 1 (must not see later writes)", len(snapshot))
	}
	if snapshot[0] != "INFO one" {
		t.Errorf("snapshot[0] = %q, want %q", snapshot[0], "INFO one")
	}
}

func TestSink_RecordsAttrValues(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	bound := logger.With("component", "daemon")

	bound.Info("tick complete", "panes", 4, "took", 3*time.Second)

	recs := sink.Records()
	if len(recs) != 1 {
		t.Fatalf("Records() len = %d, want 1", len(recs))
	}
	r := recs[0]

	comp, ok := r.Attrs["component"]
	if !ok {
		t.Fatalf("Attrs missing bound component: %+v", r.Attrs)
	}
	if comp.String() != "daemon" {
		t.Errorf("component = %q, want %q", comp.String(), "daemon")
	}
	panes, ok := r.Attrs["panes"]
	if !ok || panes.Kind() != slog.KindInt64 || panes.Int64() != 4 {
		t.Errorf("panes attr = %+v, want Int64 4", panes)
	}
	took, ok := r.Attrs["took"]
	if !ok || took.Kind() != slog.KindDuration || took.Duration() != 3*time.Second {
		t.Errorf("took attr = %+v, want Duration 3s", took)
	}

	if len(r.Attrs) != len(r.Keys) {
		t.Fatalf("Attrs key count %d != Keys count %d (%v / %v)", len(r.Attrs), len(r.Keys), r.Attrs, r.Keys)
	}
	for _, k := range r.Keys {
		if _, ok := r.Attrs[k]; !ok {
			t.Errorf("key %q present in Keys but missing from Attrs", k)
		}
	}
}

func TestSink_RecordsAttrValues_LastWriteWins(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	bound := logger.With("via", "internal")

	bound.Info("set", "via", "cli")

	recs := sink.Records()
	if len(recs) != 1 {
		t.Fatalf("Records() len = %d, want 1", len(recs))
	}
	via, ok := recs[0].Attrs["via"]
	if !ok {
		t.Fatalf("Attrs missing via: %+v", recs[0].Attrs)
	}
	if via.String() != "cli" {
		t.Errorf("via = %q, want %q (per-call attr must win over bound)", via.String(), "cli")
	}
}

func TestRecord_AttrString(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Info("set", "op", "set")

	r := sink.Records().Only(t, "log record")
	if got := r.AttrString(t, "op"); got != "set" {
		t.Errorf("AttrString(op) = %q, want %q", got, "set")
	}

	if !expectFail(func(sub harnesstest.TestingT) { r.AttrString(sub, "nope") }) {
		t.Errorf("AttrString must fail the test for a missing key")
	}
}

func TestRecord_AttrOrEmpty(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Info("set", "op", "set")

	r := sink.Records().Only(t, "log record")

	t.Run("it returns the value of a present attr", func(t *testing.T) {
		if got := r.AttrOrEmpty("op"); got != "set" {
			t.Errorf("AttrOrEmpty(op) = %q, want %q", got, "set")
		}
	})

	t.Run("it returns the empty string for an absent attr", func(t *testing.T) {
		if got := r.AttrOrEmpty("nope"); got != "" {
			t.Errorf("AttrOrEmpty(nope) = %q, want the empty string", got)
		}
	})
}

func TestRecord_DurationAttr(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Info("tick complete", "took", 5*time.Millisecond, "entries", "2")

	r := sink.Records().Only(t, "log record")

	t.Run("it returns the duration carried by the named attr", func(t *testing.T) {
		if got := r.DurationAttr(t, "took"); got != 5*time.Millisecond {
			t.Errorf("DurationAttr(took) = %v, want %v", got, 5*time.Millisecond)
		}
	})

	t.Run("it fatals when the attr is not a duration", func(t *testing.T) {
		if !expectFail(func(sub harnesstest.TestingT) { _ = r.DurationAttr(sub, "entries") }) {
			t.Errorf("DurationAttr must fail for a non-Duration attr")
		}
	})

	t.Run("it fatals when the record carries no attr of that name", func(t *testing.T) {
		if !expectFail(func(sub harnesstest.TestingT) { _ = r.DurationAttr(sub, "elapsed") }) {
			t.Errorf("DurationAttr must fail for a missing key")
		}
	})
}

func TestRecords_Only(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	clean := logger.With("component", "clean")
	clean.Debug("orphan killed")
	clean.Warn("orphan killed")
	clean.Info("sweep complete")
	logger.With("component", "bootstrap").Info("sweep complete")

	t.Run("it returns the sole record of a component-filtered set", func(t *testing.T) {
		rec := sink.Records().Matching("clean", "sweep complete").Only(t, "clean sweep-complete record")
		if got := rec.AttrString(t, "component"); got != "clean" {
			t.Errorf("component = %q, want %q", got, "clean")
		}
	})

	t.Run("it returns the sole record of a level-filtered set", func(t *testing.T) {
		rec := sink.Records().AtExactLevel(slog.LevelWarn).Only(t, "clean orphan-killed WARN")
		if rec.Level != slog.LevelWarn {
			t.Errorf("Level = %v, want WARN", rec.Level)
		}
	})

	t.Run("it fatals unless exactly one record matched", func(t *testing.T) {
		if !expectFail(func(sub harnesstest.TestingT) {
			_ = sink.Records().WithMessage("orphan killed").Only(sub, "orphan-killed record")
		}) {
			t.Errorf("Only must fail when more than one record matched")
		}
		if !expectFail(func(sub harnesstest.TestingT) {
			_ = sink.Records().WithMessage("never emitted").Only(sub, "never-emitted record")
		}) {
			t.Errorf("Only must fail when no record matched")
		}
	})

	t.Run("it names the described set in its failure message", func(t *testing.T) {
		failed, msg := captureFailure(func(sub harnesstest.TestingT) {
			_ = sink.Records().WithMessage("never emitted").Only(sub, "never-emitted record")
		})
		if !failed {
			t.Fatalf("Only did not fail on an empty set")
		}
		if !strings.Contains(msg, "never-emitted record") {
			t.Errorf("failure message = %q, want it to name the described set", msg)
		}
	})
}

func TestRecord_IntAttr(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Info("tick complete", "panes", 7, "msg", "text")

	r := sink.Records().Only(t, "log record")
	if got := r.IntAttr(t, "panes"); got != 7 {
		t.Errorf("IntAttr(panes) = %d, want 7", got)
	}

	if !expectFail(func(sub harnesstest.TestingT) { r.IntAttr(sub, "msg") }) {
		t.Errorf("IntAttr must fail for a non-Int64 attr")
	}
}

func TestRecord_ErrorAttr(t *testing.T) {
	sentinel := errors.New("temp file create failed")
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Warn("save failed", "error", sentinel, "op", "set")

	r := sink.Records().Only(t, "log record")

	t.Run("it returns the error carried by the named attr", func(t *testing.T) {
		if got := r.ErrorAttr(t, "error"); !errors.Is(got, sentinel) {
			t.Errorf("ErrorAttr(error) = %v, want %v", got, sentinel)
		}
	})

	t.Run("it fails when the record carries no attr of that name", func(t *testing.T) {
		if !expectFail(func(sub harnesstest.TestingT) { _ = r.ErrorAttr(sub, "cause") }) {
			t.Errorf("ErrorAttr must fail the test for a missing key")
		}
	})

	t.Run("it fails when the attr value is not an error", func(t *testing.T) {
		if !expectFail(func(sub harnesstest.TestingT) { _ = r.ErrorAttr(sub, "op") }) {
			t.Errorf("ErrorAttr must fail for an attr carrying a non-error value")
		}
	})

	t.Run("it reports the record's attrs in its failure message", func(t *testing.T) {
		failed, msg := captureFailure(func(sub harnesstest.TestingT) { _ = r.ErrorAttr(sub, "cause") })
		if !failed {
			t.Fatalf("ErrorAttr(cause) did not fail")
		}
		if !strings.Contains(msg, "cause") || !strings.Contains(msg, "op") {
			t.Errorf("failure message = %q, want it to name the missing key and the record's attrs", msg)
		}
	})
}

func TestRecord_RequireDuration(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Info("tick complete", "took", 5*time.Millisecond, "entries", "2")

	r := sink.Records().Only(t, "log record")
	r.RequireDuration(t, "took")

	if !expectFail(func(sub harnesstest.TestingT) { r.RequireDuration(sub, "entries") }) {
		t.Errorf("RequireDuration must fail for a non-Duration attr")
	}
}

func TestRecord_HasAttr(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Info("rm", "op", "rm")

	r := sink.Records().Only(t, "log record")
	if !r.HasAttr("op") {
		t.Errorf("HasAttr(op) = false, want true")
	}
	if r.HasAttr("value") {
		t.Errorf("HasAttr(value) = true, want false")
	}
}

func TestRecords_AtOrAboveLevel(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Debug("below")
	logger.Warn("at")
	logger.Error("above")

	got := sink.Records().AtOrAboveLevel(slog.LevelWarn)
	if len(got) != 2 {
		t.Fatalf("AtOrAboveLevel(WARN) returned %d records, want 2: %+v", len(got), got)
	}
	if got[0].Msg != "at" || got[1].Msg != "above" {
		t.Errorf("AtOrAboveLevel(WARN) messages = [%q %q], want [\"at\" \"above\"]", got[0].Msg, got[1].Msg)
	}

	if none := sink.Records().AtOrAboveLevel(slog.LevelError + 1); none != nil {
		t.Errorf("AtOrAboveLevel above every captured level = %+v, want nil", none)
	}
}

func TestRecords_AtExactLevel(t *testing.T) {
	t.Run("it filters at exactly the given level", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		logger.Debug("below")
		logger.Warn("at")
		logger.Error("above")

		got := sink.Records().AtExactLevel(slog.LevelWarn)
		if len(got) != 1 {
			t.Fatalf("AtExactLevel(WARN) returned %d records, want 1: %+v", len(got), got)
		}
		if got[0].Msg != "at" {
			t.Errorf("AtExactLevel(WARN) message = %q, want %q", got[0].Msg, "at")
		}

		if none := sink.Records().AtExactLevel(slog.LevelInfo); none != nil {
			t.Errorf("AtExactLevel(INFO) = %+v, want nil", none)
		}
	})
}

func TestRecords_Matching(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	hooks := logger.With("component", "hooks")
	hooks.Info("clean-stale")
	hooks.Info("clean-stale")
	hooks.Info("set")
	logger.With("component", "projects").Info("clean-stale")
	logger.Info("clean-stale")

	got := sink.Records().Matching("hooks", "clean-stale")
	if len(got) != 2 {
		t.Fatalf("Matching(hooks, clean-stale) returned %d records, want 2: %+v", len(got), got)
	}

	if none := sink.Records().Matching("hooks", "nothing"); none != nil {
		t.Errorf("Matching on an unemitted message = %+v, want nil", none)
	}
}

func TestRecords_FilterComposition(t *testing.T) {
	t.Run("it composes a level filter with a component filter", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		logger.With("component", "clean").Warn("orphan killed")
		logger.With("component", "clean").Error("orphan killed")
		logger.With("component", "bootstrap").Warn("orphan killed")

		got := sink.Records().Matching("clean", "orphan killed").AtExactLevel(slog.LevelWarn)
		if len(got) != 1 {
			t.Fatalf("Matching then AtExactLevel returned %d records, want 1: %+v", len(got), got)
		}
		if got[0].Level != slog.LevelWarn {
			t.Errorf("Level = %v, want WARN", got[0].Level)
		}
	})
}

func expectFail(fn func(harnesstest.TestingT)) bool {
	failed, _ := captureFailure(fn)
	return failed
}

// captureFailure runs fn against the shared stand-in, which absorbs the abort a
// Fatalf stands for, and reports whether it failed and with what message.
func captureFailure(fn func(harnesstest.TestingT)) (failed bool, msg string) {
	rec := &harnesstest.Recorder{}
	rec.Run(func() { fn(rec) })
	if len(rec.Fatals) == 0 {
		return false, ""
	}
	return true, rec.Fatals[0]
}

func TestRecords_WithMessage(t *testing.T) {
	t.Run("it filters on message alone across components", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		logger.With("component", "bootstrap").Warn("show-hooks failed")
		logger.With("component", "saver").Warn("show-hooks failed")
		logger.With("component", "bootstrap").Warn("something else")
		logger.Info("show-hooks failed")

		got := sink.Records().WithMessage("show-hooks failed")
		if len(got) != 3 {
			t.Fatalf("WithMessage(show-hooks failed) returned %d records, want 3: %+v", len(got), got)
		}

		warns := sink.Records().WithMessage("show-hooks failed").AtExactLevel(slog.LevelWarn)
		if len(warns) != 2 {
			t.Fatalf("WithMessage then AtExactLevel returned %d records, want 2: %+v", len(warns), warns)
		}

		if none := sink.Records().WithMessage("never emitted"); none != nil {
			t.Errorf("WithMessage on an unemitted message = %+v, want nil", none)
		}
	})

	t.Run("it returns every record carrying the message regardless of component", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		logger.With("component", "hooks").Info("clean-stale")
		logger.With("component", "projects").Warn("clean-stale")
		logger.Debug("clean-stale")
		logger.With("component", "hooks").Info("set")

		got := sink.Records().WithMessage("clean-stale")
		if len(got) != 3 {
			t.Fatalf("WithMessage(clean-stale) returned %d records, want 3: %+v", len(got), got)
		}
		for i, rec := range got {
			if rec.Msg != "clean-stale" {
				t.Errorf("record %d message = %q, want %q", i, rec.Msg, "clean-stale")
			}
		}
		if got[0].Level != slog.LevelInfo || got[1].Level != slog.LevelWarn || got[2].Level != slog.LevelDebug {
			t.Errorf("levels = [%v %v %v], want [INFO WARN DEBUG] in capture order", got[0].Level, got[1].Level, got[2].Level)
		}
	})

	t.Run("it returns nothing for a message no record carries", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		logger.With("component", "hooks").Info("clean-stale")

		if none := sink.Records().WithMessage("never emitted"); none != nil {
			t.Errorf("WithMessage on an unemitted message = %+v, want nil", none)
		}
	})
}

func TestRecords_ComposedRoutesCoverEveryQuery(t *testing.T) {
	t.Run("it answers each question the filters compose to over one capture", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		clean := logger.With("component", "clean")
		clean.Debug("orphan killed")
		clean.Warn("orphan killed")
		clean.Error("orphan killed")
		logger.With("component", "bootstrap").Warn("orphan killed")
		clean.Warn("sweep complete")

		cases := []struct {
			name string
			got  logtest.Records
			want int
		}{
			{"Records()", sink.Records(), 5},
			{"Records().AtExactLevel", sink.Records().AtExactLevel(slog.LevelWarn), 3},
			{"Records().AtOrAboveLevel", sink.Records().AtOrAboveLevel(slog.LevelWarn), 4},
			{"Records().Matching", sink.Records().Matching("clean", "orphan killed"), 3},
			{"Records().WithMessage", sink.Records().WithMessage("orphan killed"), 4},
			{"Records().Matching().AtExactLevel", sink.Records().Matching("clean", "orphan killed").AtExactLevel(slog.LevelWarn), 1},
			{"Records().WithMessage().AtExactLevel", sink.Records().WithMessage("orphan killed").AtExactLevel(slog.LevelWarn), 2},
		}
		for _, tc := range cases {
			if len(tc.got) != tc.want {
				t.Errorf("%s returned %d records, want %d: %+v", tc.name, len(tc.got), tc.want, tc.got)
			}
		}
	})
}

func TestRecord_Matches(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.With("component", "saver").Info("kill-barrier escalated")
	logger.With("component", "bootstrap").Info("kill-barrier escalated")
	logger.Info("kill-barrier escalated")

	recs := sink.Records()
	if !recs[0].Matches("saver", "kill-barrier escalated") {
		t.Errorf("Matches(saver, kill-barrier escalated) = false on the saver record, want true")
	}
	if recs[1].Matches("saver", "kill-barrier escalated") {
		t.Errorf("Matches matched a record emitted under a different component")
	}
	if recs[2].Matches("saver", "kill-barrier escalated") {
		t.Errorf("Matches matched a record carrying no component attr")
	}
	if recs[0].Matches("saver", "kill-barrier started") {
		t.Errorf("Matches matched a record carrying a different message")
	}
}
