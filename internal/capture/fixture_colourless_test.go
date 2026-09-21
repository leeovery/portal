package capture

import (
	"testing"

	"github.com/leeovery/portal/internal/theme"
)

func TestFixtureColourless_ReadsDepsNoColor(t *testing.T) {
	t.Run("false when the fixture sets no NO_COLOR flag", func(t *testing.T) {
		fx := &Fixture{Lister: &fakeLister{}, projectStore: &fakeProjectStore{}}
		if fx.Deps(theme.Theme{}).NoColor {
			t.Fatal("probe setup: Deps().NoColor is true on an unflagged fixture")
		}
		if fx.Colourless() {
			t.Error("Colourless() = true on an unflagged fixture, want false")
		}
	})

	t.Run("true when the fixture carries the NO_COLOR flag", func(t *testing.T) {
		fx := &Fixture{Lister: &fakeLister{}, projectStore: &fakeProjectStore{}, noColor: true}
		if !fx.Deps(theme.Theme{}).NoColor {
			t.Fatal("probe setup: Deps() does not carry the fixture's NO_COLOR flag through to the model seam set")
		}
		if !fx.Colourless() {
			t.Error("Colourless() = false on a NO_COLOR fixture, want true — the guard's exclusion would miss it")
		}
	})

	t.Run("every registered fixture agrees with its own Deps", func(t *testing.T) {
		for _, name := range FixtureNames() {
			if IsStandalone(name) {
				continue
			}
			fx, err := FixtureByName(name)
			if err != nil {
				t.Fatalf("FixtureByName(%s): %v", name, err)
			}
			if got, want := fx.Colourless(), fx.Deps(theme.Theme{}).NoColor; got != want {
				t.Errorf("%s: Colourless() = %t, Deps().NoColor = %t", name, got, want)
			}
		}
	})
}
