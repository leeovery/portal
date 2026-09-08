package tmux_test

import (
	"fmt"
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
