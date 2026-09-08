package tmux_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
)

func TestSaverPanePID(t *testing.T) {
	t.Run("it returns the parsed pid on a single-line success", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("12345\n", "list-panes"))
		client := tmux.NewClient(mock)

		pid, err := tmux.SaverPanePID(client, "_portal-saver")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pid != 12345 {
			t.Errorf("pid = %d, want 12345", pid)
		}
		if len(mock.Calls()) != 1 {
			t.Fatalf("Calls = %d, want 1", len(mock.Calls()))
		}
		got := mock.Calls()[0]
		want := []string{"list-panes", "-t", "=_portal-saver:", "-F", "#{pane_pid}"}
		if !equalStringSlice(got, want) {
			t.Errorf("Run args = %v, want %v", got, want)
		}
	})

	t.Run("it wraps ErrNoSuchSession when stderr reports the session cannot be found", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Fails(&tmux.CommandError{
			Stderr: "can't find session: _portal-saver",
			Err:    errors.New("exit status 1"),
		}, "list-panes"))
		client := tmux.NewClient(mock)

		_, err := tmux.SaverPanePID(client, "_portal-saver")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, tmux.ErrNoSuchSession) {
			t.Errorf("errors.Is(err, ErrNoSuchSession) = false, want true; err = %v", err)
		}
	})

	t.Run("it returns ErrEmptyPaneList when stdout is empty", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("", "list-panes"))
		client := tmux.NewClient(mock)

		_, err := tmux.SaverPanePID(client, "_portal-saver")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, tmux.ErrEmptyPaneList) {
			t.Errorf("errors.Is(err, ErrEmptyPaneList) = false, want true; err = %v", err)
		}
	})

	t.Run("it returns ErrEmptyPaneList when stdout is whitespace-only", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("   \n\n  ", "list-panes"))
		client := tmux.NewClient(mock)

		_, err := tmux.SaverPanePID(client, "_portal-saver")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, tmux.ErrEmptyPaneList) {
			t.Errorf("errors.Is(err, ErrEmptyPaneList) = false, want true; err = %v", err)
		}
	})

	t.Run("it returns ErrPanePIDParse when stdout is non-numeric", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("not-a-pid\n", "list-panes"))
		client := tmux.NewClient(mock)

		_, err := tmux.SaverPanePID(client, "_portal-saver")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, tmux.ErrPanePIDParse) {
			t.Errorf("errors.Is(err, ErrPanePIDParse) = false, want true; err = %v", err)
		}
	})

	t.Run("it takes the first non-empty line of a multi-line stdout", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("\n  \n777\n888\n", "list-panes"))
		client := tmux.NewClient(mock)

		pid, err := tmux.SaverPanePID(client, "_portal-saver")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pid != 777 {
			t.Errorf("pid = %d, want 777 (first non-empty line)", pid)
		}
	})

	t.Run("it returns a wrapped generic exec error without matching sentinels", func(t *testing.T) {
		genericErr := fmt.Errorf("exec lookup failed")
		mock := commandertest.New(t, commandertest.Fails(genericErr, "list-panes"))
		client := tmux.NewClient(mock)

		_, err := tmux.SaverPanePID(client, "_portal-saver")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, tmux.ErrNoSuchSession) {
			t.Errorf("errors.Is(err, ErrNoSuchSession) = true, want false; err = %v", err)
		}
		if errors.Is(err, tmux.ErrEmptyPaneList) {
			t.Errorf("errors.Is(err, ErrEmptyPaneList) = true, want false; err = %v", err)
		}
		if errors.Is(err, tmux.ErrPanePIDParse) {
			t.Errorf("errors.Is(err, ErrPanePIDParse) = true, want false; err = %v", err)
		}
		if !errors.Is(err, genericErr) {
			t.Errorf("errors.Is(err, genericErr) = false, want true; err = %v", err)
		}
	})
}

func TestSaverPanePIDOrAbsent(t *testing.T) {
	t.Run("it collapses an empty pane list to present=false", func(t *testing.T) {
		client := tmux.NewClient(commandertest.New(t, commandertest.Returns("", "list-panes")))

		pid, present, err := tmux.SaverPanePIDOrAbsent(client, "_portal-saver")
		if pid != 0 || present || err != nil {
			t.Errorf("SaverPanePIDOrAbsent = %d, %t, %v; want 0, false, nil", pid, present, err)
		}
	})

	t.Run("it does not read a missing window inside a live session as session absence", func(t *testing.T) {
		cmdErr := &tmux.CommandError{
			Stderr: "can't find window: _portal-saver",
			Err:    errors.New("exit status 1"),
		}
		client := tmux.NewClient(commandertest.New(t, commandertest.Fails(cmdErr, "list-panes")))

		pid, present, err := tmux.SaverPanePIDOrAbsent(client, "_portal-saver")
		if errors.Is(err, tmux.ErrNoSuchSession) {
			t.Errorf("errors.Is(err, ErrNoSuchSession) = true, want false; err = %v", err)
		}
		if pid != 0 || present || err == nil {
			t.Errorf("SaverPanePIDOrAbsent = %d, %t, %v; want 0, false, a non-nil error", pid, present, err)
		}
	})

	t.Run("it passes a non-absence error through to the caller", func(t *testing.T) {
		genericErr := fmt.Errorf("exec lookup failed")
		client := tmux.NewClient(commandertest.New(t, commandertest.Fails(genericErr, "list-panes")))

		pid, present, err := tmux.SaverPanePIDOrAbsent(client, "_portal-saver")
		if pid != 0 || present {
			t.Errorf("SaverPanePIDOrAbsent = %d, %t; want 0, false", pid, present)
		}
		if !errors.Is(err, genericErr) {
			t.Errorf("errors.Is(err, genericErr) = false, want true; err = %v", err)
		}
	})
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
