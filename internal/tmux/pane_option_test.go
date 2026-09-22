package tmux_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func TestSetPaneOption(t *testing.T) {
	t.Run("it runs set-option -p against the pane target", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("", "set-option"))
		client := tmux.NewClient(mock)

		if err := client.SetPaneOption("%3", state.PortalPaneIDOption, "abc123"); err != nil {
			t.Fatalf("SetPaneOption: %v", err)
		}

		if len(mock.Calls()) != 1 {
			t.Fatalf("call count = %d, want 1", len(mock.Calls()))
		}
		want := "set-option -p -t %3 " + state.PortalPaneIDOption + " abc123"
		if got := strings.Join(mock.Calls()[0], " "); got != want {
			t.Errorf("called with %q, want %q", got, want)
		}
	})

	t.Run("it scopes the write to one pane, never the server or a session", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("", "set-option"))
		client := tmux.NewClient(mock)

		_ = client.SetPaneOption("%3", state.PortalPaneIDOption, "abc123")

		if len(mock.Calls()) != 1 {
			t.Fatalf("call count = %d, want 1", len(mock.Calls()))
		}
		for _, arg := range mock.Calls()[0] {
			if arg == "-g" || arg == "-s" {
				t.Errorf("SetPaneOption must not widen the scope, got args %v", mock.Calls()[0])
			}
		}
	})

	t.Run("it wraps a tmux failure with the pane and the option name", func(t *testing.T) {
		client := tmux.NewClient(commandertest.New(t, commandertest.Fails(fmt.Errorf("no such pane: %%999"), "set-option")))

		err := client.SetPaneOption("%999", state.PortalPaneIDOption, "abc123")
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		for _, want := range []string{"%999", state.PortalPaneIDOption, "no such pane"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err.Error(), want)
			}
		}
	})
}

func TestUnsetPaneOption(t *testing.T) {
	t.Run("it composes set-option -pu with the pane target", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("", "set-option"))
		client := tmux.NewClient(mock)

		if err := client.UnsetPaneOption("%3", state.ResumePendingOption); err != nil {
			t.Fatalf("UnsetPaneOption: %v", err)
		}

		if len(mock.Calls()) != 1 {
			t.Fatalf("call count = %d, want 1", len(mock.Calls()))
		}
		want := "set-option -pu -t %3 " + state.ResumePendingOption
		if got := strings.Join(mock.Calls()[0], " "); got != want {
			t.Errorf("called with %q, want %q", got, want)
		}
	})

	t.Run("it wraps a tmux failure with the pane and the option name", func(t *testing.T) {
		client := tmux.NewClient(commandertest.New(t, commandertest.Fails(fmt.Errorf("no such pane: %%999"), "set-option")))

		err := client.UnsetPaneOption("%999", state.ResumePendingOption)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		for _, want := range []string{"%999", state.ResumePendingOption, "no such pane"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err.Error(), want)
			}
		}
	})
}

func TestReadPaneOption(t *testing.T) {
	t.Run("it composes the existence probe and then the format read", func(t *testing.T) {
		mock := commandertest.FromFunc(func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "show-options" {
				return "", nil
			}
			return "1", nil
		})

		got, err := tmux.NewClient(mock).ReadPaneOption("%3", state.ResumePendingOption)
		if err != nil {
			t.Fatalf("ReadPaneOption: %v", err)
		}
		if got != "1" {
			t.Errorf("value = %q, want %q", got, "1")
		}

		wantCalls := [][]string{
			{"show-options", "-p", "-t", "%3"},
			{"display-message", "-p", "-t", "%3", "-F", "#{" + state.ResumePendingOption + "}"},
		}
		if len(mock.Calls()) != len(wantCalls) {
			t.Fatalf("tmux calls = %v, want %v", mock.Calls(), wantCalls)
		}
		for i, want := range wantCalls {
			if !slices.Equal(mock.Calls()[i], want) {
				t.Errorf("call %d = %v, want %v", i, mock.Calls()[i], want)
			}
		}
	})

	t.Run("it returns the probe's failure before the value is read", func(t *testing.T) {
		mock := commandertest.FromFunc(func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "show-options" {
				return "", fmt.Errorf("no such pane: %%999")
			}
			return "", nil
		})

		got, err := tmux.NewClient(mock).ReadPaneOption("%999", state.ResumePendingOption)
		if err == nil {
			t.Fatalf("ReadPaneOption = (%q, nil), want the probe's error", got)
		}
		if got != "" {
			t.Errorf("value = %q, want \"\"", got)
		}
		if len(mock.Calls()) != 1 {
			t.Fatalf("tmux calls = %v, want the probe alone", mock.Calls())
		}
	})

	t.Run("it wraps a failed value read with the pane and the option name", func(t *testing.T) {
		mock := commandertest.FromFunc(func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "show-options" {
				return "", nil
			}
			return "", fmt.Errorf("server exited unexpectedly")
		})

		_, err := tmux.NewClient(mock).ReadPaneOption("%3", state.ResumePendingOption)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		for _, want := range []string{"%3", state.ResumePendingOption, "server exited unexpectedly"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err.Error(), want)
			}
		}
	})
}
