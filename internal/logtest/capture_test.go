package logtest_test

import (
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
)

// TestNewCaptureLogger_RendersLevelMessageKeyValue locks the canonical
// "<LEVEL> <msg> key=value" rendering contract that every consumer's
// substring assertions key on. This is the single place the contract is
// now pinned; if it changes here, all consumers (cmd, state, restore)
// must be re-evaluated.
func TestNewCaptureLogger_RendersLevelMessageKeyValue(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)

	logger.Info("hello world", "session", "demo", "count", 3)

	if got, want := sink.Body(), "INFO hello world session=demo count=3"; got != want {
		t.Errorf("Body() = %q, want %q", got, want)
	}
}

// TestSink_RecordsLevelMessageAndOrderedKeys verifies the structured-record
// view restore relies on for exact attr-key-set assertions.
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

// TestSink_RendersBoundComponentOnEveryLine confirms a component bound via
// .With("component", …) is rendered on every record — the behaviour cmd's
// component-binding wrapper depends on. State/restore never bind attrs, so
// this path stays dormant for them and their output is unaffected.
func TestSink_RendersBoundComponentOnEveryLine(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	bound := logger.With("component", "daemon")

	bound.Info("lock acquired", "tmux_pane", "%42")

	if got, want := sink.Body(), "INFO lock acquired component=daemon tmux_pane=%42"; got != want {
		t.Errorf("Body() = %q, want %q", got, want)
	}
}

// TestSink_Lines_ReturnsCopy verifies the Lines accessor returns a snapshot
// the caller may retain and inspect (used by cmd's at-exit snapshot tests).
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
