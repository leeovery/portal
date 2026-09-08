package tmux_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
)

func TestShowGlobalHooksForEvent(t *testing.T) {
	t.Run("calls show-hooks -g <event> and returns the unparsed output", func(t *testing.T) {
		// Two entries the method must not split, filter or reorder, and a
		// trailing newline the read through Run trims — so a move to RunRaw is
		// visible here rather than silent.
		raw := "pane-focus-out[0] run-shell 'command -v portal'\npane-focus-out[1] run-shell 'echo hi'\n"
		mock := commandertest.New(t, commandertest.Returns(raw, "show-hooks"))
		client := tmux.NewClient(mock)

		got, err := client.ShowGlobalHooksForEvent("pane-focus-out")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := strings.TrimSpace(raw); got != want {
			t.Errorf("ShowGlobalHooksForEvent() = %q, want %q", got, want)
		}

		if len(mock.Calls()) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls()))
		}
		wantArgs := []string{"show-hooks", "-g", "pane-focus-out"}
		if len(mock.Calls()[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls()[0]), mock.Calls()[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls()[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("returns empty string without error when output is empty", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("", "show-hooks"))
		client := tmux.NewClient(mock)

		got, err := client.ShowGlobalHooksForEvent("window-renamed")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("ShowGlobalHooksForEvent() = %q, want empty string", got)
		}
	})

	t.Run("propagates commander error wrapped via %w", func(t *testing.T) {
		sentinel := errors.New("tmux exec failed")
		mock := commandertest.New(t, commandertest.Fails(sentinel, "show-hooks"))
		client := tmux.NewClient(mock)

		got, err := client.ShowGlobalHooksForEvent("window-resized")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if got != "" {
			t.Errorf("ShowGlobalHooksForEvent() output = %q, want empty string on error", got)
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
		}
		if !strings.Contains(err.Error(), "failed to show global hooks: ") {
			t.Errorf("error %q does not contain expected prefix", err.Error())
		}
	})
}

func TestAppendGlobalHook(t *testing.T) {
	t.Run("calls set-hook -ga with event and command as separate argv elements", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("", "set-hook", "-ga"))
		client := tmux.NewClient(mock)

		event := "session-created"
		command := `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`

		err := client.AppendGlobalHook(event, command)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls()) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls()))
		}
		wantArgs := []string{"set-hook", "-ga", event, command}
		if len(mock.Calls()[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls()[0]), mock.Calls()[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls()[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("preserves single quotes inside the hook command argument", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("", "set-hook", "-ga"))
		client := tmux.NewClient(mock)

		command := `run-shell 'command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}'`

		err := client.AppendGlobalHook("client-attached", command)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls()) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls()))
		}
		if len(mock.Calls()[0]) != 4 {
			t.Fatalf("got %d args %v, want 4 args", len(mock.Calls()[0]), mock.Calls()[0])
		}
		if mock.Calls()[0][3] != command {
			t.Errorf("command argv element = %q, want %q (single quotes preserved)", mock.Calls()[0][3], command)
		}
	})

	t.Run("wraps commander error with the event name", func(t *testing.T) {
		sentinel := errors.New("tmux failed")
		mock := commandertest.New(t, commandertest.Fails(sentinel, "set-hook", "-ga"))
		client := tmux.NewClient(mock)

		err := client.AppendGlobalHook("session-renamed", "run-shell 'noop'")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
		}
		if !strings.Contains(err.Error(), "failed to append hook") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "session-renamed") {
			t.Errorf("error %q does not contain event name", err.Error())
		}
	})
}

func TestUnsetGlobalHookAt(t *testing.T) {
	t.Run("formats target as event[index] for set-hook -gu", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("", "set-hook", "-gu"))
		client := tmux.NewClient(mock)

		err := client.UnsetGlobalHookAt("session-created", 2)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls()) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls()))
		}
		wantArgs := []string{"set-hook", "-gu", "session-created[2]"}
		if len(mock.Calls()[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls()[0]), mock.Calls()[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls()[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("wraps commander error with event and index", func(t *testing.T) {
		sentinel := errors.New("tmux failed")
		mock := commandertest.New(t, commandertest.Fails(sentinel, "set-hook", "-gu"))
		client := tmux.NewClient(mock)

		err := client.UnsetGlobalHookAt("client-attached", 5)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
		}
		if !strings.Contains(err.Error(), "failed to unset hook") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "client-attached[5]") {
			t.Errorf("error %q does not contain formatted target", err.Error())
		}
	})
}
