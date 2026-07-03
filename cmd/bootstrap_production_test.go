package cmd

// Shared test scaffolding for the hooks stale-cleanup path. The
// duplicated cleanStaleAdapterT mirror type and its TestCleanStaleAdapter
// subtests were removed once runHookStaleCleanup became the single source
// of truth for the algorithm (see cmd/run_hook_stale_cleanup.go and
// cmd/run_hook_stale_cleanup_test.go) — production wiring composes the
// helper through cleanCmd.RunE (cmd/clean.go) so no test mirror is
// needed (the bootstrap-step CleanStale callsite was removed when hooks
// cleanup left the orchestrator). The remaining helpers (recordingLogger,
// stubAllPaneLister, newTempHooksStore, readFileBytes, countMatching,
// keysOf) are consumed by run_hook_stale_cleanup_test.go.

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/tmux"
)

// Compile-time assertion that *tmux.Client satisfies bootstrap.LatchWriter via
// its existing SetServerOption(name, value) method, so buildProductionOrchestrator
// can wire the shared client into the Orchestrator.Latch field unchanged.
var _ bootstrap.LatchWriter = (*tmux.Client)(nil)

// recordedLog captures one Logger emission for post-call assertions. After
// the observability migration the message is the slog terse phrase and
// component is the bound component attr; args is no longer captured (data
// lives in slog attrs).
type recordedLog struct {
	level     string
	component string
	message   string
}

// recordingLogger is a slog.Handler that appends every emission to an
// in-memory slice. Tests inspect entries directly. Use Logger() to obtain a
// *slog.Logger to inject into a step core or adapter.
//
// WithAttrs accumulates the bound attrs (notably the component bound via
// .With("component", ...)) and replays them onto every record so the captured
// recordedLog.component is populated even though production binds the
// component at the logger, not at each call site.
type recordingLogger struct {
	entries []recordedLog
	// shared points at the entries-owning recorder so handlers derived via
	// WithAttrs/WithGroup record back into the same slice; nil on the root.
	shared *recordingLogger
	bound  []slog.Attr
}

// Logger returns a *slog.Logger whose records are captured by this recorder.
func (r *recordingLogger) Logger() *slog.Logger { return slog.New(r) }

func (r *recordingLogger) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (r *recordingLogger) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(r.bound)+len(attrs))
	next = append(next, r.bound...)
	next = append(next, attrs...)
	return &recordingLogger{shared: r.owner(), bound: next}
}

func (r *recordingLogger) WithGroup(_ string) slog.Handler {
	return &recordingLogger{shared: r.owner(), bound: r.bound}
}

func (r *recordingLogger) owner() *recordingLogger {
	if r.shared != nil {
		return r.shared
	}
	return r
}

func (r *recordingLogger) Handle(_ context.Context, rec slog.Record) error {
	component := ""
	read := func(a slog.Attr) bool {
		if a.Key == "component" {
			component = a.Value.String()
		}
		return true
	}
	for _, a := range r.bound {
		read(a)
	}
	rec.Attrs(read)
	var level string
	switch rec.Level {
	case slog.LevelDebug:
		level = "debug"
	case slog.LevelInfo:
		level = "info"
	case slog.LevelWarn:
		level = "warn"
	case slog.LevelError:
		level = "error"
	}
	owner := r.owner()
	owner.entries = append(owner.entries, recordedLog{level, component, rec.Message})
	return nil
}

// Compile-time assertion that recordingLogger satisfies slog.Handler.
var _ slog.Handler = (*recordingLogger)(nil)

// stubAllPaneLister returns canned panes/err pairs from ListAllPanes.
type stubAllPaneLister struct {
	panes []string
	err   error
}

// ListAllPanes returns the canned panes/err pair.
func (s *stubAllPaneLister) ListAllPanes() ([]string, error) {
	return s.panes, s.err
}

// Compile-time assertion that *tmux.Client satisfies AllPaneLister so
// the production adapter's direct *tmux.Client field stays substitutable
// in concept with the test-local lister field.
var _ AllPaneLister = (*tmux.Client)(nil)

// newTempHooksStore writes seed JSON to a fresh temp dir's hooks.json
// and returns a real *hooks.Store pointed at that file plus the absolute
// path (so tests can read the file back).
func newTempHooksStore(t *testing.T, seed string) (*hooks.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hooks.json")
	if seed != "" {
		if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
			t.Fatalf("write seed hooks.json: %v", err)
		}
	}
	return hooks.NewStore(path), path
}

// readFileBytes returns the raw file contents or fails the test. ENOENT
// returns nil so callers can distinguish "file absent" from "file empty".
func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// countMatching returns the number of recorded log entries matching the
// given level + component + message-equality predicates. Message equality
// (rather than substring) keeps assertions tight against the adapter's
// emitted terse messages.
func countMatching(entries []recordedLog, level, component, message string) int {
	n := 0
	for _, e := range entries {
		if e.level == level && e.component == component && e.message == message {
			n++
		}
	}
	return n
}

// keysOf returns the sorted-insertion-order keys of a hooksFile-shaped
// map for test diagnostics. The map type from internal/hooks is
// unexported as hooksFile; the public Load returns the same shape via
// the type alias so we accept the concrete map type here.
func keysOf(m map[string]map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
