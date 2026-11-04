package cmd

import (
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/saverhost"
)

func TestSaverMarks(t *testing.T) {
	dir := t.TempDir()
	b, err := bindZmxAt(zmxProgram, dir)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := saverMarks(b), []string{"ZMX_SESSION=_portal-saver", "ZMX_DIR=" + dir}; !slices.Equal(got, want) {
		t.Errorf("saverMarks = %q, want %q: the saver session's name and the bound directory", got, want)
	}
}

// A stop that SIGKILLed a daemon still inside its own shutdown bound would cut
// short a flush the daemon keeps to, and leave its successor meeting a live
// pid in the lock's check.
func TestSaverStopOutwaitsTheDaemonsShutdown(t *testing.T) {
	if saverhost.TermWait <= shutdownBound {
		t.Errorf("saverhost.TermWait = %s, want longer than the daemon's shutdown bound, %s", saverhost.TermWait, shutdownBound)
	}
}
