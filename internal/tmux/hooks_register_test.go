package tmux_test

import (
	"errors"
	"fmt"
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

// expectedHydrationTriggerEvents is the canonical hydration-trigger event
// list, in registration order. Mirrors hydrationTriggerEvents in
// hooks_register.go.
var expectedHydrationTriggerEvents = []string{
	"client-attached",
	"client-session-changed",
}

// expectedNotifyCommand is the exact full command Portal registers on every
// save-trigger event. Mirrors notifyCommand in hooks_register.go.
const expectedNotifyCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`

// expectedSignalHydrateCommand is the exact full command Portal registers on
// every hydration-trigger event. Mirrors signalHydrateCommand in
// hooks_register.go. The literal #{session_name} is preserved verbatim — tmux
// expands it at hook-fire time.
const expectedSignalHydrateCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"`

// dispatchPortalHooks builds a RunFunc that returns showOutput for every
// "show-hooks -g" call and records "set-hook -ga" calls. setHookErrFor, when
// non-nil, returns the configured error for matching events; nil otherwise.
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
		t.Fatalf("unexpected command: %v", args)
		return "", nil
	}
}

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
// a Portal entry for every save-trigger and hydration-trigger event.
func allPortalHooksRegisteredOutput() string {
	var b strings.Builder
	for _, e := range expectedSaveTriggerEvents {
		fmt.Fprintf(&b, "%s[0] => %q\n", e, expectedNotifyCommand)
	}
	for _, e := range expectedHydrationTriggerEvents {
		fmt.Fprintf(&b, "%s[0] => %q\n", e, expectedSignalHydrateCommand)
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
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
		client := tmux.NewClient(mock)

		err := tmux.RegisterPortalHooks(client)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := setHookCalls(mock.Calls)
		if len(got) != 9 {
			t.Fatalf("set-hook -ga call count = %d, want 9: %v", len(got), got)
		}
	})

	t.Run("it registers hooks in the documented order", func(t *testing.T) {
		// Save-trigger events first, then hydration-trigger events.
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := setHookCalls(mock.Calls)
		want := append([]string{}, expectedSaveTriggerEvents...)
		want = append(want, expectedHydrationTriggerEvents...)

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

		err := tmux.RegisterPortalHooks(client)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := setHookCalls(mock.Calls); len(got) != 0 {
			t.Errorf("expected 0 set-hook -ga calls (all present), got %d: %v", len(got), got)
		}
	})

	t.Run("it tops up only the missing events on a partially-registered server", func(t *testing.T) {
		// Five save-trigger events present, two missing (window-unlinked and pane-focus-out).
		// Both hydration events present.
		var b strings.Builder
		present := map[string]bool{
			"session-created":       true,
			"session-closed":        true,
			"session-renamed":       true,
			"window-linked":         true,
			"window-layout-changed": true,
		}
		for _, e := range expectedSaveTriggerEvents {
			if present[e] {
				fmt.Fprintf(&b, "%s[0] => %q\n", e, expectedNotifyCommand)
			}
		}
		for _, e := range expectedHydrationTriggerEvents {
			fmt.Fprintf(&b, "%s[0] => %q\n", e, expectedSignalHydrateCommand)
		}

		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, b.String(), nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client); err != nil {
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
		// Fresh server: all 9 events would be appended. We make set-hook fail
		// for two specific events. RegisterPortalHooks must still attempt every
		// event (9 set-hook calls in total).
		failures := map[string]error{
			"session-renamed":        errors.New("transient tmux failure"),
			"client-session-changed": errors.New("another transient failure"),
		}
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", failures)}
		client := tmux.NewClient(mock)

		err := tmux.RegisterPortalHooks(client)

		if err == nil {
			t.Fatal("expected aggregate error, got nil")
		}
		got := setHookCalls(mock.Calls)
		if len(got) != 9 {
			t.Errorf("set-hook -ga call count = %d, want 9 (every event attempted): %v", len(got), got)
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

		err := tmux.RegisterPortalHooks(client)

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
		// registers all 9. Second bootstrap sees those 9 in show-hooks output
		// and registers nothing.
		var registered []string
		runFunc := func(args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
				var b strings.Builder
				for _, ev := range registered {
					// Use the same command body Portal would have appended.
					cmd := expectedNotifyCommand
					for _, h := range expectedHydrationTriggerEvents {
						if ev == h {
							cmd = expectedSignalHydrateCommand
							break
						}
					}
					fmt.Fprintf(&b, "%s[0] => %q\n", ev, cmd)
				}
				return b.String(), nil
			}
			if len(args) >= 4 && args[0] == "set-hook" && args[1] == "-ga" {
				registered = append(registered, args[2])
				return "", nil
			}
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
		mock := &MockCommander{RunFunc: runFunc}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client); err != nil {
			t.Fatalf("first bootstrap: unexpected error: %v", err)
		}
		firstBootstrapAppends := len(setHookCalls(mock.Calls))
		if firstBootstrapAppends != 9 {
			t.Fatalf("first bootstrap set-hook count = %d, want 9", firstBootstrapAppends)
		}

		// Reset call log; run again.
		mock.Calls = nil
		if err := tmux.RegisterPortalHooks(client); err != nil {
			t.Fatalf("second bootstrap: unexpected error: %v", err)
		}
		if got := setHookCalls(mock.Calls); len(got) != 0 {
			t.Errorf("second bootstrap appended %d entries, want 0 (idempotent): %v", len(got), got)
		}
	})

	t.Run("it wraps each save-trigger event's command with command -v portal guard", func(t *testing.T) {
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := setHookCalls(mock.Calls)
		// First seven calls correspond to save-trigger events.
		for i, ev := range expectedSaveTriggerEvents {
			if got[i][0] != ev {
				t.Errorf("call[%d] event = %q, want %q", i, got[i][0], ev)
			}
			if got[i][1] != expectedNotifyCommand {
				t.Errorf("call[%d] command = %q, want %q", i, got[i][1], expectedNotifyCommand)
			}
		}
	})

	t.Run("it wraps each hydration-trigger event's command with command -v portal guard and #{session_name}", func(t *testing.T) {
		mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := setHookCalls(mock.Calls)
		// Last two calls correspond to hydration-trigger events.
		base := len(expectedSaveTriggerEvents)
		for i, ev := range expectedHydrationTriggerEvents {
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
}
