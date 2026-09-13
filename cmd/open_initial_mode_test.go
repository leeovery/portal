package cmd

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tui"
)

type fakeModePersister struct {
	calls int
	last  prefs.SessionListMode
}

func (f *fakeModePersister) Save(mode prefs.SessionListMode) error {
	f.calls++
	f.last = mode
	return nil
}

var keyS = tea.KeyPressMsg{Code: 's', Text: "s"}

func TestBuildTUIModel_InjectsInitialMode(t *testing.T) {
	t.Run("Flat initial mode paints the plain Sessions title", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.initialMode = prefs.ModeFlat

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		if got := m.SessionListTitle(); got != "Sessions" {
			t.Errorf("SessionListTitle() = %q, want %q", got, "Sessions")
		}
	})

	t.Run("By Tag initial mode paints the by-tag title on the first frame", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.initialMode = prefs.ModeByTag

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		if got := m.SessionListTitle(); got != "Sessions — by tag" {
			t.Errorf("SessionListTitle() = %q, want %q", got, "Sessions — by tag")
		}
	})

	t.Run("By Project initial mode paints the by-project title", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.initialMode = prefs.ModeByProject

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		if got := m.SessionListTitle(); got != "Sessions — by project" {
			t.Errorf("SessionListTitle() = %q, want %q", got, "Sessions — by project")
		}
	})
}

func TestBuildTUIModel_InjectsPersister(t *testing.T) {
	t.Run("pressing s persists the advanced mode through the injected persister", func(t *testing.T) {
		persister := &fakeModePersister{}
		cfg := defaultTestTUIConfig()
		cfg.initialMode = prefs.ModeFlat
		cfg.modePersister = persister

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		updated, _ := m.Update(keyS)
		_ = updated
		if persister.calls != 1 {
			t.Fatalf("persister.calls = %d, want 1", persister.calls)
		}
		if persister.last != prefs.ModeByProject {
			t.Errorf("persister.last = %v, want ModeByProject", persister.last)
		}
	})

	t.Run("tolerates a nil persister without panicking on s", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.initialMode = prefs.ModeFlat
		cfg.modePersister = nil

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		// Must not panic; the toggle still advances in-memory.
		updated, _ := m.Update(keyS)
		if got := updated.(tui.Model).SessionListTitle(); got != "Sessions — by project" {
			t.Errorf("SessionListTitle() = %q, want %q (nil persister must still toggle)", got, "Sessions — by project")
		}
	})
}

// An empty content string leaves the file absent, simulating a first-ever
// launch.
func setPrefsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prefs.json")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write prefs file: %v", err)
		}
	}
	t.Setenv("PORTAL_PREFS_FILE", path)
	return path
}

// Mirrors the tolerant prefs read openTUI performs at construction: any
// failure collapses to Flat.
func loadInitialModeForTest(t *testing.T) prefs.SessionListMode {
	t.Helper()
	load, err := loadPrefsStore()
	if err != nil || load.Store == nil {
		return prefs.ModeFlat
	}
	mode, _ := load.Store.Load()
	return mode
}

func TestOpenTUI_InitialModeFromPrefs(t *testing.T) {
	t.Run("opens in Flat for a first-ever launch with no prefs file", func(t *testing.T) {
		setPrefsFile(t, "")

		if got := loadInitialModeForTest(t); got != prefs.ModeFlat {
			t.Errorf("initial mode = %v, want ModeFlat", got)
		}
	})

	t.Run("opens in By Tag after By Tag was persisted", func(t *testing.T) {
		setPrefsFile(t, `{"session_list_mode":"by-tag"}`)

		if got := loadInitialModeForTest(t); got != prefs.ModeByTag {
			t.Errorf("initial mode = %v, want ModeByTag", got)
		}
	})

	t.Run("opens in Flat for a corrupt prefs file", func(t *testing.T) {
		setPrefsFile(t, "{ this is not json")

		if got := loadInitialModeForTest(t); got != prefs.ModeFlat {
			t.Errorf("initial mode = %v, want ModeFlat (corrupt prefs falls back to Flat)", got)
		}
	})

	t.Run("round-trips a persisted toggle back through a fresh read", func(t *testing.T) {
		setPrefsFile(t, "")

		load, err := loadPrefsStore()
		if err != nil {
			t.Fatalf("loadPrefsStore: %v", err)
		}
		cfg := defaultTestTUIConfig()
		cfg.initialMode = loadInitialModeForTest(t)
		cfg.modePersister = load.Store

		m := buildTUIModel(cfg, pickerLanding{}, nil)
		m.Update(keyS)

		if got := loadInitialModeForTest(t); got != prefs.ModeByProject {
			t.Errorf("re-read initial mode = %v, want ModeByProject", got)
		}
	})
}
