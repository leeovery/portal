package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/saverhost"
)

func TestNewSaverHost(t *testing.T) {
	t.Run("it binds a zmx directory's saver", func(t *testing.T) {
		b, err := bindZmxAt(zmxProgram, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := newSaverHost(b, t.TempDir()); err != nil {
			t.Errorf("newSaverHost = %v, want the saver of %s", err, b.Dir())
		}
	})

	t.Run("it compares the saver's zmx directory as the zmx-dir claim does", func(t *testing.T) {
		dir := t.TempDir()
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(dir, link); err != nil {
			t.Fatal(err)
		}
		b := backendtest.New()
		b.SetReachEnv([]string{"ZMX_DIR=" + dir})

		cfg, err := saverHostConfig(b, t.TempDir())
		if err != nil {
			t.Fatalf("saverHostConfig: %v", err)
		}
		canonical := cfg.Canonical["ZMX_DIR"]
		if canonical == nil {
			t.Fatalf("Canonical = %v, want ZMX_DIR compared canonically", cfg.Canonical)
		}
		for _, spelling := range []string{dir + "/", link} {
			if got, want := canonical(spelling), zmx.CanonicalDir(dir); got != want {
				t.Errorf("canonical(%q) = %q, want zmx.CanonicalDir's %q", spelling, got, want)
			}
		}
	})

	t.Run("it refuses a backend whose reach names no directory", func(t *testing.T) {
		if h, err := newSaverHost(backendtest.New(), t.TempDir()); err == nil {
			t.Errorf("newSaverHost = %v, want a refusal", h)
		}
	})
}

// A stop that SIGKILLed a daemon still inside its own shutdown bound would cut
// short a flush the daemon keeps to, and leave its successor meeting a live
// pid in the lock's check.
func TestSaverStopOutwaitsTheDaemonsShutdown(t *testing.T) {
	if saverhost.TermWait <= shutdownBound {
		t.Errorf("saverhost.TermWait = %s, want longer than the daemon's shutdown bound, %s", saverhost.TermWait, shutdownBound)
	}
}
