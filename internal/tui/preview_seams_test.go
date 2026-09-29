package tui_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

type stubScrollbackReader struct {
	bytes []byte
	err   error
}

func (s stubScrollbackReader) Tail(tui.PaneScrollback) ([]byte, error) {
	return s.bytes, s.err
}

func TestTmuxEnumeratorIsSatisfiedByTmuxClient(t *testing.T) {
	var _ tui.TmuxEnumerator = (*tmux.Client)(nil)
}

func TestScrollbackReaderHidesStateDir(t *testing.T) {
	var _ tui.ScrollbackReader = stubScrollbackReader{}
}

func TestScrollbackReaderSupportsThreeReturnShapes(t *testing.T) {
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
			got, err := tt.reader.Tail(tui.PaneScrollback{PaneKey: "any-pane-key"})
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
