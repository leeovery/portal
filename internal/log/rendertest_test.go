package log

import (
	"bytes"
	"log/slog"
	"testing"
	"time"
)

func TestRenderLineForTest_ByteIdenticalToHandle(t *testing.T) {
	ts := time.Date(2026, 6, 9, 10, 15, 30, 123456789, time.UTC)
	const component = "daemon"
	const message = "tick complete"
	attrs := []slog.Attr{
		slog.Duration("took", 12*time.Millisecond),
		slog.String("session", "foo"),
	}

	var buf bytes.Buffer
	h := newTextHandler(&buf, slog.LevelInfo, testRenderPID, testRenderVersion, testRenderProcessRole)
	h = h.WithAttrs([]slog.Attr{slog.String(componentKey, component)})
	rec := slog.NewRecord(ts, slog.LevelWarn, message, 0)
	rec.AddAttrs(attrs...)
	handleRecord(t, h, rec)

	want := buf.String()
	got := RenderLineForTest(t, ts, slog.LevelWarn, component, message, attrs...)

	if got != want {
		t.Errorf("RenderLineForTest output mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestRenderLineForTest_IncludesPrefixBaselinesAndNewline(t *testing.T) {
	ts := time.Date(2026, 6, 9, 10, 15, 30, 0, time.UTC)
	got := RenderLineForTest(t, ts, slog.LevelWarn, "daemon", "tick complete",
		slog.String("session", "foo"))

	parsed, ok := ParseLogLine(got)
	if !ok {
		t.Fatalf("ParseLogLine ok = false for rendered line %q", got)
	}
	if parsed.Component != "daemon" {
		t.Errorf("Component = %q, want %q", parsed.Component, "daemon")
	}
	if parsed.Message != "tick complete" {
		t.Errorf("Message = %q, want %q", parsed.Message, "tick complete")
	}
	if parsed.Level != "WARN" {
		t.Errorf("Level = %q, want WARN", parsed.Level)
	}
	for _, want := range []string{
		" daemon: tick complete ",
		"session=foo",
		"pid=",
		"version=",
		"process_role=",
	} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Errorf("rendered line missing %q, got: %q", want, got)
		}
	}
	if len(got) == 0 || got[len(got)-1] != '\n' {
		t.Errorf("rendered line must end with a trailing newline, got: %q", got)
	}
}

func TestRenderLineForTest_DoesNotMutateProcessGlobalHandler(t *testing.T) {
	before := currentHandler()
	_ = RenderLineForTest(t, time.Now(), slog.LevelWarn, "daemon", "tick complete")
	after := currentHandler()
	if before != after {
		t.Errorf("RenderLineForTest mutated the process-global handler: before=%p after=%p", before, after)
	}
}

func TestRenderLineForTest_TestingTFirst(t *testing.T) {
	line := RenderLineForTest(t, time.Now(), slog.LevelWarn, "daemon", "msg")
	if line == "" {
		t.Fatal("RenderLineForTest returned empty line")
	}
}
