package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
)

func TestWithAppearance(t *testing.T) {
	t.Run("sets the appearance field on construction", func(t *testing.T) {
		m := New(fakeLister{}, WithAppearance(prefs.AppearanceDark))
		if m.appearance != prefs.AppearanceDark {
			t.Errorf("appearance = %v, want AppearanceDark", m.appearance)
		}
	})

	t.Run("defaults to AppearanceAuto when the option is omitted", func(t *testing.T) {
		m := New(fakeLister{})
		if m.appearance != prefs.AppearanceAuto {
			t.Errorf("appearance = %v, want AppearanceAuto", m.appearance)
		}
	})

	t.Run("stores AppearanceLight", func(t *testing.T) {
		m := New(fakeLister{}, WithAppearance(prefs.AppearanceLight))
		if m.appearance != prefs.AppearanceLight {
			t.Errorf("appearance = %v, want AppearanceLight", m.appearance)
		}
	})
}
