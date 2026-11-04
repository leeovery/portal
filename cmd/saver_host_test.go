package cmd

import (
	"testing"

	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/saverhost"
)

func TestNewSaverStopper(t *testing.T) {
	t.Run("it binds a zmx directory's saver", func(t *testing.T) {
		b, err := bindZmxAt(zmxProgram, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := newSaverStopper(b); err != nil {
			t.Errorf("newSaverStopper = %v, want the saver of %s", err, b.Dir())
		}
	})

	t.Run("it refuses a backend whose reach names no directory", func(t *testing.T) {
		if s, err := newSaverStopper(backendtest.New()); err == nil {
			t.Errorf("newSaverStopper = %v, want a refusal", s)
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
