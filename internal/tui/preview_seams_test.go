package tui_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// stubScrollbackReader is a minimal mock that satisfies tui.ScrollbackReader
// using only a paneKey lookup — proving stateDir is hidden behind the
// interface and not part of the method signature.
type stubScrollbackReader struct {
	bytes []byte
	err   error
}

func (s stubScrollbackReader) Tail(paneKey string) ([]byte, error) {
	return s.bytes, s.err
}

func TestTmuxEnumeratorIsSatisfiedByTmuxClient(t *testing.T) {
	// Compile-time assertion: *tmux.Client implements tui.TmuxEnumerator
	// via its Phase 1 ListWindowsAndPanesInSession method. If the method
	// signature drifts, this test fails to compile.
	var _ tui.TmuxEnumerator = (*tmux.Client)(nil)
}

func TestScrollbackReaderHidesStateDir(t *testing.T) {
	// Compile-time assertion: a mock implementing only Tail(paneKey string)
	// ([]byte, error) satisfies tui.ScrollbackReader. If a stateDir
	// parameter were ever added to Tail, this would fail to compile.
	var _ tui.ScrollbackReader = stubScrollbackReader{}
}

func TestScrollbackReaderSupportsThreeReturnShapes(t *testing.T) {
	// Three observable shapes per spec — each compiles against the interface
	// and is callable through it.
	tests := []struct {
		name      string
		reader    tui.ScrollbackReader
		wantBytes bool
		wantErr   bool
	}{
		{
			name:      "bytes and nil error renders content verbatim",
			reader:    stubScrollbackReader{bytes: []byte("content"), err: nil},
			wantBytes: true,
			wantErr:   false,
		},
		{
			name:      "nil bytes and nil error signals no saved content placeholder",
			reader:    stubScrollbackReader{bytes: nil, err: nil},
			wantBytes: false,
			wantErr:   false,
		},
		{
			name:      "nil bytes and error signals OS-level read failure",
			reader:    stubScrollbackReader{bytes: nil, err: errors.New("permission denied")},
			wantBytes: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.reader.Tail("any-pane-key")
			if tt.wantBytes && got == nil {
				t.Errorf("expected non-nil bytes, got nil")
			}
			if !tt.wantBytes && got != nil {
				t.Errorf("expected nil bytes, got %q", got)
			}
			if tt.wantErr && err == nil {
				t.Errorf("expected non-nil error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected nil error, got %v", err)
			}
		})
	}
}
