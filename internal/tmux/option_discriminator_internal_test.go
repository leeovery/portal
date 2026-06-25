package tmux

import (
	"errors"
	"slices"
	"testing"
)

// internalMockCommander is a minimal Commander implementation used by
// same-package tests that need to address unexported symbols (e.g. the
// optionAbsentStderrPatterns slice). The external package's MockCommander is
// not reachable from here without an import cycle, so we duplicate the
// pinhole shape locally — Run/RunRaw return the configured Output/Err and
// nothing else.
type internalMockCommander struct {
	Output string
	Err    error
}

func (m *internalMockCommander) Run(args ...string) (string, error) {
	return m.Output, m.Err
}

func (m *internalMockCommander) RunRaw(args ...string) (string, error) {
	return m.Output, m.Err
}

// TestGetServerOption_DiscriminatorSet exercises every entry in the
// unexported optionAbsentStderrPatterns slice directly. Each pattern, when
// it appears as a substring of a *CommandError.Stderr, must drive
// GetServerOption to ErrOptionNotFound. The unrelated_stderr_does_not_match
// negative subtest pins the contract that stderrs containing a colon but
// none of the absence phrasings must propagate, not collapse to absence.
//
// This test lives in package tmux (internal) because it iterates the
// unexported slice — that gives a one-line slice extension a corresponding
// test-coverage extension automatically, so adding a future pattern cannot
// silently drift away from its test surface.
func TestGetServerOption_DiscriminatorSet(t *testing.T) {
	for _, pat := range optionAbsentStderrPatterns {
		t.Run(pat, func(t *testing.T) {
			stderr := pat + " @foo"
			mock := &internalMockCommander{Err: &CommandError{
				Stderr: stderr,
				Err:    errors.New("exit status 1"),
			}}
			client := NewClient(mock)

			got, err := client.GetServerOption("@foo")

			if got != "" {
				t.Errorf("GetServerOption() = %q, want empty string", got)
			}
			if !errors.Is(err, ErrOptionNotFound) {
				t.Errorf("GetServerOption() error = %v, want ErrOptionNotFound", err)
			}
		})
	}

	t.Run("unrelated_stderr_does_not_match", func(t *testing.T) {
		stderr := "some unrelated error: connection refused"
		cmdErr := &CommandError{Stderr: stderr, Err: errors.New("exit status 1")}
		mock := &internalMockCommander{Err: cmdErr}
		client := NewClient(mock)

		got, err := client.GetServerOption("@foo")

		if got != "" {
			t.Errorf("GetServerOption() = %q, want empty string", got)
		}
		if err == nil {
			t.Fatal("GetServerOption() error = nil, want non-nil")
		}
		if errors.Is(err, ErrOptionNotFound) {
			t.Errorf("GetServerOption() error = %v, must not be ErrOptionNotFound", err)
		}
		var recovered *CommandError
		if !errors.As(err, &recovered) {
			t.Fatalf("errors.As did not recover *CommandError from %v (%T)", err, err)
		}
		if recovered.Stderr != stderr {
			t.Errorf("recovered Stderr = %q, want %q", recovered.Stderr, stderr)
		}
	})

	t.Run("slice_contents_pinned", func(t *testing.T) {
		// Pin the exact slice contents so future drift is loud. Order is
		// not load-bearing for the discriminator (strings.Contains is
		// checked against each entry independently), but membership is.
		want := []string{"invalid option:", "unknown option:", "ambiguous option:"}
		if len(optionAbsentStderrPatterns) != len(want) {
			t.Fatalf("optionAbsentStderrPatterns has %d entries, want %d (got %v)",
				len(optionAbsentStderrPatterns), len(want), optionAbsentStderrPatterns)
		}
		for _, w := range want {
			found := slices.Contains(optionAbsentStderrPatterns, w)
			if !found {
				t.Errorf("optionAbsentStderrPatterns missing pattern %q (got %v)",
					w, optionAbsentStderrPatterns)
			}
		}
	})
}
