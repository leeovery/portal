package cmd

// Unit coverage for the shared runHookStaleCleanup helper extracted from
// cleanStaleAdapter.CleanStale (cmd/bootstrap_production.go) and the
// portal-clean RunE (cmd/clean.go). One helper, one declaration of each
// load-bearing log format string, one source of truth that both production
// callsites delegate to so integration substring-asserts cannot drift
// between sites.
//
// Coverage axes (one subtest each):
//   - hazard guard fires when len(live)==0 && len(persisted)>0
//   - both-empty no-op (no Warn, no completion Debug)
//   - ListAllPanes error under returnError policy propagates
//   - ListAllPanes error under swallow policy logs Warn and returns nil
//   - onRemoved invoked once per removed entry (and nil onRemoved is safe)
//   - happy-path entry-point + completion Debug logging

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/state"
)

// newTempHooksStoreForHelper writes seed JSON to a fresh temp dir's
// hooks.json and returns a real *hooks.Store pointed at that file plus
// the absolute path. Mirrors the helper formerly used by the deleted
// cleanStaleAdapterT mirror suite (newTempHooksStore) — re-declared here
// so this file can stand on its own once cleanStaleAdapterT is removed.
func TestRunHookStaleCleanup(t *testing.T) {
	const entryDebugFmt = "stale-hook cleanup: live=%d persisted=%d"
	const completionDebugFmt = "stale-hook cleanup: removed=%d"
	const hazardWarnFmt = "stale-hook cleanup: zero live panes parsed with %d hook(s) present; skipping to avoid mass-deletion hazard (next bootstrap retries)"
	const listPanesWarnFmt = "stale-hook cleanup: list-panes failed: %v"
	const loadWarnFmt = "stale-hook cleanup: hookStore.Load failed: %v"

	t.Run("hazard guard fires on empty live + non-empty persisted", func(t *testing.T) {
		seed := `{
  "a:0.0": {"on-resume": "cmd-a"},
  "b:0.0": {"on-resume": "cmd-b"}
}`
		store, path := newTempHooksStore(t, seed)
		before := readFileBytes(t, path)

		logger := &recordingLogger{}
		lister := &stubAllPaneLister{panes: []string{}, err: nil}

		if err := runHookStaleCleanup(lister, store, logger, returnError, nil); err != nil {
			t.Fatalf("runHookStaleCleanup: %v", err)
		}

		after := readFileBytes(t, path)
		if !reflect.DeepEqual(before, after) {
			t.Errorf("hooks.json modified by hazard-guard branch: before=%q after=%q", before, after)
		}

		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, entryDebugFmt); got != 1 {
			t.Errorf("entry-point Debug count = %d, want 1; entries=%+v", got, logger.entries)
		}
		for _, e := range logger.entries {
			if e.format == entryDebugFmt {
				if len(e.args) != 2 || e.args[0] != 0 || e.args[1] != 2 {
					t.Errorf("entry-point Debug args = %v, want [0 2]", e.args)
				}
			}
		}

		if got := countMatching(logger.entries, "warn", state.ComponentBootstrap, hazardWarnFmt); got != 1 {
			t.Errorf("hazard Warn count = %d, want 1; entries=%+v", got, logger.entries)
		}

		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, completionDebugFmt); got != 0 {
			t.Errorf("completion Debug count = %d, want 0 (must NOT fire on hazard branch); entries=%+v", got, logger.entries)
		}
	})

	t.Run("both-sides-empty no-op", func(t *testing.T) {
		store, path := newTempHooksStore(t, "")
		before := readFileBytes(t, path)

		logger := &recordingLogger{}
		lister := &stubAllPaneLister{panes: []string{}, err: nil}

		if err := runHookStaleCleanup(lister, store, logger, returnError, nil); err != nil {
			t.Fatalf("runHookStaleCleanup: %v", err)
		}

		after := readFileBytes(t, path)
		if !reflect.DeepEqual(before, after) {
			t.Errorf("hooks.json materialised under both-sides-empty path: before=%v after=%v", before, after)
		}

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

		for _, e := range logger.entries {
			if e.level == "warn" {
				t.Errorf("unexpected Warn under both-sides-empty: %+v", e)
			}
		}

		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, completionDebugFmt); got != 0 {
			t.Errorf("completion Debug count = %d, want 0; entries=%+v", got, logger.entries)
		}
	})

	t.Run("ListAllPanes error under returnError propagates as soft warning", func(t *testing.T) {
		seed := `{"a:0.0": {"on-resume": "cmd-a"}}`
		store, path := newTempHooksStore(t, seed)
		before := readFileBytes(t, path)

		sentinel := errors.New("tmux dead")
		logger := &recordingLogger{}
		lister := &stubAllPaneLister{panes: nil, err: sentinel}

		err := runHookStaleCleanup(lister, store, logger, returnError, nil)
		if err == nil {
			t.Fatalf("runHookStaleCleanup: want error, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("err = %v, want errors.Is == sentinel %v", err, sentinel)
		}

		after := readFileBytes(t, path)
		if !reflect.DeepEqual(before, after) {
			t.Errorf("hooks.json modified on ListAllPanes-error path: before=%q after=%q", before, after)
		}

		if got := countMatching(logger.entries, "warn", state.ComponentBootstrap, listPanesWarnFmt); got != 1 {
			t.Errorf("list-panes Warn count = %d, want 1; entries=%+v", got, logger.entries)
		}

		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, entryDebugFmt); got != 0 {
			t.Errorf("entry-point Debug count = %d, want 0 (must NOT fire on ListAllPanes-error branch); entries=%+v", got, logger.entries)
		}

		if got := countMatching(logger.entries, "warn", state.ComponentBootstrap, hazardWarnFmt); got != 0 {
			t.Errorf("hazard Warn count = %d, want 0 on ListAllPanes-error branch; entries=%+v", got, logger.entries)
		}
	})

	t.Run("ListAllPanes error under swallow logs Warn and returns nil", func(t *testing.T) {
		seed := `{"a:0.0": {"on-resume": "cmd-a"}}`
		store, path := newTempHooksStore(t, seed)
		before := readFileBytes(t, path)

		sentinel := errors.New("tmux dead")
		logger := &recordingLogger{}
		lister := &stubAllPaneLister{panes: nil, err: sentinel}

		err := runHookStaleCleanup(lister, store, logger, swallow, nil)
		if err != nil {
			t.Fatalf("runHookStaleCleanup under swallow: want nil err, got %v", err)
		}

		after := readFileBytes(t, path)
		if !reflect.DeepEqual(before, after) {
			t.Errorf("hooks.json modified on ListAllPanes-error swallow path: before=%q after=%q", before, after)
		}

		if got := countMatching(logger.entries, "warn", state.ComponentBootstrap, listPanesWarnFmt); got != 1 {
			t.Errorf("list-panes Warn count = %d, want 1 under swallow; entries=%+v", got, logger.entries)
		}

		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, entryDebugFmt); got != 0 {
			t.Errorf("entry-point Debug count = %d, want 0 under swallow; entries=%+v", got, logger.entries)
		}
	})

	t.Run("hookStore.Load error returns err with Warn", func(t *testing.T) {
		// Force a Load failure by pointing the store at a directory entry
		// (os.ReadFile fails with EISDIR). Use returnError policy because
		// the spec keeps Load-error as a returnable failure (not the
		// list-panes swallow policy).
		dir := t.TempDir()
		// Create a subdir at the would-be hooks.json path so ReadFile fails.
		bogusPath := filepath.Join(dir, "hooks.json")
		if err := os.MkdirAll(bogusPath, 0o755); err != nil {
			t.Fatalf("mkdir bogus path: %v", err)
		}
		store := hooks.NewStore(bogusPath)

		logger := &recordingLogger{}
		lister := &stubAllPaneLister{panes: []string{"a:0.0"}, err: nil}

		err := runHookStaleCleanup(lister, store, logger, returnError, nil)
		if err == nil {
			t.Fatalf("runHookStaleCleanup: want Load error, got nil")
		}

		if got := countMatching(logger.entries, "warn", state.ComponentBootstrap, loadWarnFmt); got != 1 {
			t.Errorf("hookStore.Load Warn count = %d, want 1; entries=%+v", got, logger.entries)
		}

		// Entry-point Debug MUST NOT fire when Load fails (terminal-Warn-only branch).
		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, entryDebugFmt); got != 0 {
			t.Errorf("entry-point Debug count = %d, want 0 on Load-error branch; entries=%+v", got, logger.entries)
		}
	})

	t.Run("onRemoved invoked once per removed entry", func(t *testing.T) {
		seed := `{
  "a:0.0": {"on-resume": "cmd-a"},
  "b:0.0": {"on-resume": "cmd-b"},
  "c:0.0": {"on-resume": "cmd-c"},
  "d:0.0": {"on-resume": "cmd-d"}
}`
		store, _ := newTempHooksStore(t, seed)

		logger := &recordingLogger{}
		lister := &stubAllPaneLister{panes: []string{"a:0.0"}, err: nil}

		var removedSeen []string
		onRemoved := func(name string) {
			removedSeen = append(removedSeen, name)
		}

		if err := runHookStaleCleanup(lister, store, logger, returnError, onRemoved); err != nil {
			t.Fatalf("runHookStaleCleanup: %v", err)
		}

		// b, c, d were stale; a survives. Order may vary because
		// store.CleanStale iterates a map; assert set-equality.
		want := map[string]struct{}{"b:0.0": {}, "c:0.0": {}, "d:0.0": {}}
		if len(removedSeen) != len(want) {
			t.Errorf("onRemoved invocations = %d (%v), want %d (%v)", len(removedSeen), removedSeen, len(want), want)
		}
		for _, k := range removedSeen {
			if _, ok := want[k]; !ok {
				t.Errorf("onRemoved invoked with unexpected key %q; want one of %v", k, want)
			}
		}
	})

	t.Run("nil onRemoved is safe under normal removal", func(t *testing.T) {
		seed := `{
  "a:0.0": {"on-resume": "cmd-a"},
  "b:0.0": {"on-resume": "cmd-b"}
}`
		store, _ := newTempHooksStore(t, seed)

		logger := &recordingLogger{}
		lister := &stubAllPaneLister{panes: []string{"a:0.0"}, err: nil}

		// Must not panic when onRemoved is nil and entries are removed.
		if err := runHookStaleCleanup(lister, store, logger, returnError, nil); err != nil {
			t.Fatalf("runHookStaleCleanup: %v", err)
		}

		if got := countMatching(logger.entries, "debug", state.ComponentBootstrap, completionDebugFmt); got != 1 {
			t.Errorf("completion Debug count = %d, want 1; entries=%+v", got, logger.entries)
		}
	})

	t.Run("happy-path normal removal emits entry + completion Debug", func(t *testing.T) {
		seed := `{
  "a:0.0": {"on-resume": "cmd-a"},
  "b:0.0": {"on-resume": "cmd-b"},
  "c:0.0": {"on-resume": "cmd-c"},
  "d:0.0": {"on-resume": "cmd-d"}
}`
		store, _ := newTempHooksStore(t, seed)

		logger := &recordingLogger{}
		lister := &stubAllPaneLister{panes: []string{"a:0.0", "b:0.0", "c:0.0"}, err: nil}

		if err := runHookStaleCleanup(lister, store, logger, returnError, nil); err != nil {
			t.Fatalf("runHookStaleCleanup: %v", err)
		}

		postRun, err := store.Load()
		if err != nil {
			t.Fatalf("store.Load post-run: %v", err)
		}
		wantKeys := map[string]struct{}{"a:0.0": {}, "b:0.0": {}, "c:0.0": {}}
		if len(postRun) != len(wantKeys) {
			t.Errorf("post-run hook count = %d (keys=%v), want %d", len(postRun), keysOf(postRun), len(wantKeys))
		}
		if _, ok := postRun["d:0.0"]; ok {
			t.Errorf("post-run hooks still contains stale key d:0.0; got %v", keysOf(postRun))
		}

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

		for _, e := range logger.entries {
			if e.level == "warn" {
				t.Errorf("unexpected Warn on normal-removal path: %+v", e)
			}
		}
	})

	t.Run("nil logger does not panic", func(t *testing.T) {
		// Helper must tolerate a nil logger per spec — adapters substitute
		// the noop logger before invoking the helper, but defence-in-depth
		// is cheap.
		seed := `{"a:0.0": {"on-resume": "cmd-a"}}`
		store, _ := newTempHooksStore(t, seed)
		lister := &stubAllPaneLister{panes: []string{"a:0.0"}, err: nil}

		if err := runHookStaleCleanup(lister, store, nil, returnError, nil); err != nil {
			t.Fatalf("runHookStaleCleanup with nil logger: %v", err)
		}
	})
}
