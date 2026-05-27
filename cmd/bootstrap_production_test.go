package cmd

// Shared test scaffolding for the bootstrap step-11 stale-hook cleanup
// path. The duplicated cleanStaleAdapterT mirror type and its
// TestCleanStaleAdapter subtests were removed once
// runHookStaleCleanup became the single source of truth for the
// algorithm (see cmd/run_hook_stale_cleanup.go and
// cmd/run_hook_stale_cleanup_test.go) — production wiring composes the
// helper through cleanStaleAdapter (cmd/bootstrap_production.go) and
// cleanCmd.RunE (cmd/clean.go) so no test mirror is needed. The
// remaining helpers (recordingLogger, stubAllPaneLister,
// newTempHooksStore, readFileBytes, countMatching, keysOf) are
// consumed by run_hook_stale_cleanup_test.go.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/tmux"
)

// recordedLog captures one Logger emission for post-call assertions.
type recordedLog struct {
	level     string
	component string
	format    string
	args      []any
}

// recordingLogger satisfies bootstrap.Logger by appending every emission
// to an in-memory slice. Tests inspect entries directly.
type recordingLogger struct {
	entries []recordedLog
}

// Debug records a debug-level emission.
func (r *recordingLogger) Debug(component, format string, args ...any) {
	r.entries = append(r.entries, recordedLog{"debug", component, format, args})
}

// Info records an info-level emission.
func (r *recordingLogger) Info(component, format string, args ...any) {
	r.entries = append(r.entries, recordedLog{"info", component, format, args})
}

// Warn records a warn-level emission.
func (r *recordingLogger) Warn(component, format string, args ...any) {
	r.entries = append(r.entries, recordedLog{"warn", component, format, args})
}

// Error records an error-level emission.
func (r *recordingLogger) Error(component, format string, args ...any) {
	r.entries = append(r.entries, recordedLog{"error", component, format, args})
}

// Compile-time assertion that recordingLogger satisfies bootstrap.Logger.
var _ bootstrap.Logger = (*recordingLogger)(nil)

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
// given level + component-substring + format-string-equality predicates.
// Format equality (rather than substring) keeps assertions tight against
// the adapter's emitted format strings.
func countMatching(entries []recordedLog, level, component, format string) int {
	n := 0
	for _, e := range entries {
		if e.level == level && e.component == component && e.format == format {
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
