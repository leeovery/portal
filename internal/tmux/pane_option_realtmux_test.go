package tmux_test

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// paneOptionFixtureSocketPrefix names the temp dir of this suite's socket, so a
// stray server left behind by it is recognisable.
const paneOptionFixtureSocketPrefix = "ptl-paneopt-"

func readResumePending(t *testing.T, ts *tmuxtest.Socket, target tmux.Target) string {
	t.Helper()
	out := ts.Run(t, "display-message", "-p", "-t", string(target), "#{"+state.ResumePendingOption+"}")
	return strings.TrimRight(out, "\n")
}

func seedPaneOptionServer(t *testing.T, sessionName string) (*tmuxtest.Socket, *tmux.Client) {
	t.Helper()
	ts, client, _ := seedRealTmuxServer(t, realTmuxFixture{
		socketPrefix: paneOptionFixtureSocketPrefix,
		sessions:     []string{sessionName},
	})
	return ts, client
}

func TestUnsetPaneOption_RealTmux(t *testing.T) {
	t.Run("it round-trips the marker on a real tmux pane", func(t *testing.T) {
		const sessionName = "paneopt-roundtrip"
		ts, client := seedPaneOptionServer(t, sessionName)
		target := tmux.PaneIDTarget(sessionPaneIDs(t, ts, sessionName)[0])

		if err := client.SetPaneOption(target, state.ResumePendingOption, "1"); err != nil {
			t.Fatalf("SetPaneOption: %v", err)
		}
		if got := readResumePending(t, ts, target); got != "1" {
			t.Errorf("marker after set = %q, want %q", got, "1")
		}

		if err := client.UnsetPaneOption(target, state.ResumePendingOption); err != nil {
			t.Fatalf("UnsetPaneOption: %v", err)
		}
		if got := readResumePending(t, ts, target); got != "" {
			t.Errorf("marker after unset = %q, want an empty read", got)
		}
	})

	t.Run("it treats an unset of an already-absent marker as a no-op", func(t *testing.T) {
		const sessionName = "paneopt-absent"
		ts, client := seedPaneOptionServer(t, sessionName)
		target := tmux.PaneIDTarget(sessionPaneIDs(t, ts, sessionName)[0])

		if err := client.UnsetPaneOption(target, state.ResumePendingOption); err != nil {
			t.Fatalf("first unset of a never-set marker: %v", err)
		}
		if err := client.UnsetPaneOption(target, state.ResumePendingOption); err != nil {
			t.Fatalf("second unset of an absent marker: %v", err)
		}
		if got := readResumePending(t, ts, target); got != "" {
			t.Errorf("marker after two unsets = %q, want an empty read", got)
		}
	})

	t.Run("it fails an unset against a target no live pane answers to", func(t *testing.T) {
		const sessionName = "paneopt-gone"
		_, client := seedPaneOptionServer(t, sessionName)

		err := client.UnsetPaneOption(tmux.PaneIDTarget("%99999"), state.ResumePendingOption)
		if err == nil {
			t.Fatal("expected an error for a target no live pane answers to, got nil")
		}
	})
}

func TestReadPaneOption_RealTmux(t *testing.T) {
	t.Run("it reads back a set and an unset pane option", func(t *testing.T) {
		const sessionName = "paneopt-read"
		ts, client := seedPaneOptionServer(t, sessionName)
		target := tmux.PaneIDTarget(sessionPaneIDs(t, ts, sessionName)[0])

		got, err := client.ReadPaneOption(target, state.ResumePendingOption)
		if err != nil {
			t.Fatalf("ReadPaneOption on a never-set marker: %v", err)
		}
		if got != "" {
			t.Errorf("marker before set = %q, want an empty read", got)
		}

		if err := client.SetPaneOption(target, state.ResumePendingOption, "1"); err != nil {
			t.Fatalf("SetPaneOption: %v", err)
		}
		got, err = client.ReadPaneOption(target, state.ResumePendingOption)
		if err != nil {
			t.Fatalf("ReadPaneOption after set: %v", err)
		}
		if got != "1" {
			t.Errorf("marker after set = %q, want %q", got, "1")
		}

		if err := client.UnsetPaneOption(target, state.ResumePendingOption); err != nil {
			t.Fatalf("UnsetPaneOption: %v", err)
		}
		got, err = client.ReadPaneOption(target, state.ResumePendingOption)
		if err != nil {
			t.Fatalf("ReadPaneOption after unset: %v", err)
		}
		if got != "" {
			t.Errorf("marker after unset = %q, want an empty read", got)
		}
	})

	t.Run("it fails a read against a target no live pane answers to", func(t *testing.T) {
		const sessionName = "paneopt-read-gone"
		_, client := seedPaneOptionServer(t, sessionName)

		got, err := client.ReadPaneOption(tmux.PaneIDTarget("%99999"), state.ResumePendingOption)
		if err == nil {
			t.Fatalf("ReadPaneOption = (%q, nil); a gone pane must not read as an absent marker", got)
		}
	})
}
