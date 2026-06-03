package tmux_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// dispatchUnregisterHooks builds a RunFunc that answers the per-event
// "show-hooks -g <event>" reads UnregisterPortalHooks issues, and records
// "set-hook -gu" calls. showOutput is the whole hook table (across every
// event); on each per-event read the helper returns ONLY the lines belonging
// to the queried event (argv[2]), mirroring how real tmux scopes
// `show-hooks -g <event>`. unsetErrFor, when non-nil, returns the configured
// error for matching event[index] targets; nil otherwise.
//
// It is a thin shim over perEventDispatchWithFaults (declared in
// hooks_register_test.go) — the single owner of the per-event read/unset
// skeleton AND its no-arg-global-read t.Fatalf guard. Routing teardown through
// the shared dispatcher means the teardown path now inherits that guard: a
// teardown regression that reverts to the blind no-arg global `show-hooks -g`
// read fails loudly instead of passing silently. The register-side
// `set-hook -ga` and read-fault channels are unused on the teardown path, so
// setHookErrFor and readErrFor are nil; unsetErrFor carries the per-index
// unset-fault injection. Teardown tests that DO need read-fault injection call
// perEventDispatchWithFaults directly (with readErrFor populated) rather than
// widening this shim's signature — see the read-failure cases in this file and
// hooks_unregister_warn_test.go.
func dispatchUnregisterHooks(t *testing.T, showOutput string, unsetErrFor map[string]error) func(args ...string) (string, error) {
	t.Helper()
	return perEventDispatchWithFaults(t, showOutput, nil, nil, unsetErrFor)
}

// linesForEvent returns only the lines of showOutput whose entry belongs to
// event (i.e. begin with "<event>["), preserving order. Reproduces the
// per-event scoping of `tmux show-hooks -g <event>` so a whole-table fixture
// can drive the per-event read loop. It is a thin lookup over
// parseSeededTableByEvent (the single line-scoping primitive, declared in
// hooks_register_test.go), which keys purely on the `<event>[` prefix so both
// the register (`<event>[i] => '...'`) and unregister
// (`<event>[i] run-shell '...'`) fixture shapes scope identically.
func linesForEvent(showOutput, event string) string {
	return parseSeededTableByEvent(showOutput)[event]
}

// unsetHookCalls extracts the set-hook -gu calls from a MockCommander's call
// log, in invocation order, returning each as the formatted target string
// (e.g. "session-created[0]").
func unsetHookCalls(calls [][]string) []string {
	var out []string
	for _, c := range calls {
		if len(c) >= 3 && c[0] == "set-hook" && c[1] == "-gu" {
			out = append(out, c[2])
		}
	}
	return out
}

func TestUnregisterPortalHooks(t *testing.T) {
	t.Run("removes a single Portal entry from an otherwise-empty array", func(t *testing.T) {
		raw := `session-created[0] => "run-shell \"command -v portal >/dev/null 2>&1 && portal state notify\""` + "\n"
		mock := &MockCommander{RunFunc: dispatchUnregisterHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		err := tmux.UnregisterPortalHooks(client)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := unsetHookCalls(mock.Calls)
		want := []string{"session-created[0]"}
		if len(got) != len(want) {
			t.Fatalf("set-hook -gu calls = %v, want %v", got, want)
		}
		for i, g := range got {
			if g != want[i] {
				t.Errorf("call[%d] = %q, want %q", i, g, want[i])
			}
		}
	})

	t.Run("removes interleaved Portal entries and leaves user entries in place", func(t *testing.T) {
		// session-created has Portal at indices 1 and 3, user entries at 0 and 2.
		raw := "session-created[0] run-shell 'tmux-resurrect save'\n" +
			"session-created[1] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n" +
			"session-created[2] run-shell 'user custom hook'\n" +
			"session-created[3] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n"
		mock := &MockCommander{RunFunc: dispatchUnregisterHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		err := tmux.UnregisterPortalHooks(client)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := unsetHookCalls(mock.Calls)
		// Reverse index order: 3 first, then 1. User indices 0 and 2 untouched.
		want := []string{"session-created[3]", "session-created[1]"}
		if len(got) != len(want) {
			t.Fatalf("set-hook -gu calls = %v, want %v", got, want)
		}
		for i, g := range got {
			if g != want[i] {
				t.Errorf("call[%d] = %q, want %q", i, g, want[i])
			}
		}
		// Defensive: ensure no calls were made for user indices.
		for _, g := range got {
			if g == "session-created[0]" || g == "session-created[2]" {
				t.Errorf("user entry incorrectly removed: %q", g)
			}
		}
	})

	t.Run("removes entries in reverse index order", func(t *testing.T) {
		// All Portal entries on a single event at indices 0, 2, 4.
		raw := "session-closed[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n" +
			"session-closed[2] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n" +
			"session-closed[4] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n"
		mock := &MockCommander{RunFunc: dispatchUnregisterHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		err := tmux.UnregisterPortalHooks(client)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := unsetHookCalls(mock.Calls)
		want := []string{"session-closed[4]", "session-closed[2]", "session-closed[0]"}
		if len(got) != len(want) {
			t.Fatalf("set-hook -gu calls = %v, want %v", got, want)
		}
		for i, g := range got {
			if g != want[i] {
				t.Errorf("call[%d] = %q, want %q", i, g, want[i])
			}
		}
	})

	t.Run("removes both Portal entries on session-renamed (notify and migrate-rename)", func(t *testing.T) {
		// session-renamed has BOTH a notify entry AND a migrate-rename entry.
		// Both must be removed in reverse index order.
		raw := "session-renamed[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n" +
			"session-renamed[1] run-shell 'command -v portal >/dev/null 2>&1 && portal state migrate-rename old new'\n"
		mock := &MockCommander{RunFunc: dispatchUnregisterHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		err := tmux.UnregisterPortalHooks(client)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := unsetHookCalls(mock.Calls)
		want := []string{"session-renamed[1]", "session-renamed[0]"}
		if len(got) != len(want) {
			t.Fatalf("set-hook -gu calls = %v, want %v", got, want)
		}
		for i, g := range got {
			if g != want[i] {
				t.Errorf("call[%d] = %q, want %q", i, g, want[i])
			}
		}
	})

	t.Run("is a no-op when no Portal entries are present", func(t *testing.T) {
		raw := "session-created[0] run-shell 'tmux-resurrect save'\n" +
			"session-closed[0] run-shell 'user-defined notify'\n"
		mock := &MockCommander{RunFunc: dispatchUnregisterHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		err := tmux.UnregisterPortalHooks(client)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := unsetHookCalls(mock.Calls); len(got) != 0 {
			t.Errorf("expected 0 set-hook -gu calls, got %d: %v", len(got), got)
		}
	})

	t.Run("ignores matching substrings on events outside Portal's event list", func(t *testing.T) {
		// window-renamed is NOT a Portal event. Even if its command body mentions
		// "portal state notify", removal must not target this event.
		raw := "window-renamed[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n" +
			"after-select-pane[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state migrate-rename a b'\n"
		mock := &MockCommander{RunFunc: dispatchUnregisterHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		err := tmux.UnregisterPortalHooks(client)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := unsetHookCalls(mock.Calls); len(got) != 0 {
			t.Errorf("expected 0 set-hook -gu calls (events outside portalEvents), got %d: %v", len(got), got)
		}
	})

	t.Run("returns the aggregate error and removes nothing when every per-event read fails", func(t *testing.T) {
		// Fold-and-continue replaces the old single-read all-or-nothing abort:
		// if every per-event read fails the joined error returns and, because no
		// event's entries are ever read, zero removals are issued.
		sentinel := errors.New("tmux exec failed")
		mock := &MockCommander{
			RunFunc: perEventDispatchWithFaults(t, "", nil, readErrForAllManagedEvents(sentinel), nil),
		}
		client := tmux.NewClient(mock)

		err := tmux.UnregisterPortalHooks(client)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
		}
		if !strings.Contains(err.Error(), "show-hooks failed") {
			t.Errorf("error %q does not contain expected wrap message %q", err.Error(), "show-hooks failed")
		}
		if got := unsetHookCalls(mock.Calls); len(got) != 0 {
			t.Errorf("expected 0 removals when every read fails, got %d: %v", len(got), got)
		}
	})

	t.Run("folds a single-event read failure into the aggregate and still reaps Portal entries on other events", func(t *testing.T) {
		// One event's read fails; every other event reads cleanly. The failing
		// event folds into the joined error while the Portal entries on the
		// readable events are still reaped — all-or-nothing is gone.
		sentinel := errors.New("transient show-hooks failure")
		raw := "session-created[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n" +
			"pane-focus-out[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n"
		mock := &MockCommander{
			RunFunc: perEventDispatchWithFaults(t, raw, nil, map[string]error{"pane-focus-out": sentinel}, nil),
		}
		client := tmux.NewClient(mock)

		err := tmux.UnregisterPortalHooks(client)

		if err == nil {
			t.Fatal("expected aggregate error from the failing event, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
		}
		if !strings.Contains(err.Error(), "show-hooks failed") {
			t.Errorf("error %q does not contain expected wrap message %q", err.Error(), "show-hooks failed")
		}
		// The readable event's Portal entry is still reaped despite the other
		// event's read failure.
		got := unsetHookCalls(mock.Calls)
		want := []string{"session-created[0]"}
		if len(got) != len(want) {
			t.Fatalf("set-hook -gu calls = %v, want %v (readable event still torn down)", got, want)
		}
		for i, g := range got {
			if g != want[i] {
				t.Errorf("call[%d] = %q, want %q", i, g, want[i])
			}
		}
	})

	t.Run("attempts every removal even when one set-hook -gu call fails", func(t *testing.T) {
		// Three Portal entries on session-created; the middle removal fails.
		raw := "session-created[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n" +
			"session-created[1] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n" +
			"session-created[2] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n"
		failures := map[string]error{
			"session-created[1]": errors.New("transient tmux failure"),
		}
		mock := &MockCommander{RunFunc: dispatchUnregisterHooks(t, raw, failures)}
		client := tmux.NewClient(mock)

		err := tmux.UnregisterPortalHooks(client)

		if err == nil {
			t.Fatal("expected aggregate error, got nil")
		}
		got := unsetHookCalls(mock.Calls)
		// Must attempt all three removals in reverse order even though the middle one fails.
		want := []string{"session-created[2]", "session-created[1]", "session-created[0]"}
		if len(got) != len(want) {
			t.Fatalf("set-hook -gu calls = %v, want %v (every removal attempted)", got, want)
		}
		for i, g := range got {
			if g != want[i] {
				t.Errorf("call[%d] = %q, want %q", i, g, want[i])
			}
		}
	})

	t.Run("returns a joined error naming every failed index", func(t *testing.T) {
		sentinelA := errors.New("tmux failure A")
		sentinelB := errors.New("tmux failure B")
		raw := "session-created[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n" +
			"client-attached[2] run-shell 'command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}'\n"
		failures := map[string]error{
			"session-created[0]": sentinelA,
			"client-attached[2]": sentinelB,
		}
		mock := &MockCommander{RunFunc: dispatchUnregisterHooks(t, raw, failures)}
		client := tmux.NewClient(mock)

		err := tmux.UnregisterPortalHooks(client)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, sentinelA) {
			t.Errorf("error %v does not wrap sentinelA %v", err, sentinelA)
		}
		if !errors.Is(err, sentinelB) {
			t.Errorf("error %v does not wrap sentinelB %v", err, sentinelB)
		}
		if !strings.Contains(err.Error(), "session-created") {
			t.Errorf("error %q does not name failed event session-created", err.Error())
		}
		if !strings.Contains(err.Error(), "client-attached") {
			t.Errorf("error %q does not name failed event client-attached", err.Error())
		}
		// Sanity: error string mentions both indices.
		if !strings.Contains(err.Error(), "[0]") || !strings.Contains(err.Error(), "[2]") {
			t.Errorf("error %q does not name both indices", err.Error())
		}
	})

	t.Run("idempotent: a second run after a successful removal does nothing", func(t *testing.T) {
		// Stateful mock: first run sees a Portal entry and removes it; subsequent
		// show-hooks returns empty.
		var removed bool
		runFunc := func(args ...string) (string, error) {
			if len(args) >= 3 && args[0] == "show-hooks" && args[1] == "-g" {
				if removed {
					return "", nil
				}
				return linesForEvent(
					"session-created[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n",
					args[2],
				), nil
			}
			if len(args) >= 3 && args[0] == "set-hook" && args[1] == "-gu" {
				removed = true
				return "", nil
			}
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
		mock := &MockCommander{RunFunc: runFunc}
		client := tmux.NewClient(mock)

		if err := tmux.UnregisterPortalHooks(client); err != nil {
			t.Fatalf("first run: unexpected error: %v", err)
		}
		firstRunRemovals := len(unsetHookCalls(mock.Calls))
		if firstRunRemovals != 1 {
			t.Fatalf("first run set-hook -gu count = %d, want 1", firstRunRemovals)
		}

		mock.Calls = nil
		if err := tmux.UnregisterPortalHooks(client); err != nil {
			t.Fatalf("second run: unexpected error: %v", err)
		}
		if got := unsetHookCalls(mock.Calls); len(got) != 0 {
			t.Errorf("second run produced %d removals, want 0 (idempotent): %v", len(got), got)
		}
	})

	t.Run("does not match user entries that mention 'portal' but not Portal commands", func(t *testing.T) {
		// User-authored hook that references portal in a comment / message but is
		// NOT one of Portal's command substrings. Must be left alone.
		raw := "session-created[0] run-shell 'echo my portal is open'\n" +
			"session-closed[0] run-shell 'echo migrating my portal'\n"
		mock := &MockCommander{RunFunc: dispatchUnregisterHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		err := tmux.UnregisterPortalHooks(client)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := unsetHookCalls(mock.Calls); len(got) != 0 {
			t.Errorf("expected 0 set-hook -gu calls (no Portal command substrings present), got %d: %v",
				len(got), got)
		}
	})

	// Defensive sanity: ensure the dispatch helper is wired correctly.
	t.Run("integration smoke: removes Portal entries across multiple Portal events", func(t *testing.T) {
		raw := "session-created[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n" +
			"client-attached[1] run-shell 'command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}'\n" +
			"session-renamed[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n" +
			"session-renamed[1] run-shell 'command -v portal >/dev/null 2>&1 && portal state migrate-rename a b'\n"
		mock := &MockCommander{RunFunc: dispatchUnregisterHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		if err := tmux.UnregisterPortalHooks(client); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := unsetHookCalls(mock.Calls)
		// Order across events follows portalEvents (save events first, then hydration);
		// within each event, reverse index order.
		want := []string{
			"session-created[0]",
			"session-renamed[1]",
			"session-renamed[0]",
			"client-attached[1]",
		}
		if len(got) != len(want) {
			t.Fatalf("set-hook -gu calls = %v, want %v", got, want)
		}
		for i, g := range got {
			if g != want[i] {
				t.Errorf("call[%d] = %q, want %q", i, g, want[i])
			}
		}
	})

}
