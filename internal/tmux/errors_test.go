package tmux_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// TestShowEnvironment_ErrNoSuchSession exercises the sentinel-wrapping
// behaviour that ShowEnvironment must apply when the underlying tmux command
// reports "no such session" on stderr. The contract is asymmetric: a
// case-sensitive substring match drives wrapping to tmux.ErrNoSuchSession,
// while every other failure mode (empty stderr, mixed-case stderr, unrelated
// stderr, non-*CommandError exec failure) must NOT match, but must still
// surface the wrapped error so callers can recover *CommandError via
// errors.As.
//
// The discriminator lives at the internal/tmux boundary (see errors.go) so
// daemon-layer callers can classify errors via errors.Is without coupling to
// tmux's exact stderr phrasing.
func TestShowEnvironment_ErrNoSuchSession(t *testing.T) {
	t.Run("it returns an error matching ErrNoSuchSession when stderr contains 'no such session'", func(t *testing.T) {
		mock := &MockCommander{Err: &tmux.CommandError{
			Stderr: "no such session: missing",
			Err:    errors.New("exit status 1"),
		}}
		client := tmux.NewClient(mock)

		_, err := client.ShowEnvironment("missing")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, tmux.ErrNoSuchSession) {
			t.Errorf("errors.Is(err, ErrNoSuchSession) = false, want true; err = %v", err)
		}
	})

	t.Run("it does not match ErrNoSuchSession when stderr is empty", func(t *testing.T) {
		mock := &MockCommander{Err: &tmux.CommandError{
			Stderr: "",
			Err:    errors.New("exit status 1"),
		}}
		client := tmux.NewClient(mock)

		_, err := client.ShowEnvironment("missing")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, tmux.ErrNoSuchSession) {
			t.Errorf("errors.Is(err, ErrNoSuchSession) = true, want false; err = %v", err)
		}
	})

	t.Run("it does not match ErrNoSuchSession for a non-CommandError exec failure", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("exec lookup failed")}
		client := tmux.NewClient(mock)

		_, err := client.ShowEnvironment("missing")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, tmux.ErrNoSuchSession) {
			t.Errorf("errors.Is(err, ErrNoSuchSession) = true, want false; err = %v", err)
		}
	})

	t.Run("it preserves *CommandError recoverability via errors.As", func(t *testing.T) {
		cmdErr := &tmux.CommandError{
			Stderr: "no such session: missing",
			Err:    errors.New("exit status 1"),
		}
		mock := &MockCommander{Err: cmdErr}
		client := tmux.NewClient(mock)

		_, err := client.ShowEnvironment("missing")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var recovered *tmux.CommandError
		if !errors.As(err, &recovered) {
			t.Fatalf("errors.As did not recover *CommandError from %v (%T)", err, err)
		}
		if recovered.Stderr != "no such session: missing" {
			t.Errorf("recovered Stderr = %q, want %q", recovered.Stderr, "no such session: missing")
		}
	})

	t.Run("it does not match ErrNoSuchSession for mixed-case 'No such session'", func(t *testing.T) {
		// tmux emits lowercase "no such session"; case-sensitive Contains is
		// intentional so daemon-layer classification cannot drift to a
		// permissive matcher that absorbs unrelated phrasings.
		mock := &MockCommander{Err: &tmux.CommandError{
			Stderr: "No such session: missing",
			Err:    errors.New("exit status 1"),
		}}
		client := tmux.NewClient(mock)

		_, err := client.ShowEnvironment("missing")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, tmux.ErrNoSuchSession) {
			t.Errorf("errors.Is(err, ErrNoSuchSession) = true, want false; err = %v", err)
		}
	})

	t.Run("it does not match ErrNoSuchSession for unrelated non-zero exits", func(t *testing.T) {
		mock := &MockCommander{Err: &tmux.CommandError{
			Stderr: "connection refused",
			Err:    errors.New("exit status 1"),
		}}
		client := tmux.NewClient(mock)

		_, err := client.ShowEnvironment("missing")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, tmux.ErrNoSuchSession) {
			t.Errorf("errors.Is(err, ErrNoSuchSession) = true, want false; err = %v", err)
		}
	})
}
