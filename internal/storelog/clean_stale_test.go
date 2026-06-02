package storelog_test

import (
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/storelog"
)

// installCapture swaps the shared logtest.Sink into the process-wide log
// indirection for the duration of the test and returns it. The storelog tests
// assert on the helper's emitted record (component, attr values) via the sink's
// shared accessors.
func installCapture(t *testing.T) *logtest.Sink {
	t.Helper()
	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)
	return sink
}

func TestEmitCleanStaleSummary_SuccessInfo(t *testing.T) {
	logger := log.For("hooks")
	sink := installCapture(t)

	storelog.EmitCleanStaleSummary(logger, 2, time.Now().Add(-5*time.Millisecond), nil)

	rec := sink.OnlyRecord(t)
	if rec.Level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.Level)
	}
	if rec.Msg != "clean-stale" {
		t.Errorf("msg = %q, want %q", rec.Msg, "clean-stale")
	}
	if got := rec.AttrString(t, "op"); got != "clean-stale" {
		t.Errorf("op = %q, want %q", got, "clean-stale")
	}
	if got := rec.AttrString(t, "component"); got != "hooks" {
		t.Errorf("component = %q, want %q", got, "hooks")
	}
	if got := rec.AttrString(t, "entries"); got != "2" {
		t.Errorf("entries = %q, want %q", got, "2")
	}
	if got := rec.AttrString(t, "via"); got != "internal" {
		t.Errorf("via = %q, want %q", got, "internal")
	}
	tookVal, ok := rec.Attrs["took"]
	if !ok {
		t.Fatalf("summary missing took attr: %+v", rec.Attrs)
	}
	if tookVal.Kind() != slog.KindDuration {
		t.Errorf("took attr kind = %v, want Duration", tookVal.Kind())
	}
	// A successful summary must NOT carry error / error_class.
	if _, ok := rec.Attrs["error"]; ok {
		t.Errorf("success summary must omit error attr: %+v", rec.Attrs)
	}
	if _, ok := rec.Attrs["error_class"]; ok {
		t.Errorf("success summary must omit error_class attr: %+v", rec.Attrs)
	}
}

func TestEmitCleanStaleSummary_FailureWarn(t *testing.T) {
	logger := log.For("projects")
	sink := installCapture(t)

	saveErr := fmt.Errorf("%w: boom", fileutil.ErrWriteTempCreate)
	storelog.EmitCleanStaleSummary(logger, 3, time.Now().Add(-5*time.Millisecond), saveErr)

	rec := sink.OnlyRecord(t)
	if rec.Level != slog.LevelWarn {
		t.Errorf("level = %v, want WARN", rec.Level)
	}
	if rec.Msg != "clean-stale" {
		t.Errorf("msg = %q, want %q", rec.Msg, "clean-stale")
	}
	if got := rec.AttrString(t, "op"); got != "clean-stale" {
		t.Errorf("op = %q, want %q", got, "clean-stale")
	}
	if got := rec.AttrString(t, "component"); got != "projects" {
		t.Errorf("component = %q, want %q", got, "projects")
	}
	if got := rec.AttrString(t, "entries"); got != "3" {
		t.Errorf("entries = %q, want %q", got, "3")
	}
	if got := rec.AttrString(t, "via"); got != "internal" {
		t.Errorf("via = %q, want %q", got, "internal")
	}
	// error_class is derived from fileutil.ClassifyWriteError inside the helper.
	if got := rec.AttrString(t, "error_class"); got != "write-failed-temp-create" {
		t.Errorf("error_class = %q, want %q", got, "write-failed-temp-create")
	}
	tookVal, ok := rec.Attrs["took"]
	if !ok {
		t.Fatalf("WARN missing took attr: %+v", rec.Attrs)
	}
	if tookVal.Kind() != slog.KindDuration {
		t.Errorf("took attr kind = %v, want Duration", tookVal.Kind())
	}
	errVal, ok := rec.Attrs["error"]
	if !ok {
		t.Fatalf("WARN record missing error attr: %+v", rec.Attrs)
	}
	loggedErr, ok := errVal.Any().(error)
	if !ok {
		t.Fatalf("error attr is not an error value: %T", errVal.Any())
	}
	if loggedErr != saveErr {
		t.Errorf("logged error attr = %v, want the raw saveErr %v", loggedErr, saveErr)
	}
}
