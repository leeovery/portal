package tmux_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
)

func TestResolveHookKey_ProbeOrdering(t *testing.T) {
	t.Run("it probes before reading the token", func(t *testing.T) {
		mock := commandertest.FromFunc(func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "show-options" {
				return "", fmt.Errorf("no such pane: %%999")
			}
			return "", nil
		})

		got, err := tmux.NewClient(mock).ResolveHookKey("%999")
		if err == nil {
			t.Fatalf("ResolveHookKey = (%q, nil), want the probe's error", got)
		}
		if got != "" {
			t.Errorf("hook key = %q, want \"\"", got)
		}

		if len(mock.Calls()) != 1 {
			t.Fatalf("tmux calls = %v, want the probe alone", mock.Calls())
		}
		wantProbe := []string{"show-options", "-p", "-t", "%999"}
		if !slices.Equal(mock.Calls()[0], wantProbe) {
			t.Errorf("probe argv = %v, want %v (the option name must be omitted)", mock.Calls()[0], wantProbe)
		}
	})

	t.Run("it reads the token once the probe passes", func(t *testing.T) {
		mock := commandertest.FromFunc(func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "show-options" {
				return "", nil
			}
			return "tok777", nil
		})

		got, err := tmux.NewClient(mock).ResolveHookKey("%3")
		if err != nil {
			t.Fatalf("ResolveHookKey: %v", err)
		}
		if got != "tok777" {
			t.Errorf("hook key = %q, want %q", got, "tok777")
		}

		wantCalls := [][]string{
			{"show-options", "-p", "-t", "%3"},
			{"display-message", "-p", "-t", "%3", tmux.HookKeyFormat},
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

	t.Run("it reports a failed token read rather than a resolved key", func(t *testing.T) {
		const stderr = "can't find pane: %3"
		mock := commandertest.FromFunc(func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "show-options" {
				return "", nil
			}
			return "", &tmux.CommandError{Stderr: stderr}
		})

		got, err := tmux.NewClient(mock).ResolveHookKey("%3")
		if err == nil {
			t.Fatalf("ResolveHookKey = (%q, nil), want the failed read's error", got)
		}
		if got != "" {
			t.Errorf("hook key on a failed read = %q, want \"\"", got)
		}
		if _, ok := errors.AsType[*tmux.CommandError](err); !ok {
			t.Fatalf("errors.AsType[*tmux.CommandError](err) = false; want a recoverable *tmux.CommandError; err = %v", err)
		}
		if !strings.Contains(err.Error(), stderr) {
			t.Errorf("error = %q, want it to carry tmux's own words %q", err.Error(), stderr)
		}
	})
}
