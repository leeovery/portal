package tmux_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
)

func TestShowEnvironment_ErrNoSuchSession(t *testing.T) {
	t.Run("it returns an error matching ErrNoSuchSession when stderr contains 'no such session'", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Fails(&tmux.CommandError{
			Stderr: "no such session: missing",
			Err:    errors.New("exit status 1"),
		}, "show-environment"))
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
		mock := commandertest.New(t, commandertest.Fails(&tmux.CommandError{
			Stderr: "",
			Err:    errors.New("exit status 1"),
		}, "show-environment"))
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
		mock := commandertest.New(t, commandertest.Fails(fmt.Errorf("exec lookup failed"), "show-environment"))
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
		mock := commandertest.New(t, commandertest.Fails(cmdErr, "show-environment"))
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
		mock := commandertest.New(t, commandertest.Fails(&tmux.CommandError{
			Stderr: "No such session: missing",
			Err:    errors.New("exit status 1"),
		}, "show-environment"))
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
		mock := commandertest.New(t, commandertest.Fails(&tmux.CommandError{
			Stderr: "connection refused",
			Err:    errors.New("exit status 1"),
		}, "show-environment"))
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
