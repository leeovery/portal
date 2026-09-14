package tui

import (
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

func pickerSessionNames(sessions []tmux.Session) []string {
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, s.Name)
	}
	return names
}

func TestPickerSessions(t *testing.T) {
	t.Run("it drops the named session and keeps the rest in enumeration order", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "alpha"}, {Name: "current"}, {Name: "beta"}}

		got := pickerSessionNames(PickerSessions(sessions, "current"))

		if want := []string{"alpha", "beta"}; !slices.Equal(got, want) {
			t.Errorf("PickerSessions() = %v, want %v", got, want)
		}
	})

	t.Run("it drops nothing when the current session name is empty", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "alpha"}, {Name: "beta"}}

		got := pickerSessionNames(PickerSessions(sessions, ""))

		if want := []string{"alpha", "beta"}; !slices.Equal(got, want) {
			t.Errorf("PickerSessions() = %v, want %v", got, want)
		}
	})

	t.Run("it drops nothing when no session carries the name", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "alpha"}, {Name: "beta"}}

		got := pickerSessionNames(PickerSessions(sessions, "gamma"))

		if want := []string{"alpha", "beta"}; !slices.Equal(got, want) {
			t.Errorf("PickerSessions() = %v, want %v", got, want)
		}
	})

	t.Run("it leaves the caller's slice unmodified", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "alpha"}, {Name: "current"}, {Name: "beta"}}

		PickerSessions(sessions, "current")

		got := pickerSessionNames(sessions)
		if want := []string{"alpha", "current", "beta"}; !slices.Equal(got, want) {
			t.Errorf("caller's slice = %v, want %v unchanged", got, want)
		}
	})
}
