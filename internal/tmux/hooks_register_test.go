package tmux_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// expectedSaveTriggerEvents is the canonical save-trigger event list, in
// registration order. Mirrors saveTriggerEvents in hooks_register.go; kept
// here so tests can assert order without exporting the internal slice.
var expectedSaveTriggerEvents = []string{
	"session-created",
	"session-closed",
	"session-renamed",
	"window-linked",
	"window-unlinked",
	"window-layout-changed",
	"pane-focus-out",
}

// expectedNotifyCommand is the exact full command Portal registers on each of
// the six non-session-closed save-trigger events. Mirrors notifyCommand in
// hooks_register.go. The seventh save-trigger event (session-closed) is on
// commitNowCommand following the killed-session-resurrects-within-tick-window
// migration; see expectedCommitNowCommand.
const expectedNotifyCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`

// expectedCommitNowCommand is the exact full command Portal registers on
// session-closed following the killed-session-resurrects-within-tick-window
// migration. Mirrors commitNowCommand in hooks_register.go. session-closed is
// the single tmux-side seam that fires uniformly across every kill path; this
// command invokes a synchronous sessions.json commit to close the resurrection
// window between kill and the daemon's next tick.
const expectedCommitNowCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state commit-now"`

// expectedSignalHydrateCommand is the exact full command Portal registers on
// every hydration-trigger event. Mirrors signalHydrateCommand in
// hooks_register.go. The literal #{session_name} is preserved verbatim — tmux
// expands it at hook-fire time. The `--` end-of-flags separator before
// #{session_name} prevents cobra/pflag from misparsing leading-dash session
// names (e.g. `-dotfiles-HM9Zhw`) as short-flag clusters.
const expectedSignalHydrateCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate -- #{session_name}"`

// The migrate-rename hook category was deferred to v2 (see hooks_register.go
// note). RegisterPortalHooks ships only the two surviving categories —
// save-trigger and hydration-trigger — so no migrate-rename test fixtures
// live here.

// dispatchPortalHooks builds a RunFunc that returns showOutput for every
// "show-hooks -g" call and records "set-hook -ga" / "set-hook -gu" calls.
// setHookErrFor, when non-nil, returns the configured error for matching
// events on -ga; nil otherwise. -gu (unset) calls succeed by default — tests
// exercising unset failures use their own RunFunc.
func dispatchPortalHooks(t *testing.T, showOutput string, setHookErrFor map[string]error) func(args ...string) (string, error) {
	t.Helper()
	return func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
			return showOutput, nil
		}
		if len(args) >= 4 && args[0] == "set-hook" && args[1] == "-ga" {
			if setHookErrFor != nil {
				if err, ok := setHookErrFor[args[2]]; ok {
					return "", err
				}
			}
			return "", nil
		}
		if len(args) >= 3 && args[0] == "set-hook" && args[1] == "-gu" {
			return "", nil
		}
		t.Fatalf("unexpected command: %v", args)
		return "", nil
	}
}

// unsetHookCalls (set-hook -gu extractor) is shared with hooks_unregister_test.go
// in the same package — see that file for the definition.

// setHookCalls extracts the set-hook -ga calls from a MockCommander's call
// log, in invocation order, returning each as [event, command].
func setHookCalls(calls [][]string) [][2]string {
	var out [][2]string
	for _, c := range calls {
		if len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga" {
			out = append(out, [2]string{c[2], c[3]})
		}
	}
	return out
}

// allPortalHooksRegisteredOutput builds a show-hooks -g output that contains
// a Portal entry for every save-trigger and hydration-trigger event in their
// post-migration shape: notifyCommand on the six non-session-closed
// save-trigger events, commitNowCommand on session-closed, and
// signalHydrateCommand on the two hydration-trigger events. The migrate-rename
// category was deferred to v2 and is intentionally absent.
//
// Each line is single-outer-quoted to mirror tmux's actual show-hooks output
// format: the Portal-emitted command bodies contain literal double quotes
// (e.g. `run-shell "..."`), and tmux wraps them with single quotes to
// preserve the inner double quotes verbatim. ParseShowHooks strips the outer
// single quotes, so the parsed command equals the original literal — which
// is what the migration's exact-string match relies on.
func allPortalHooksRegisteredOutput() string {
	var b strings.Builder
	for _, e := range expectedSaveTriggerEvents {
		cmd := expectedNotifyCommand
		if e == "session-closed" {
			cmd = expectedCommitNowCommand
		}
		fmt.Fprintf(&b, "%s[0] => '%s'\n", e, cmd)
	}
	for _, e := range tmux.HydrationTriggerEvents {
		fmt.Fprintf(&b, "%s[0] => '%s'\n", e, expectedSignalHydrateCommand)
	}
	return b.String()
}

// dispatchShowHooks builds a RunFunc that returns showOutput for "show-hooks -g"
// and dispatches "set-hook -ga" calls to setHookErr (or nil).
func dispatchShowHooks(t *testing.T, showOutput string, setHookErr error) func(args ...string) (string, error) {
	t.Helper()
	return func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
			return showOutput, nil
		}
		if len(args) >= 2 && args[0] == "set-hook" && args[1] == "-ga" {
			return "", setHookErr
		}
		t.Fatalf("unexpected command: %v", args)
		return "", nil
	}
}

// TestSignalHydrateCommand_HasEndOfFlagsSeparator pins the shape of the two
// hydration-trigger constants. The `--` end-of-flags separator before
// #{session_name} is load-bearing: leading-dash session names (which arise
// when SanitiseProjectName substitutes `.` -> `-` for projects whose basename
// starts with `.`) would otherwise be parsed by cobra/pflag as short-flag
// clusters and the hook would exit non-zero before runSignalHydrate runs.
//
// The dedupe substring must include `--` so RegisterHookIfAbsent distinguishes
// the new fixed entry from any pre-existing un-separated entry left behind by
// an older portal install.
func TestSignalHydrateCommand_HasEndOfFlagsSeparator(t *testing.T) {
	t.Run("signalHydrateCommand resolves with -- before #{session_name}", func(t *testing.T) {
		want := `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate -- #{session_name}"`
		if expectedSignalHydrateCommand != want {
			t.Errorf("expectedSignalHydrateCommand = %q, want %q", expectedSignalHydrateCommand, want)
		}
		if !strings.Contains(expectedSignalHydrateCommand, " -- #{session_name}") {
			t.Errorf("expectedSignalHydrateCommand %q missing ` -- #{session_name}` separator", expectedSignalHydrateCommand)
		}
	})

	t.Run("RegisterPortalHooks emits the -- separator on every hydration event", func(t *testing.T) {
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := setHookCalls(mock.Calls)
		base := len(expectedSaveTriggerEvents)
		for i := range tmux.HydrationTriggerEvents {
			cmd := got[base+i][1]
			if !strings.Contains(cmd, "portal state signal-hydrate -- #{session_name}") {
				t.Errorf("hydration call[%d] command = %q, missing `signal-hydrate -- #{session_name}`", base+i, cmd)
			}
		}
	})
}

func TestRegisterHookIfAbsent(t *testing.T) {
	t.Run("skips append when Portal entry already present on the target event", func(t *testing.T) {
		raw := "session-created[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n"
		mock := &MockCommander{RunFunc: dispatchShowHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		err := tmux.RegisterHookIfAbsent(client, "session-created", "portal state notify",
			`run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'`)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mock.Calls) != 1 {
			t.Errorf("expected 1 call (show-hooks only), got %d: %v", len(mock.Calls), mock.Calls)
		}
		if len(mock.Calls) > 0 && mock.Calls[0][0] != "show-hooks" {
			t.Errorf("expected first call to be show-hooks, got %v", mock.Calls[0])
		}
	})

	t.Run("appends when target event array is empty", func(t *testing.T) {
		// show-hooks output contains entries for OTHER events but none for our target.
		raw := "client-attached[0] run-shell 'something else'\n"
		mock := &MockCommander{RunFunc: dispatchShowHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		fullCmd := `run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'`
		err := tmux.RegisterHookIfAbsent(client, "session-created", "portal state notify", fullCmd)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mock.Calls) != 2 {
			t.Fatalf("expected 2 calls (show-hooks + set-hook), got %d: %v", len(mock.Calls), mock.Calls)
		}
		got := mock.Calls[1]
		want := []string{"set-hook", "-ga", "session-created", fullCmd}
		if len(got) != len(want) {
			t.Fatalf("set-hook args = %v, want %v", got, want)
		}
		for i, arg := range got {
			if arg != want[i] {
				t.Errorf("set-hook arg[%d] = %q, want %q", i, arg, want[i])
			}
		}
	})

	t.Run("appends when target event has only non-Portal entries", func(t *testing.T) {
		raw := "session-created[0] run-shell 'tmux-resurrect save'\n" +
			"session-created[1] run-shell 'some-plugin do-thing'\n"
		mock := &MockCommander{RunFunc: dispatchShowHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		fullCmd := `run-shell 'portal state notify'`
		err := tmux.RegisterHookIfAbsent(client, "session-created", "portal state notify", fullCmd)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mock.Calls) != 2 {
			t.Fatalf("expected 2 calls, got %d: %v", len(mock.Calls), mock.Calls)
		}
		if mock.Calls[1][0] != "set-hook" || mock.Calls[1][1] != "-ga" || mock.Calls[1][2] != "session-created" {
			t.Errorf("expected set-hook -ga session-created, got %v", mock.Calls[1])
		}
		if mock.Calls[1][3] != fullCmd {
			t.Errorf("set-hook command = %q, want %q", mock.Calls[1][3], fullCmd)
		}
	})

	t.Run("does not skip when matching substring lives on a different event", func(t *testing.T) {
		// 'portal state notify' is registered on session-closed but we are
		// asked to register for session-created. Per-event scoping must NOT
		// suppress the append.
		raw := "session-closed[0] run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'\n"
		mock := &MockCommander{RunFunc: dispatchShowHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		fullCmd := `run-shell 'portal state notify'`
		err := tmux.RegisterHookIfAbsent(client, "session-created", "portal state notify", fullCmd)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mock.Calls) != 2 {
			t.Fatalf("expected 2 calls (per-event scoping), got %d: %v", len(mock.Calls), mock.Calls)
		}
		if mock.Calls[1][2] != "session-created" {
			t.Errorf("set-hook event = %q, want session-created", mock.Calls[1][2])
		}
	})

	t.Run("leaves unrelated user/plugin entries in place when appending", func(t *testing.T) {
		// This is the "no rewrite/delete in this layer" check: we only ever
		// emit a set-hook -ga (append). No set-hook -gu, no set-hook -g (overwrite).
		raw := "session-created[0] run-shell 'tmux-resurrect save'\n" +
			"session-created[1] run-shell 'user custom hook'\n"
		mock := &MockCommander{RunFunc: dispatchShowHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		err := tmux.RegisterHookIfAbsent(client, "session-created", "portal state notify",
			`run-shell 'portal state notify'`)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mock.Calls) != 2 {
			t.Fatalf("expected 2 calls, got %d: %v", len(mock.Calls), mock.Calls)
		}
		// Ensure the only mutating call is an append (-ga), never -gu or plain -g.
		mutating := mock.Calls[1]
		if mutating[0] != "set-hook" {
			t.Fatalf("expected set-hook, got %q", mutating[0])
		}
		if mutating[1] != "-ga" {
			t.Errorf("expected -ga (append), got %q — must not rewrite or delete", mutating[1])
		}
	})

	t.Run("propagates show-hooks failure without attempting an append", func(t *testing.T) {
		sentinel := errors.New("tmux exec failed")
		mock := &MockCommander{
			RunFunc: func(args ...string) (string, error) {
				if args[0] == "show-hooks" {
					return "", sentinel
				}
				t.Fatalf("set-hook must not be called when show-hooks fails: %v", args)
				return "", nil
			},
		}
		client := tmux.NewClient(mock)

		err := tmux.RegisterHookIfAbsent(client, "session-created", "portal state notify",
			`run-shell 'portal state notify'`)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
		}
		if !strings.Contains(err.Error(), "show-hooks failed") {
			t.Errorf("error %q does not contain expected wrap message %q", err.Error(), "show-hooks failed")
		}
		if len(mock.Calls) != 1 {
			t.Errorf("expected exactly 1 call (show-hooks only), got %d: %v", len(mock.Calls), mock.Calls)
		}
	})

	t.Run("propagates set-hook -ga failure to the caller", func(t *testing.T) {
		sentinel := errors.New("tmux append failed")
		mock := &MockCommander{RunFunc: dispatchShowHooks(t, "", sentinel)}
		client := tmux.NewClient(mock)

		err := tmux.RegisterHookIfAbsent(client, "session-created", "portal state notify",
			`run-shell 'portal state notify'`)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
		}
	})

	t.Run("recognises a Portal entry regardless of tmux's outer quoting", func(t *testing.T) {
		// tmux may render the stored command surrounded by either single or
		// double quotes. ParseShowHooks strips matched outer quoting, so the
		// substring check must hit in both cases.
		cases := []struct {
			name string
			raw  string
		}{
			{
				name: "double-quoted outer",
				raw:  `session-created[0] => "run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'"` + "\n",
			},
			{
				name: "single-quoted outer",
				raw:  `session-created[0] => 'run-shell "command -v portal >/dev/null 2>&1 && portal state notify"'` + "\n",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				mock := &MockCommander{
					RunFunc: func(args ...string) (string, error) {
						if args[0] == "show-hooks" {
							return tc.raw, nil
						}
						t.Fatalf("set-hook must not be called when entry already present: %v", args)
						return "", nil
					},
				}
				client := tmux.NewClient(mock)

				err := tmux.RegisterHookIfAbsent(client, "session-created", "portal state notify",
					`run-shell 'portal state notify'`)

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(mock.Calls) != 1 {
					t.Errorf("expected 1 call (show-hooks only), got %d: %v", len(mock.Calls), mock.Calls)
				}
			})
		}
	})
}

func TestRegisterPortalHooks(t *testing.T) {
	t.Run("it registers all nine Portal hooks on a fresh server", func(t *testing.T) {
		// Two categories survive in v1: save-trigger (7) + hydration-trigger
		// (2) = 9 appends. The migrate-rename category was deferred to v2.
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
		client := tmux.NewClient(mock)

		err := tmux.RegisterPortalHooks(client, nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := setHookCalls(mock.Calls)
		want := len(expectedSaveTriggerEvents) + len(tmux.HydrationTriggerEvents)
		if len(got) != want {
			t.Fatalf("set-hook -ga call count = %d, want %d: %v", len(got), want, got)
		}
	})

	t.Run("it registers hooks in the documented order", func(t *testing.T) {
		// Save-trigger events first, then hydration-trigger events. The
		// migrate-rename category is deferred to v2 and is not registered.
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := setHookCalls(mock.Calls)
		want := append([]string{}, expectedSaveTriggerEvents...)
		want = append(want, tmux.HydrationTriggerEvents...)

		if len(got) != len(want) {
			t.Fatalf("set-hook count = %d, want %d (got %v)", len(got), len(want), got)
		}
		for i, g := range got {
			if g[0] != want[i] {
				t.Errorf("set-hook[%d] event = %q, want %q", i, g[0], want[i])
			}
		}
	})

	t.Run("it skips all appends when every Portal hook is already present", func(t *testing.T) {
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, allPortalHooksRegisteredOutput(), nil)}
		client := tmux.NewClient(mock)

		err := tmux.RegisterPortalHooks(client, nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := setHookCalls(mock.Calls); len(got) != 0 {
			t.Errorf("expected 0 set-hook -ga calls (all present), got %d: %v", len(got), got)
		}
	})

	t.Run("it tops up only the missing events on a partially-registered server", func(t *testing.T) {
		// Five save-trigger events present (using their post-migration command
		// shapes — commitNowCommand on session-closed, notifyCommand on the
		// others), two missing (window-unlinked and pane-focus-out). Both
		// hydration events present. The only top-ups are the two missing
		// save-trigger events.
		var b strings.Builder
		present := map[string]bool{
			"session-created":       true,
			"session-closed":        true,
			"session-renamed":       true,
			"window-linked":         true,
			"window-layout-changed": true,
		}
		for _, e := range expectedSaveTriggerEvents {
			if !present[e] {
				continue
			}
			cmd := expectedNotifyCommand
			if e == "session-closed" {
				cmd = expectedCommitNowCommand
			}
			fmt.Fprintf(&b, "%s[0] => '%s'\n", e, cmd)
		}
		for _, e := range tmux.HydrationTriggerEvents {
			fmt.Fprintf(&b, "%s[0] => '%s'\n", e, expectedSignalHydrateCommand)
		}

		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, b.String(), nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := setHookCalls(mock.Calls)
		wantEvents := []string{"window-unlinked", "pane-focus-out"}
		if len(got) != len(wantEvents) {
			t.Fatalf("set-hook count = %d, want %d (got %v)", len(got), len(wantEvents), got)
		}
		for i, g := range got {
			if g[0] != wantEvents[i] {
				t.Errorf("set-hook[%d] event = %q, want %q", i, g[0], wantEvents[i])
			}
		}
	})

	t.Run("it attempts every event even if one set-hook -ga call fails", func(t *testing.T) {
		// Fresh server: 9 events would be appended (7 save-trigger + 2
		// hydration-trigger; migrate-rename deferred to v2). We make
		// set-hook fail for two specific events. RegisterPortalHooks must
		// still attempt every event (9 set-hook calls in total).
		failures := map[string]error{
			"session-renamed":        errors.New("transient tmux failure"),
			"client-session-changed": errors.New("another transient failure"),
		}
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", failures)}
		client := tmux.NewClient(mock)

		err := tmux.RegisterPortalHooks(client, nil)

		if err == nil {
			t.Fatal("expected aggregate error, got nil")
		}
		got := setHookCalls(mock.Calls)
		want := len(expectedSaveTriggerEvents) + len(tmux.HydrationTriggerEvents)
		if len(got) != want {
			t.Errorf("set-hook -ga call count = %d, want %d (every event attempted): %v", len(got), want, got)
		}
	})

	t.Run("it returns a joined error naming every failed event", func(t *testing.T) {
		sentinelA := errors.New("tmux failure A")
		sentinelB := errors.New("tmux failure B")
		failures := map[string]error{
			"session-renamed":        sentinelA,
			"client-session-changed": sentinelB,
		}
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", failures)}
		client := tmux.NewClient(mock)

		err := tmux.RegisterPortalHooks(client, nil)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, sentinelA) {
			t.Errorf("error %v does not wrap sentinelA %v", err, sentinelA)
		}
		if !errors.Is(err, sentinelB) {
			t.Errorf("error %v does not wrap sentinelB %v", err, sentinelB)
		}
		if !strings.Contains(err.Error(), "session-renamed") {
			t.Errorf("error %q does not name failed event session-renamed", err.Error())
		}
		if !strings.Contains(err.Error(), "client-session-changed") {
			t.Errorf("error %q does not name failed event client-session-changed", err.Error())
		}
	})

	t.Run("it does not double-register on two consecutive bootstraps in the same process", func(t *testing.T) {
		// Simulate a stateful tmux: first bootstrap on empty show-hooks
		// registers all 9. Second bootstrap sees those 9 in show-hooks
		// output and registers nothing. The show-hooks rendering uses
		// single-outer-quote wrapping to mirror tmux's real output format,
		// so ParseShowHooks recovers the stored command verbatim and the
		// session-closed migration's exact-string match hits cleanly.
		var registered [][2]string // (event, command) in registration order
		runFunc := func(args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
				var b strings.Builder
				perEventCount := map[string]int{}
				for _, entry := range registered {
					ev, cmd := entry[0], entry[1]
					idx := perEventCount[ev]
					perEventCount[ev] = idx + 1
					fmt.Fprintf(&b, "%s[%d] => '%s'\n", ev, idx, cmd)
				}
				return b.String(), nil
			}
			if len(args) >= 4 && args[0] == "set-hook" && args[1] == "-ga" {
				registered = append(registered, [2]string{args[2], args[3]})
				return "", nil
			}
			if len(args) >= 3 && args[0] == "set-hook" && args[1] == "-gu" {
				return "", nil
			}
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
		mock := &MockCommander{RunFunc: runFunc}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("first bootstrap: unexpected error: %v", err)
		}
		firstBootstrapAppends := len(setHookCalls(mock.Calls))
		want := len(expectedSaveTriggerEvents) + len(tmux.HydrationTriggerEvents)
		if firstBootstrapAppends != want {
			t.Fatalf("first bootstrap set-hook count = %d, want %d", firstBootstrapAppends, want)
		}

		// Reset call log; run again.
		mock.Calls = nil
		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("second bootstrap: unexpected error: %v", err)
		}
		if got := setHookCalls(mock.Calls); len(got) != 0 {
			t.Errorf("second bootstrap appended %d entries, want 0 (idempotent): %v", len(got), got)
		}
	})

	t.Run("it wraps each save-trigger event's command with command -v portal guard", func(t *testing.T) {
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := setHookCalls(mock.Calls)
		// First seven calls correspond to save-trigger events. session-closed
		// receives commitNowCommand following the
		// killed-session-resurrects-within-tick-window migration; the other
		// six save-trigger events receive notifyCommand unchanged.
		for i, ev := range expectedSaveTriggerEvents {
			if got[i][0] != ev {
				t.Errorf("call[%d] event = %q, want %q", i, got[i][0], ev)
			}
			wantCmd := expectedNotifyCommand
			if ev == "session-closed" {
				wantCmd = expectedCommitNowCommand
			}
			if got[i][1] != wantCmd {
				t.Errorf("call[%d] command = %q, want %q", i, got[i][1], wantCmd)
			}
		}
	})

	t.Run("it wraps each hydration-trigger event's command with command -v portal guard and #{session_name}", func(t *testing.T) {
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := setHookCalls(mock.Calls)
		// The two hydration-trigger calls sit immediately after the
		// save-trigger calls in the current 9-hook registration order.
		base := len(expectedSaveTriggerEvents)
		for i, ev := range tmux.HydrationTriggerEvents {
			idx := base + i
			if got[idx][0] != ev {
				t.Errorf("call[%d] event = %q, want %q", idx, got[idx][0], ev)
			}
			if got[idx][1] != expectedSignalHydrateCommand {
				t.Errorf("call[%d] command = %q, want %q", idx, got[idx][1], expectedSignalHydrateCommand)
			}
			if !strings.Contains(got[idx][1], "#{session_name}") {
				t.Errorf("call[%d] command = %q, missing literal #{session_name}", idx, got[idx][1])
			}
		}
	})

	t.Run("it registers only the two surviving categories — no migrate-rename hook (v2 deferral)", func(t *testing.T) {
		// Path (b) deferral: the rename-key migration hook is not registered in
		// v1 because tmux's session-renamed event does not reliably expose the
		// prior session name. Daemon-side last-name tracking is post-v1.
		// RegisterPortalHooks must therefore register exactly two categories:
		// save-trigger (notify) and hydration-trigger (signal-hydrate). No
		// "portal state migrate-rename" command may appear in any append.
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := setHookCalls(mock.Calls)
		wantCount := len(expectedSaveTriggerEvents) + len(tmux.HydrationTriggerEvents)
		if len(got) != wantCount {
			t.Fatalf("set-hook -ga call count = %d, want %d (only two categories: %d save + %d hydrate)",
				len(got), wantCount, len(expectedSaveTriggerEvents), len(tmux.HydrationTriggerEvents))
		}
		for _, c := range got {
			if strings.Contains(c[1], "portal state migrate-rename") {
				t.Errorf("unexpected migrate-rename registration on event %q: %q", c[0], c[1])
			}
		}
		// Sanity: every command body is exactly one of the three Portal
		// command literals. commitNowCommand appears only on session-closed;
		// notifyCommand on the six other save-trigger events;
		// signalHydrateCommand on the two hydration-trigger events.
		for _, c := range got {
			if c[1] != expectedNotifyCommand && c[1] != expectedSignalHydrateCommand && c[1] != expectedCommitNowCommand {
				t.Errorf("unexpected command body on event %q: %q", c[0], c[1])
			}
		}
	})
}

// recordingMigrationLogger is a slog.Handler that captures Info / Warn
// records for assertion in migration-flow tests. Each captured record is
// rendered as "[<component>] <message>" so the pre-migration assertions keep
// working against the post-migration terse-message-plus-component-attr shape.
// Use Logger() to obtain a *slog.Logger to pass into RegisterPortalHooks.
type recordingMigrationLogger struct {
	infos []string
	warns []string
	// shared points at the slice-owning recorder so handlers derived via
	// WithAttrs/WithGroup (notably .With("component", ...)) record into the
	// same slices; nil on the root.
	shared *recordingMigrationLogger
	bound  []slog.Attr
}

// Logger returns a *slog.Logger whose records are captured by this recorder.
func (r *recordingMigrationLogger) Logger() *slog.Logger { return slog.New(r) }

func (r *recordingMigrationLogger) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (r *recordingMigrationLogger) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(r.bound)+len(attrs))
	next = append(next, r.bound...)
	next = append(next, attrs...)
	return &recordingMigrationLogger{shared: r.owner(), bound: next}
}

func (r *recordingMigrationLogger) WithGroup(_ string) slog.Handler {
	return &recordingMigrationLogger{shared: r.owner(), bound: r.bound}
}

func (r *recordingMigrationLogger) owner() *recordingMigrationLogger {
	if r.shared != nil {
		return r.shared
	}
	return r
}

func (r *recordingMigrationLogger) Handle(_ context.Context, rec slog.Record) error {
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
	line := "[" + component + "] " + rec.Message
	owner := r.owner()
	switch rec.Level {
	case slog.LevelInfo:
		owner.infos = append(owner.infos, line)
	case slog.LevelWarn:
		owner.warns = append(owner.warns, line)
	}
	return nil
}

// nonSessionClosedSaveTriggerEvents is the canonical save-trigger event list
// minus session-closed — the six events that retain the notifyCommand append-
// if-absent registration after the migration. Mirrors the implicit split in
// hooks_register.go.
var nonSessionClosedSaveTriggerEvents = []string{
	"session-created",
	"session-renamed",
	"window-linked",
	"window-unlinked",
	"window-layout-changed",
	"pane-focus-out",
}

func TestRegisterPortalHooks_SessionClosedMigration(t *testing.T) {
	t.Run("it registers commitNowCommand on session-closed from an empty hook state", func(t *testing.T) {
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := setHookCalls(mock.Calls)
		var sessionClosedCalls [][2]string
		for _, c := range got {
			if c[0] == "session-closed" {
				sessionClosedCalls = append(sessionClosedCalls, c)
			}
		}
		if len(sessionClosedCalls) != 1 {
			t.Fatalf("expected exactly 1 set-hook -ga on session-closed, got %d: %v", len(sessionClosedCalls), sessionClosedCalls)
		}
		if sessionClosedCalls[0][1] != expectedCommitNowCommand {
			t.Errorf("session-closed command = %q, want %q", sessionClosedCalls[0][1], expectedCommitNowCommand)
		}
		// No unset calls should fire against an empty hook state.
		if unsets := unsetHookCalls(mock.Calls); len(unsets) != 0 {
			t.Errorf("expected 0 set-hook -gu calls on empty state, got %d: %v", len(unsets), unsets)
		}
	})

	t.Run("it registers notifyCommand on each of the six other save-trigger events from an empty hook state", func(t *testing.T) {
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := setHookCalls(mock.Calls)
		seen := map[string]string{}
		for _, c := range got {
			if c[0] == "session-closed" {
				continue
			}
			// Skip hydration-trigger events; they belong to a different category.
			isHydration := false
			for _, h := range tmux.HydrationTriggerEvents {
				if c[0] == h {
					isHydration = true
					break
				}
			}
			if isHydration {
				continue
			}
			if prior, dup := seen[c[0]]; dup {
				t.Errorf("event %q appended twice: prior=%q now=%q", c[0], prior, c[1])
			}
			seen[c[0]] = c[1]
		}
		for _, ev := range nonSessionClosedSaveTriggerEvents {
			cmd, ok := seen[ev]
			if !ok {
				t.Errorf("expected one set-hook -ga on %q, none recorded", ev)
				continue
			}
			if cmd != expectedNotifyCommand {
				t.Errorf("event %q command = %q, want %q", ev, cmd, expectedNotifyCommand)
			}
		}
	})

	t.Run("it removes a stale notifyCommand from session-closed during a pre-fix upgrade", func(t *testing.T) {
		// Pre-fix state: only the stale notifyCommand sits on session-closed.
		// Single-outer-quote wrapping mirrors tmux's real show-hooks format —
		// the body contains literal double quotes so tmux wraps it in singles.
		raw := fmt.Sprintf("session-closed[0] => '%s'\n", expectedNotifyCommand)
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		unsets := unsetHookCalls(mock.Calls)
		// Exactly one unset on session-closed[0] should fire for the stale entry.
		var sessionClosedUnsets []string
		for _, u := range unsets {
			if strings.HasPrefix(u, "session-closed[") {
				sessionClosedUnsets = append(sessionClosedUnsets, u)
			}
		}
		if len(sessionClosedUnsets) != 1 {
			t.Fatalf("expected 1 set-hook -gu on session-closed, got %d: %v", len(sessionClosedUnsets), sessionClosedUnsets)
		}
		if sessionClosedUnsets[0] != "session-closed[0]" {
			t.Errorf("unset target = %q, want %q", sessionClosedUnsets[0], "session-closed[0]")
		}
	})

	t.Run("it registers commitNowCommand after removing the stale notifyCommand on session-closed", func(t *testing.T) {
		raw := fmt.Sprintf("session-closed[0] => '%s'\n", expectedNotifyCommand)
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// The session-closed append must follow the unset in invocation order
		// — order matters so the migration is observable as "evict, then
		// register" rather than "register, then evict" (which would create a
		// transient duplicate).
		unsetIdx, appendIdx := -1, -1
		for i, c := range mock.Calls {
			if len(c) >= 3 && c[0] == "set-hook" && c[1] == "-gu" && c[2] == "session-closed[0]" {
				unsetIdx = i
			}
			if len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga" && c[2] == "session-closed" && c[3] == expectedCommitNowCommand {
				appendIdx = i
			}
		}
		if unsetIdx < 0 {
			t.Fatalf("expected set-hook -gu session-closed[0], not found in calls: %v", mock.Calls)
		}
		if appendIdx < 0 {
			t.Fatalf("expected set-hook -ga session-closed commitNowCommand, not found in calls: %v", mock.Calls)
		}
		if unsetIdx >= appendIdx {
			t.Errorf("unset (call[%d]) must precede append (call[%d]) to avoid a transient duplicate window", unsetIdx, appendIdx)
		}
	})

	t.Run("it does not duplicate commitNowCommand when run against a post-fix state", func(t *testing.T) {
		// Post-fix state: commitNowCommand already registered on session-closed.
		raw := fmt.Sprintf("session-closed[0] => '%s'\n", expectedCommitNowCommand)
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// No unset and no append should touch session-closed.
		for _, c := range mock.Calls {
			if c[0] != "set-hook" {
				continue
			}
			if c[1] == "-ga" && len(c) >= 4 && c[2] == "session-closed" {
				t.Errorf("unexpected set-hook -ga on session-closed: %q (would duplicate)", c[3])
			}
			if c[1] == "-gu" && len(c) >= 3 && strings.HasPrefix(c[2], "session-closed[") {
				t.Errorf("unexpected set-hook -gu on session-closed: %q (post-fix state has no stale entry)", c[2])
			}
		}
	})

	t.Run("it preserves a user-customised hook on session-closed that does not exact-match the Portal literals", func(t *testing.T) {
		// User-authored hook references portal state notify but with a flag —
		// textually different from the historical notifyCommand literal. Must
		// be left untouched. Migration still appends commitNowCommand
		// alongside it (the user hook is not the Portal-owned entry).
		const userHook = `run-shell "portal state notify --debug"`
		raw := fmt.Sprintf("session-closed[0] => '%s'\n", userHook)
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// No unset call against session-closed: the user hook is preserved.
		for _, u := range unsetHookCalls(mock.Calls) {
			if strings.HasPrefix(u, "session-closed[") {
				t.Errorf("unexpected unset on user-customised session-closed entry: %q", u)
			}
		}

		// Exactly one append on session-closed and it must be commitNowCommand.
		var appendCmd string
		var appendCount int
		for _, c := range mock.Calls {
			if len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga" && c[2] == "session-closed" {
				appendCount++
				appendCmd = c[3]
			}
		}
		if appendCount != 1 {
			t.Fatalf("expected 1 append on session-closed alongside the user hook, got %d", appendCount)
		}
		if appendCmd != expectedCommitNowCommand {
			t.Errorf("append command = %q, want %q", appendCmd, expectedCommitNowCommand)
		}
	})

	t.Run("it skips the session-closed migration and logs WARN when ShowGlobalHooks fails, leaving the six other events to be processed", func(t *testing.T) {
		// The session-closed migration's ShowGlobalHooks call is the third
		// show-hooks call inside RegisterPortalHooks: migrateHydrationHooks
		// makes the first; the session-created RegisterHookIfAbsent makes
		// the second; migrateSessionClosedHook makes the third (session-
		// closed sits at index 1 in saveTriggerEvents). Failing call #3
		// isolates the session-closed migration failure from the hydration
		// migration and from the six structurally-independent
		// RegisterHookIfAbsent paths, which must still complete.
		sentinel := errors.New("tmux show-hooks failed")
		var showCallCount int
		runFunc := func(args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
				showCallCount++
				if showCallCount == 3 {
					return "", sentinel
				}
				return "", nil
			}
			if len(args) >= 4 && args[0] == "set-hook" && args[1] == "-ga" {
				return "", nil
			}
			if len(args) >= 3 && args[0] == "set-hook" && args[1] == "-gu" {
				return "", nil
			}
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
		mock := &MockCommander{RunFunc: runFunc}
		client := tmux.NewClient(mock)

		logger := &recordingMigrationLogger{}
		err := tmux.RegisterPortalHooks(client, logger.Logger().With("component", "bootstrap"))

		// The migration's ShowGlobalHooks failure surfaces as an aggregate
		// error consistent with how other step-2 register failures surface
		// (errors.Join wrapping the underlying sentinel).
		if err == nil {
			t.Fatal("expected aggregate error wrapping show-hooks sentinel, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("error %v does not wrap show-hooks sentinel %v", err, sentinel)
		}

		// A WARN must have been logged under the bootstrap component.
		if len(logger.warns) == 0 {
			t.Errorf("expected at least one WARN log line for the show-hooks failure, got none")
		}

		// session-closed must NOT have been appended (migration skipped).
		for _, c := range mock.Calls {
			if len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga" && c[2] == "session-closed" {
				t.Errorf("session-closed must not be appended when ShowGlobalHooks fails: %v", c)
			}
			if len(c) >= 3 && c[0] == "set-hook" && c[1] == "-gu" && strings.HasPrefix(c[2], "session-closed[") {
				t.Errorf("session-closed must not be unset when ShowGlobalHooks fails: %v", c)
			}
		}

		// The six non-session-closed save-trigger events and the two
		// hydration-trigger events must all still have been processed
		// (one set-hook -ga each).
		gotEvents := map[string]int{}
		for _, c := range mock.Calls {
			if len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga" {
				gotEvents[c[2]]++
			}
		}
		for _, ev := range nonSessionClosedSaveTriggerEvents {
			if gotEvents[ev] != 1 {
				t.Errorf("event %q set-hook -ga count = %d, want 1 (must still be processed)", ev, gotEvents[ev])
			}
		}
		for _, ev := range tmux.HydrationTriggerEvents {
			if gotEvents[ev] != 1 {
				t.Errorf("event %q set-hook -ga count = %d, want 1 (must still be processed)", ev, gotEvents[ev])
			}
		}
	})

	t.Run("it logs WARN and continues when UnsetGlobalHookAt fails for a specific index", func(t *testing.T) {
		// Two stale notifyCommand entries on session-closed at indices 0 and 1.
		// The first unset (highest index — 1) fails; the second (index 0)
		// must still be attempted. After the partial-failure eviction pass,
		// commitNowCommand must still be appended (the post-removal scan
		// finds no exact-matching entry: only the stale notifyCommand at 0
		// remains, which exact-matches notifyCommand, not commitNowCommand).
		raw := fmt.Sprintf("session-closed[0] => '%s'\nsession-closed[1] => '%s'\n",
			expectedNotifyCommand, expectedNotifyCommand)
		sentinel := errors.New("tmux unset failed at index 1")
		var unsetCallCount int
		runFunc := func(args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
				return raw, nil
			}
			if len(args) >= 3 && args[0] == "set-hook" && args[1] == "-gu" {
				unsetCallCount++
				if args[2] == "session-closed[1]" {
					return "", sentinel
				}
				return "", nil
			}
			if len(args) >= 4 && args[0] == "set-hook" && args[1] == "-ga" {
				return "", nil
			}
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
		mock := &MockCommander{RunFunc: runFunc}
		client := tmux.NewClient(mock)

		logger := &recordingMigrationLogger{}
		// The unset failure is best-effort and must not propagate; the
		// follow-up append is still attempted.
		if err := tmux.RegisterPortalHooks(client, logger.Logger().With("component", "bootstrap")); err != nil {
			t.Fatalf("unexpected error from RegisterPortalHooks: %v", err)
		}

		// Both indices must have been attempted, highest-first.
		var sessionClosedUnsets []string
		for _, u := range unsetHookCalls(mock.Calls) {
			if strings.HasPrefix(u, "session-closed[") {
				sessionClosedUnsets = append(sessionClosedUnsets, u)
			}
		}
		if len(sessionClosedUnsets) != 2 {
			t.Fatalf("expected 2 set-hook -gu calls on session-closed, got %d: %v", len(sessionClosedUnsets), sessionClosedUnsets)
		}
		if sessionClosedUnsets[0] != "session-closed[1]" || sessionClosedUnsets[1] != "session-closed[0]" {
			t.Errorf("unset order = %v, want [session-closed[1] session-closed[0]] (highest-first)", sessionClosedUnsets)
		}

		// WARN must have been logged for the failure.
		if len(logger.warns) == 0 {
			t.Errorf("expected at least one WARN line for the per-index unset failure, got none")
		}

		// Append of commitNowCommand still happens after the partial-eviction.
		var appended bool
		for _, c := range mock.Calls {
			if len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga" && c[2] == "session-closed" && c[3] == expectedCommitNowCommand {
				appended = true
				break
			}
		}
		if !appended {
			t.Errorf("expected commitNowCommand to be appended after partial-eviction, none recorded")
		}
	})

	t.Run("it is idempotent across repeated invocations", func(t *testing.T) {
		// Stateful tmux: first bootstrap on an empty hook table registers all
		// 9 entries (notifyCommand on six save-trigger events + commitNowCommand
		// on session-closed + signalHydrateCommand on two hydration events).
		// Second bootstrap reads the same 9 entries and must produce zero
		// appends and zero unsets.
		var registered [][2]string
		runFunc := func(args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
				var b strings.Builder
				perEventCount := map[string]int{}
				for _, entry := range registered {
					ev, cmd := entry[0], entry[1]
					idx := perEventCount[ev]
					perEventCount[ev] = idx + 1
					fmt.Fprintf(&b, "%s[%d] => '%s'\n", ev, idx, cmd)
				}
				return b.String(), nil
			}
			if len(args) >= 4 && args[0] == "set-hook" && args[1] == "-ga" {
				registered = append(registered, [2]string{args[2], args[3]})
				return "", nil
			}
			if len(args) >= 3 && args[0] == "set-hook" && args[1] == "-gu" {
				return "", nil
			}
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
		mock := &MockCommander{RunFunc: runFunc}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("first bootstrap: %v", err)
		}
		firstAppends := len(setHookCalls(mock.Calls))
		firstUnsets := len(unsetHookCalls(mock.Calls))
		want := len(expectedSaveTriggerEvents) + len(tmux.HydrationTriggerEvents)
		if firstAppends != want {
			t.Fatalf("first bootstrap append count = %d, want %d", firstAppends, want)
		}
		if firstUnsets != 0 {
			t.Fatalf("first bootstrap unset count = %d, want 0 (empty initial state)", firstUnsets)
		}

		mock.Calls = nil
		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("second bootstrap: %v", err)
		}
		if appends := setHookCalls(mock.Calls); len(appends) != 0 {
			t.Errorf("second bootstrap appended %d entries, want 0 (idempotent): %v", len(appends), appends)
		}
		if unsets := unsetHookCalls(mock.Calls); len(unsets) != 0 {
			t.Errorf("second bootstrap issued %d unsets, want 0 (idempotent): %v", len(unsets), unsets)
		}
	})

	t.Run("it processes removal indices highest-first so earlier removals do not shift later indices", func(t *testing.T) {
		// Three stale notifyCommand entries on session-closed at indices
		// 0, 1, 2. They must be unset in descending order (2, 1, 0) so each
		// removal targets the entry it identified during the pre-removal
		// scan — an ascending order would shift later indices and either
		// remove the wrong entry or fail.
		raw := fmt.Sprintf("session-closed[0] => '%s'\nsession-closed[1] => '%s'\nsession-closed[2] => '%s'\n",
			expectedNotifyCommand, expectedNotifyCommand, expectedNotifyCommand)
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, raw, nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var sessionClosedUnsets []string
		for _, u := range unsetHookCalls(mock.Calls) {
			if strings.HasPrefix(u, "session-closed[") {
				sessionClosedUnsets = append(sessionClosedUnsets, u)
			}
		}
		want := []string{"session-closed[2]", "session-closed[1]", "session-closed[0]"}
		if len(sessionClosedUnsets) != len(want) {
			t.Fatalf("unset call count = %d, want %d: %v", len(sessionClosedUnsets), len(want), sessionClosedUnsets)
		}
		for i, got := range sessionClosedUnsets {
			if got != want[i] {
				t.Errorf("unset[%d] = %q, want %q (descending-index order required)", i, got, want[i])
			}
		}
	})
}

// TestRegisterPortalHooks_EvictionLineEmittedUnderBootstrapComponent is the
// Task 1-9 acceptance for the hooks-register migration's eviction logging: when
// migrateHydrationHooks evicts at least one stale un-separated signal-hydrate
// entry, it emits one INFO summary line, and that line is bound to the
// component the caller injected (bootstrap, per Phase-1). Hermetic — no real
// tmux; the MockCommander returns a stale entry on each hydration event.
func TestRegisterPortalHooks_EvictionLineEmittedUnderBootstrapComponent(t *testing.T) {
	// Seed one stale (un-separated) signal-hydrate entry on each hydration
	// event so migrateHydrationHooks evicts them and emits its INFO summary.
	var b strings.Builder
	for _, ev := range tmux.HydrationTriggerEvents {
		fmt.Fprintf(&b, "%s[0] => '%s'\n", ev, staleSignalHydrateCommand)
	}
	mock := &MockCommander{RunFunc: dispatchPortalHooks(t, b.String(), nil)}
	client := tmux.NewClient(mock)

	logger := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, logger.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("RegisterPortalHooks: %v", err)
	}

	// Exactly one eviction INFO summary, rendered as "[<component>] <msg>" by
	// recordingMigrationLogger — must carry the bootstrap component and the
	// canonical eviction phrase.
	var evictionLines []string
	for _, line := range logger.infos {
		if strings.Contains(line, "evicted stale signal-hydrate") {
			evictionLines = append(evictionLines, line)
		}
	}
	if len(evictionLines) != 1 {
		t.Fatalf("eviction INFO line count = %d, want 1; infos=%v", len(evictionLines), logger.infos)
	}
	if !strings.HasPrefix(evictionLines[0], "[bootstrap] ") {
		t.Errorf("eviction line %q not bound to the bootstrap component", evictionLines[0])
	}
}
