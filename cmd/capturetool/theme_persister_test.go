package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

func TestCapturetool_NoThemePersister(t *testing.T) {
	names := capture.FixtureNames()
	if len(names) == 0 {
		t.Fatal("no fixtures are registered; the enumeration below would be vacuous")
	}

	for _, name := range names {
		if capture.IsStandalone(name) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			fx, err := capture.FixtureByName(name)
			if err != nil {
				t.Fatalf("FixtureByName(%s): %v", name, err)
			}

			deps := fx.Deps(themetest.Builtin(t, defaultThemeSlug))
			if deps.ThemePersister != nil {
				t.Errorf("fixture %s wires a ThemePersister (%#v); a commit during a capture must write nowhere", name, deps.ThemePersister)
			}
			if deps.ModePersister != nil {
				t.Errorf("fixture %s wires a ModePersister (%#v); an `s`-toggle during a capture must write nowhere", name, deps.ModePersister)
			}
		})
	}
}

func TestCapturetool_WritesNoPrefsFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("PORTAL_PREFS_FILE", filepath.Join(dir, "prefs.json"))

	m, err := resolveModel("sessions-flat", themetest.Builtin(t, defaultThemeSlug))
	if err != nil {
		t.Fatalf("resolveModel(sessions-flat): %v", err)
	}

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for range 3 {
		model, _ = model.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	}
	if content := model.(tui.Model).View().Content; content == "" {
		t.Fatal("the capture model painted nothing; the no-write assertion would be vacuous")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read the isolated config dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("a capture wrote %d entr(y/ies) into the config dir: %v — the harness reads and writes no real config", len(entries), entries)
	}
}
