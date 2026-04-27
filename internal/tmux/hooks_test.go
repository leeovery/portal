package tmux_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

func TestShowGlobalHooks(t *testing.T) {
	t.Run("calls show-hooks -g verbatim and returns raw output", func(t *testing.T) {
		// Output intentionally contains leading/trailing whitespace and newlines
		// to confirm the wrapper does not trim.
		raw := "\nsession-created[0] run-shell 'command -v portal'\nsession-closed[0] run-shell 'command -v portal'\n\n"
		mock := &MockCommander{Output: raw}
		client := tmux.NewClient(mock)

		got, err := client.ShowGlobalHooks()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != raw {
			t.Errorf("ShowGlobalHooks() = %q, want %q (verbatim)", got, raw)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := []string{"show-hooks", "-g"}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("returns empty string without error when output is empty", func(t *testing.T) {
		mock := &MockCommander{Output: ""}
		client := tmux.NewClient(mock)

		got, err := client.ShowGlobalHooks()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("ShowGlobalHooks() = %q, want empty string", got)
		}
	})

	t.Run("propagates commander error wrapped via %w", func(t *testing.T) {
		sentinel := errors.New("tmux exec failed")
		mock := &MockCommander{Err: sentinel}
		client := tmux.NewClient(mock)

		got, err := client.ShowGlobalHooks()

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if got != "" {
			t.Errorf("ShowGlobalHooks() output = %q, want empty string on error", got)
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
		}
	})
}

func TestAppendGlobalHook(t *testing.T) {
	t.Run("calls set-hook -ga with event and command as separate argv elements", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		event := "session-created"
		command := `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`

		err := client.AppendGlobalHook(event, command)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := []string{"set-hook", "-ga", event, command}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("preserves single quotes inside the hook command argument", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		// Single-quoted command — must arrive verbatim as one argv element.
		command := `run-shell 'command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}'`

		err := client.AppendGlobalHook("client-attached", command)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		// The command must be a single argv element (index 3) with single quotes intact.
		if len(mock.Calls[0]) != 4 {
			t.Fatalf("got %d args %v, want 4 args", len(mock.Calls[0]), mock.Calls[0])
		}
		if mock.Calls[0][3] != command {
			t.Errorf("command argv element = %q, want %q (single quotes preserved)", mock.Calls[0][3], command)
		}
	})

	t.Run("wraps commander error with the event name", func(t *testing.T) {
		sentinel := errors.New("tmux failed")
		mock := &MockCommander{Err: sentinel}
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
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.UnsetGlobalHookAt("session-created", 2)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := []string{"set-hook", "-gu", "session-created[2]"}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("wraps commander error with event and index", func(t *testing.T) {
		sentinel := errors.New("tmux failed")
		mock := &MockCommander{Err: sentinel}
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
