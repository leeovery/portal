package log

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// handleRecord drives a record through h, failing the test on a Handle error.
// The rendered output is captured by the buffer the caller passed to
// newTextHandler.
func handleRecord(t *testing.T, h slog.Handler, r slog.Record) {
	t.Helper()
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
}

// newRecord builds a slog.Record carrying the given message, level, and attrs.
func newRecord(level slog.Level, msg string, attrs ...slog.Attr) slog.Record {
	r := slog.NewRecord(time.Date(2026, 5, 29, 8, 38, 0, 0, time.UTC), level, msg, 0)
	r.AddAttrs(attrs...)
	return r
}

func TestTextHandler_RendersComponentAsPrefixAndOmitsFromAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := newTextHandler(&buf, slog.LevelInfo, 12345, "0.5.0", "hydrate")
	// component arrives via WithAttrs, exactly as For (root.With("component", ...)) delivers it.
	h = h.WithAttrs([]slog.Attr{slog.String("component", "hydrate")})

	handleRecord(t, h, newRecord(slog.LevelInfo, "ok", slog.String("pane_key", "foo:0.0")))

	line := buf.String()
	if !strings.Contains(line, " hydrate: ok ") {
		t.Errorf("expected literal component prefix %q, got line: %q", " hydrate: ok ", line)
	}
	if strings.Contains(line, "component=") {
		t.Errorf("component must NOT appear in the key=value attr list, got line: %q", line)
	}
}

func TestTextHandler_AppendsBaselinesInTrailingOrder(t *testing.T) {
	var buf bytes.Buffer
	h := newTextHandler(&buf, slog.LevelInfo, 12345, "0.5.0", "hydrate")
	h = h.WithAttrs([]slog.Attr{slog.String("component", "hydrate")})

	took, _ := time.ParseDuration("1.2s")
	handleRecord(t, h, newRecord(slog.LevelInfo, "ok",
		slog.String("pane_key", "foo:0.0"),
		slog.Duration("took", took),
	))

	want := "2026-05-29T08:38:00Z INFO hydrate: ok pane_key=foo:0.0 took=1.2s pid=12345 version=0.5.0 process_role=hydrate\n"
	if got := buf.String(); got != want {
		t.Errorf("rendered line mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestTextHandler_InjectsBaselinesOnLoggerCachedBeforeHandlerExisted(t *testing.T) {
	restore := snapshotHandler()
	t.Cleanup(restore)

	// Obtain a logger BEFORE the configured handler is constructed/swapped.
	cached := For("daemon")

	var buf bytes.Buffer
	setHandler(newTextHandler(&buf, slog.LevelInfo, 999, "9.9.9", "daemon"))

	cached.Info("cached")

	line := buf.String()
	if !strings.Contains(line, " daemon: cached ") {
		t.Errorf("expected component prefix from cached logger, got: %q", line)
	}
	for _, want := range []string{"pid=999", "version=9.9.9", "process_role=daemon"} {
		if !strings.Contains(line, want) {
			t.Errorf("expected baseline %q on cached-logger line, got: %q", want, line)
		}
	}
}

func TestTextHandler_QuotesMultiWordValuesAndLeavesSingleTokens(t *testing.T) {
	var buf bytes.Buffer
	h := newTextHandler(&buf, slog.LevelInfo, 1, "v", "tui")
	h = h.WithAttrs([]slog.Attr{slog.String("component", "tui")})

	handleRecord(t, h, newRecord(slog.LevelInfo, "msg",
		slog.String("single", "token"),
		slog.String("multi", "two words"),
	))

	line := buf.String()
	if !strings.Contains(line, "single=token") {
		t.Errorf("single-token value must be unquoted, got: %q", line)
	}
	if !strings.Contains(line, `multi="two words"`) {
		t.Errorf("multi-word value must be double-quoted, got: %q", line)
	}
}

func TestTextHandler_RendersDurationViaString(t *testing.T) {
	var buf bytes.Buffer
	h := newTextHandler(&buf, slog.LevelInfo, 1, "v", "daemon")
	h = h.WithAttrs([]slog.Attr{slog.String("component", "daemon")})

	handleRecord(t, h, newRecord(slog.LevelInfo, "msg",
		slog.Duration("took", 3*time.Second),
	))

	line := buf.String()
	if !strings.Contains(line, "took=3s") {
		t.Errorf("duration must render via String() (e.g. 3s), got: %q", line)
	}
	if strings.Contains(line, "took=3000000000") {
		t.Errorf("duration must NOT render as integer nanoseconds, got: %q", line)
	}
}

func TestTextHandler_FlattensGroupAttrsToDottedKeys(t *testing.T) {
	var buf bytes.Buffer
	h := newTextHandler(&buf, slog.LevelInfo, 1, "v", "daemon")
	h = h.WithAttrs([]slog.Attr{slog.String("component", "daemon")})

	handleRecord(t, h, newRecord(slog.LevelInfo, "msg",
		slog.Group("g", slog.String("k", "v")),
	))

	line := buf.String()
	if !strings.Contains(line, "g.k=v") {
		t.Errorf("group attr must flatten to dotted key g.k=v, got: %q", line)
	}
}

func TestTextHandler_DropsDebugWhenConfiguredLevelIsInfo(t *testing.T) {
	var buf bytes.Buffer
	h := newTextHandler(&buf, slog.LevelInfo, 1, "v", "daemon")
	h = h.WithAttrs([]slog.Attr{slog.String("component", "daemon")})

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Enabled(DEBUG) must be false when configured level is INFO")
	}
	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Enabled(INFO) must be true when configured level is INFO")
	}
}
