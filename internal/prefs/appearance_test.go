package prefs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
)

func TestLoadAppearance(t *testing.T) {
	t.Run("returns AppearanceAuto for a missing prefs file", func(t *testing.T) {
		dir := t.TempDir()
		store := prefs.NewStore(filepath.Join(dir, "nonexistent", "prefs.json"))

		got, err := store.LoadAppearance()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != prefs.AppearanceAuto {
			t.Errorf("appearance = %v, want AppearanceAuto", got)
		}
	})

	t.Run("returns AppearanceAuto when the field is missing from an otherwise valid file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "prefs.json")
		if err := os.WriteFile(filePath, []byte(`{"session_list_mode":"by-tag"}`), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := prefs.NewStore(filePath)
		got, err := store.LoadAppearance()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != prefs.AppearanceAuto {
			t.Errorf("appearance = %v, want AppearanceAuto", got)
		}
	})

	t.Run("returns AppearanceAuto for an empty prefs file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "prefs.json")
		if err := os.WriteFile(filePath, []byte(""), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := prefs.NewStore(filePath)
		got, err := store.LoadAppearance()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != prefs.AppearanceAuto {
			t.Errorf("appearance = %v, want AppearanceAuto", got)
		}
	})

	t.Run("returns AppearanceAuto for corrupt unparseable JSON", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "prefs.json")
		if err := os.WriteFile(filePath, []byte("{invalid json!!!"), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := prefs.NewStore(filePath)
		got, err := store.LoadAppearance()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != prefs.AppearanceAuto {
			t.Errorf("appearance = %v, want AppearanceAuto", got)
		}
	})

	t.Run("collapses an unrecognised appearance value to AppearanceAuto", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "prefs.json")
		if err := os.WriteFile(filePath, []byte(`{"appearance":"sepia"}`), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := prefs.NewStore(filePath)
		got, err := store.LoadAppearance()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != prefs.AppearanceAuto {
			t.Errorf("appearance = %v, want AppearanceAuto", got)
		}
	})

	t.Run("propagates only a non-ErrNotExist read error, returning AppearanceAuto alongside", func(t *testing.T) {
		dir := t.TempDir()
		// A directory at the prefs path makes os.ReadFile fail with a non-ErrNotExist
		// error (EISDIR), exercising the propagated-error branch.
		filePath := filepath.Join(dir, "prefs.json")
		if err := os.Mkdir(filePath, 0o755); err != nil {
			t.Fatalf("failed to create dir at prefs path: %v", err)
		}

		store := prefs.NewStore(filePath)
		got, err := store.LoadAppearance()
		if err == nil {
			t.Fatalf("expected a read error, got nil")
		}
		if got != prefs.AppearanceAuto {
			t.Errorf("appearance = %v, want AppearanceAuto alongside the error", got)
		}
	})
}

func TestAppearanceRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		want prefs.Appearance
	}{
		{"light", prefs.AppearanceLight},
		{"dark", prefs.AppearanceDark},
		{"auto", prefs.AppearanceAuto},
	}
	for _, c := range cases {
		t.Run("round-trips "+c.name+" through SaveAppearance and LoadAppearance", func(t *testing.T) {
			dir := t.TempDir()
			store := prefs.NewStore(filepath.Join(dir, "prefs.json"))

			if err := store.SaveAppearance(c.want); err != nil {
				t.Fatalf("unexpected save error: %v", err)
			}

			got, err := store.LoadAppearance()
			if err != nil {
				t.Fatalf("unexpected load error: %v", err)
			}
			if got != c.want {
				t.Errorf("appearance = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAppearanceString(t *testing.T) {
	cases := []struct {
		appearance prefs.Appearance
		want       string
	}{
		{prefs.AppearanceAuto, "auto"},
		{prefs.AppearanceLight, "light"},
		{prefs.AppearanceDark, "dark"},
		{prefs.Appearance(99), "auto"}, // out-of-range maps to auto
	}
	for _, c := range cases {
		if got := c.appearance.String(); got != c.want {
			t.Errorf("appearance %d String() = %q, want %q", c.appearance, got, c.want)
		}
	}
}

func TestNoCrossFieldBlanking(t *testing.T) {
	t.Run("SaveAppearance preserves a previously-saved session_list_mode", func(t *testing.T) {
		dir := t.TempDir()
		store := prefs.NewStore(filepath.Join(dir, "prefs.json"))

		if err := store.Save(prefs.ModeByTag); err != nil {
			t.Fatalf("unexpected Save error: %v", err)
		}
		if err := store.SaveAppearance(prefs.AppearanceDark); err != nil {
			t.Fatalf("unexpected SaveAppearance error: %v", err)
		}

		mode, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected Load error: %v", err)
		}
		if mode != prefs.ModeByTag {
			t.Errorf("session_list_mode = %v, want ModeByTag (blanked by SaveAppearance)", mode)
		}
		appearance, err := store.LoadAppearance()
		if err != nil {
			t.Fatalf("unexpected LoadAppearance error: %v", err)
		}
		if appearance != prefs.AppearanceDark {
			t.Errorf("appearance = %v, want AppearanceDark", appearance)
		}
	})

	t.Run("Save preserves a previously-saved appearance", func(t *testing.T) {
		dir := t.TempDir()
		store := prefs.NewStore(filepath.Join(dir, "prefs.json"))

		if err := store.SaveAppearance(prefs.AppearanceLight); err != nil {
			t.Fatalf("unexpected SaveAppearance error: %v", err)
		}
		if err := store.Save(prefs.ModeByProject); err != nil {
			t.Fatalf("unexpected Save error: %v", err)
		}

		appearance, err := store.LoadAppearance()
		if err != nil {
			t.Fatalf("unexpected LoadAppearance error: %v", err)
		}
		if appearance != prefs.AppearanceLight {
			t.Errorf("appearance = %v, want AppearanceLight (blanked by Save)", appearance)
		}
		mode, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected Load error: %v", err)
		}
		if mode != prefs.ModeByProject {
			t.Errorf("session_list_mode = %v, want ModeByProject", mode)
		}
	})

	t.Run("both fields are present in the on-disk JSON after both saves", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "prefs.json")
		store := prefs.NewStore(filePath)

		if err := store.Save(prefs.ModeByTag); err != nil {
			t.Fatalf("unexpected Save error: %v", err)
		}
		if err := store.SaveAppearance(prefs.AppearanceDark); err != nil {
			t.Fatalf("unexpected SaveAppearance error: %v", err)
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read prefs file: %v", err)
		}
		got := string(data)
		if !strings.Contains(got, `"session_list_mode": "by-tag"`) {
			t.Errorf("file content = %q, want session_list_mode by-tag", got)
		}
		if !strings.Contains(got, `"appearance": "dark"`) {
			t.Errorf("file content = %q, want appearance dark", got)
		}
	})
}
