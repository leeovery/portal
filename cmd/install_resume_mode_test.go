package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/resumemode"
)

func TestInstallResumeModeOf(t *testing.T) {
	t.Run("it answers the shipped default when there is no store", func(t *testing.T) {
		if got := installResumeModeOf(nil); got != resumemode.Default {
			t.Errorf("installResumeModeOf(nil) = %v; want %v", got, resumemode.Default)
		}
	})

	t.Run("it answers the mode the store holds", func(t *testing.T) {
		for _, tc := range []struct {
			body string
			want resumemode.Mode
		}{
			{body: `{"resume_mode":"eager"}`, want: resumemode.Eager},
			{body: `{"resume_mode":"lazy"}`, want: resumemode.Lazy},
		} {
			path := filepath.Join(t.TempDir(), "prefs.json")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatalf("write prefs.json: %v", err)
			}
			if got := installResumeModeOf(prefs.NewStore(path)); got != tc.want {
				t.Errorf("installResumeModeOf(%s) = %v; want %v", tc.body, got, tc.want)
			}
		}
	})

	t.Run("it answers the shipped default when the store cannot be read", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "prefs.json")
		if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
			t.Fatalf("write prefs.json: %v", err)
		}
		if got := installResumeModeOf(prefs.NewStore(path)); got != resumemode.Default {
			t.Errorf("installResumeModeOf(corrupt) = %v; want %v", got, resumemode.Default)
		}
	})
}
