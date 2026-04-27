package tmux_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

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
