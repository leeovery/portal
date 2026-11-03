package tui_test

import (
	"testing"

	"github.com/leeovery/portal/internal/tui"
)

func TestSession_DisplayNameShowsTheLabelElseTheName(t *testing.T) {
	tests := []struct {
		name    string
		session tui.Session
		want    string
	}{
		{"a label is shown", tui.Session{Name: "dotfiles-PpaTER", Label: "user search"}, "user search"},
		{"an empty label falls back to the name", tui.Session{Name: "dotfiles-PpaTER"}, "dotfiles-PpaTER"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.session.DisplayName(); got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}
