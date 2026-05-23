package tmux_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// TestSaverPanePID covers the six observable shapes documented on
// tmux.SaverPanePID:
//
//  1. success — single pane_pid line parses into an int.
//  2. ErrNoSuchSession — stderr contains "no such session" → wrap.
//  3. ErrEmptyPaneList — stdout empty (no panes) → wrap.
//  4. ErrPanePIDParse — stdout non-numeric → wrap.
//  5. multi-line stdout — first non-empty line is taken.
//  6. generic exec error — non-CommandError chains return wrapped, no sentinel.
//
// The helper is the dependency Component D's saverMembershipProbe consumes
// to detect a pid-mismatch (orphan daemon) condition; the error classification
// here is what lets the probe collapse every failure mode to "absent" without
// substring-matching tmux stderr.
func TestSaverPanePID(t *testing.T) {
	t.Run("it returns the parsed pid on a single-line success", func(t *testing.T) {
		mock := &MockCommander{Output: "12345\n"}
		client := tmux.NewClient(mock)

		pid, err := tmux.SaverPanePID(client, "_portal-saver")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pid != 12345 {
			t.Errorf("pid = %d, want 12345", pid)
		}
		if len(mock.Calls) != 1 {
			t.Fatalf("Calls = %d, want 1", len(mock.Calls))
		}
		got := mock.Calls[0]
		want := []string{"list-panes", "-t", "=_portal-saver", "-F", "#{pane_pid}"}
		if !equalStringSlice(got, want) {
			t.Errorf("Run args = %v, want %v", got, want)
		}
	})

	t.Run("it wraps ErrNoSuchSession when stderr contains 'no such session'", func(t *testing.T) {
		mock := &MockCommander{Err: &tmux.CommandError{
			Stderr: "no such session: _portal-saver",
			Err:    errors.New("exit status 1"),
		}}
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
		mock := &MockCommander{Output: ""}
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
		// Whitespace-only output is observably equivalent to "no panes" — the
		// helper must classify it the same as a bare empty string rather than
		// drift into ErrPanePIDParse on the trimmed empty token.
		mock := &MockCommander{Output: "   \n\n  "}
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
		mock := &MockCommander{Output: "not-a-pid\n"}
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
		mock := &MockCommander{Output: "\n  \n777\n888\n"}
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
		mock := &MockCommander{Err: genericErr}
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

// equalStringSlice is a small local helper so the test does not pull in a
// comparison dependency just to assert tmux argv vectors.
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
