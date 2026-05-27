package cmd

// Unit coverage for the bootstrap step-11 stale-hook cleanup adapter
// (cleanStaleAdapter in cmd/bootstrap_production.go). The production
// adapter consumes *tmux.Client directly — no seam interface exists for
// pane enumeration in the production type — so this file defines a
// test-local cleanStaleAdapterT struct mirroring the production field
// layout but substituting the existing cmd.AllPaneLister interface (see
// cmd/clean.go:13-15) for the *tmux.Client field. The test-local type
// re-implements the six-branch algorithm from cleanStaleAdapter.CleanStale
// verbatim. Drift risk is mitigated by the algorithm being short and
// fully specified plus Phase 3 integration tests covering the real
// production adapter.
//
// Spec reference: §Test Requirements §New File ("Inverting the existing
// clean_test.go subtest is necessary but not sufficient — the adapter
// has its own path") and §Acceptance Criteria item 4 (mutual
// exclusivity of the entry-point Debug and the terminal log line).

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/state"
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

// cleanStaleAdapterT is the test-local mirror of cleanStaleAdapter — same
// field layout, but lister AllPaneLister substitutes for client *tmux.Client.
// CleanStale re-implements the six-branch algorithm from
// cleanStaleAdapter.CleanStale verbatim (cmd/bootstrap_production.go:129-180).
type cleanStaleAdapterT struct {
	lister AllPaneLister
	store  *hooks.Store
	Logger bootstrap.Logger
}

// CleanStale mirrors cleanStaleAdapter.CleanStale verbatim.
func (a *cleanStaleAdapterT) CleanStale() error {
	logger := a.Logger
	if logger == nil {
		logger = cleanStaleNoopLogger{}
	}

	livePanes, err := a.lister.ListAllPanes()
	if err != nil {
		logger.Warn(state.ComponentBootstrap, "stale-hook cleanup: list-panes failed: %v", err)
		return err
	}

	persisted, err := a.store.Load()
	if err != nil {
		logger.Warn(state.ComponentBootstrap, "stale-hook cleanup: hookStore.Load failed: %v", err)
		return err
	}

	logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: live=%d persisted=%d", len(livePanes), len(persisted))

	if len(livePanes) == 0 {
		if len(persisted) == 0 {
			return nil
		}
		logger.Warn(state.ComponentBootstrap,
			"stale-hook cleanup: zero live panes parsed with %d hook(s) present; skipping to avoid mass-deletion hazard (next bootstrap retries)",
			len(persisted))
		return nil
	}

	removed, err := a.store.CleanStale(livePanes)
	if err != nil {
		return err
	}
	logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: removed=%d", len(removed))
	return nil
}

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

func TestCleanStaleAdapter(t *testing.T) {
	const entryDebugFmt = "stale-hook cleanup: live=%d persisted=%d"
	const completionDebugFmt = "stale-hook cleanup: removed=%d"
	const hazardWarnFmt = "stale-hook cleanup: zero live panes parsed with %d hook(s) present; skipping to avoid mass-deletion hazard (next bootstrap retries)"
	const listPanesWarnFmt = "stale-hook cleanup: list-panes failed: %v"

	t.Run("hazard guard fires on empty live + non-empty persisted", func(t *testing.T) {
		seed := `{
  "a:0.0": {"on-resume": "cmd-a"},
  "b:0.0": {"on-resume": "cmd-b"}
}`
		store, path := newTempHooksStore(t, seed)
		before := readFileBytes(t, path)

		logger := &recordingLogger{}
		adapter := &cleanStaleAdapterT{
			lister: &stubAllPaneLister{panes: []string{}, err: nil},
			store:  store,
			Logger: logger,
		}

		if err := adapter.CleanStale(); err != nil {
			t.Fatalf("CleanStale: %v", err)
		}

		after := readFileBytes(t, path)
		if !reflect.DeepEqual(before, after) {
			t.Errorf("hooks.json modified by hazard-guard branch: before=%q after=%q", before, after)
		}

		// Exactly one entry-point Debug with live=0 persisted=2.
		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, entryDebugFmt); got != 1 {
			t.Errorf("entry-point Debug count = %d, want 1; entries=%+v", got, logger.entries)
		}
		// Verify entry-point Debug args carry the expected counts.
		for _, e := range logger.entries {
			if e.format == entryDebugFmt {
				if len(e.args) != 2 || e.args[0] != 0 || e.args[1] != 2 {
					t.Errorf("entry-point Debug args = %v, want [0 2]", e.args)
				}
			}
		}

		// Exactly one hazard Warn.
		if got := countMatching(logger.entries, "warn", state.ComponentBootstrap, hazardWarnFmt); got != 1 {
			t.Errorf("hazard Warn count = %d, want 1; entries=%+v", got, logger.entries)
		}

		// Mutual exclusivity: NO completion Debug.
		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, completionDebugFmt); got != 0 {
			t.Errorf("completion Debug count = %d, want 0 (must NOT fire on hazard branch); entries=%+v", got, logger.entries)
		}
	})

	t.Run("both-sides-empty no-op", func(t *testing.T) {
		store, path := newTempHooksStore(t, "")
		before := readFileBytes(t, path) // ENOENT → nil; preserved post-call.

		logger := &recordingLogger{}
		adapter := &cleanStaleAdapterT{
			lister: &stubAllPaneLister{panes: []string{}, err: nil},
			store:  store,
			Logger: logger,
		}

		if err := adapter.CleanStale(); err != nil {
			t.Fatalf("CleanStale: %v", err)
		}

		after := readFileBytes(t, path)
		if !reflect.DeepEqual(before, after) {
			t.Errorf("hooks.json materialised under both-sides-empty path: before=%v after=%v", before, after)
		}

		// Exactly one entry-point Debug with live=0 persisted=0.
		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, entryDebugFmt); got != 1 {
			t.Errorf("entry-point Debug count = %d, want 1; entries=%+v", got, logger.entries)
		}
		for _, e := range logger.entries {
			if e.format == entryDebugFmt {
				if len(e.args) != 2 || e.args[0] != 0 || e.args[1] != 0 {
					t.Errorf("entry-point Debug args = %v, want [0 0]", e.args)
				}
			}
		}

		// No Warn of any kind.
		for _, e := range logger.entries {
			if e.level == "warn" {
				t.Errorf("unexpected Warn under both-sides-empty: %+v", e)
			}
		}

		// No completion Debug (nothing was removed; no follow-on line should fire).
		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, completionDebugFmt); got != 0 {
			t.Errorf("completion Debug count = %d, want 0; entries=%+v", got, logger.entries)
		}
	})

	t.Run("ListAllPanes error propagates as soft warning", func(t *testing.T) {
		seed := `{"a:0.0": {"on-resume": "cmd-a"}}`
		store, path := newTempHooksStore(t, seed)
		before := readFileBytes(t, path)

		sentinel := errors.New("tmux dead")
		logger := &recordingLogger{}
		adapter := &cleanStaleAdapterT{
			lister: &stubAllPaneLister{panes: nil, err: sentinel},
			store:  store,
			Logger: logger,
		}

		err := adapter.CleanStale()
		if err == nil {
			t.Fatalf("CleanStale: want error, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("CleanStale err = %v, want errors.Is == sentinel %v", err, sentinel)
		}

		after := readFileBytes(t, path)
		if !reflect.DeepEqual(before, after) {
			t.Errorf("hooks.json modified on ListAllPanes-error path: before=%q after=%q", before, after)
		}

		// Exactly one terminal Warn (list-panes failed).
		if got := countMatching(logger.entries, "warn", state.ComponentBootstrap, listPanesWarnFmt); got != 1 {
			t.Errorf("list-panes Warn count = %d, want 1; entries=%+v", got, logger.entries)
		}

		// NO entry-point Debug (terminal-Warn-only branch per spec §Change 4).
		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, entryDebugFmt); got != 0 {
			t.Errorf("entry-point Debug count = %d, want 0 (must NOT fire on ListAllPanes-error branch); entries=%+v", got, logger.entries)
		}

		// NO hazard Warn (mutually exclusive with terminal Warn).
		if got := countMatching(logger.entries, "warn", state.ComponentBootstrap, hazardWarnFmt); got != 0 {
			t.Errorf("hazard Warn count = %d, want 0 on ListAllPanes-error branch; entries=%+v", got, logger.entries)
		}
	})

	t.Run("legitimate stale removal", func(t *testing.T) {
		seed := `{
  "a:0.0": {"on-resume": "cmd-a"},
  "b:0.0": {"on-resume": "cmd-b"},
  "c:0.0": {"on-resume": "cmd-c"},
  "d:0.0": {"on-resume": "cmd-d"}
}`
		store, _ := newTempHooksStore(t, seed)

		logger := &recordingLogger{}
		adapter := &cleanStaleAdapterT{
			lister: &stubAllPaneLister{panes: []string{"a:0.0", "b:0.0", "c:0.0"}, err: nil},
			store:  store,
			Logger: logger,
		}

		if err := adapter.CleanStale(); err != nil {
			t.Fatalf("CleanStale: %v", err)
		}

		// Post-run: store contains exactly a,b,c.
		postRun, err := store.Load()
		if err != nil {
			t.Fatalf("store.Load post-run: %v", err)
		}
		wantKeys := map[string]struct{}{"a:0.0": {}, "b:0.0": {}, "c:0.0": {}}
		if len(postRun) != len(wantKeys) {
			t.Errorf("post-run hook count = %d (keys=%v), want %d (keys=%v)", len(postRun), keysOf(postRun), len(wantKeys), wantKeys)
		}
		for k := range wantKeys {
			if _, ok := postRun[k]; !ok {
				t.Errorf("post-run hooks missing live key %q; got %v", k, keysOf(postRun))
			}
		}
		if _, ok := postRun["d:0.0"]; ok {
			t.Errorf("post-run hooks still contains stale key d:0.0; got %v", keysOf(postRun))
		}

		// Exactly one entry-point Debug with live=3 persisted=4.
		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, entryDebugFmt); got != 1 {
			t.Errorf("entry-point Debug count = %d, want 1; entries=%+v", got, logger.entries)
		}
		for _, e := range logger.entries {
			if e.format == entryDebugFmt {
				if len(e.args) != 2 || e.args[0] != 3 || e.args[1] != 4 {
					t.Errorf("entry-point Debug args = %v, want [3 4]", e.args)
				}
			}
		}

		// Exactly one completion Debug with removed=1.
		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, completionDebugFmt); got != 1 {
			t.Errorf("completion Debug count = %d, want 1; entries=%+v", got, logger.entries)
		}
		for _, e := range logger.entries {
			if e.format == completionDebugFmt {
				if len(e.args) != 1 || e.args[0] != 1 {
					t.Errorf("completion Debug args = %v, want [1]", e.args)
				}
			}
		}

		// No Warns on the normal path.
		for _, e := range logger.entries {
			if e.level == "warn" {
				t.Errorf("unexpected Warn on normal-removal path: %+v", e)
			}
		}
	})
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
