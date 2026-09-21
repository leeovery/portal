package capture

import (
	"slices"
	"strings"
	"testing"
)

func TestFixtureRegistry_BuilderNamesAreUnique(t *testing.T) {
	builders := fixtureBuilders()
	if len(builders) == 0 {
		t.Fatal("the builder list is empty; every registry assertion below would pass vacuously")
	}
	seen := make(map[string]int, len(builders))
	for i, build := range builders {
		name := build().Name()
		if name == "" {
			t.Errorf("builder %d names its fixture the empty string, which no lookup can address", i)
			continue
		}
		if prior, dup := seen[name]; dup {
			t.Errorf("builders %d and %d both name their fixture %s; the later one is unreachable by name", prior, i, name)
		}
		seen[name] = i
	}
}

func TestFixtureRegistry_NamesDeriveFromTheBuilders(t *testing.T) {
	standalone := StandaloneNames()
	want := make([]string, 0, len(fixtureBuilders())+len(standalone))
	for _, build := range fixtureBuilders() {
		want = append(want, build().Name())
	}
	want = append(want, standalone...)
	slices.Sort(want)

	if got := FixtureNames(); !slices.Equal(got, want) {
		t.Errorf("FixtureNames() = %v, want the builders' own names plus the standalone set %v, sorted: %v", got, standalone, want)
	}
}

func TestFixtureByName_ResolvesEveryEnumeratedName(t *testing.T) {
	for _, name := range FixtureNames() {
		if IsStandalone(name) {
			continue
		}
		fx, err := FixtureByName(name)
		if err != nil {
			t.Errorf("FixtureByName(%s): %v — every enumerated name outside %v must resolve", name, err, StandaloneNames())
			continue
		}
		if fx.Name() != name {
			t.Errorf("FixtureByName(%s) returned the fixture named %s", name, fx.Name())
		}
	}
}

func TestFixtureByName_UnknownNameErrorText(t *testing.T) {
	fx, err := FixtureByName("no-such-fixture")
	if fx != nil {
		t.Errorf("FixtureByName returned %v for an unknown name, want nil", fx)
	}
	want := `unknown fixture "no-such-fixture" (available: ` + strings.Join(FixtureNames(), ", ") + ")"
	if err == nil || err.Error() != want {
		t.Errorf("FixtureByName(no-such-fixture) error = %v, want %s", err, want)
	}
}
